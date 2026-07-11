"""Assignment runtime for the Python Egress SDK.

Builds on ``protocol.py`` (Envelope/subject construction, registration
signing) and ``natsclient.py`` (the minimal Core NATS wire client) to run a
worker that registers with Control, heartbeats, and serves decoded-HTTP
assignments for one session at a time.

Behavioral reference: ``sdk/egress/runtime.go`` (registration/heartbeat loop)
and ``sdk/egress/assignment.go`` + ``sdk/egress/stream.go`` (assignment
subscription ordering, stream_seq validation, credit-based backpressure).
See ``docs/public/architecture.md`` for the wire protocol this module
implements.

ponytail: serves one assignment at a time (no concurrent in-session
requests, no raw tunnel/BodyRef/MITM). Sufficient for a single decoded-HTTP
custom worker process; add a per-request worker pool if a Python worker
needs the Go SDK's per-session concurrency.
"""

from __future__ import annotations

import time
from collections import deque
from dataclasses import dataclass, field
from typing import Callable, Deque, Iterable, List, Optional, Tuple

from straw.egress import protocol
from straw.egress.natsclient import NATSClient, NATSMessage, NATSProtocolError
from straw.proto.straw.v1 import straw_pb2 as pb

DEFAULT_HEARTBEAT_INTERVAL = 5.0
REGISTER_TIMEOUT = 5.0
HEARTBEAT_TIMEOUT = 5.0
FRAME_IDLE_TIMEOUT = 15.0
RESPONSE_FRAME_DATA_BYTES = 32 * 1024

_ERROR_FACT_DETAIL_KEY = "fact"


class ProtocolError(RuntimeError):
    """Raised on a stream protocol violation: sequence gap, attempt
    mismatch, or a frame received after the stream reached a terminal
    state."""


class RegistrationError(RuntimeError):
    """Raised when Control rejects registration or a heartbeat."""


# --- Executor callable seam -------------------------------------------------


@dataclass
class DecodedRequest:
    """The decoded HTTP request handed to a custom worker's executor."""

    method: str
    url: str
    headers: List[Tuple[str, bytes]]
    body: bytes
    attempt: int


@dataclass
class DecodedResponse:
    """What an executor returns: status/headers up front, and a body as an
    iterable of chunks so the runtime can stream it without buffering the
    full response.
    """

    status: int
    headers: List[Tuple[str, bytes]] = field(default_factory=list)
    body: Iterable[bytes] = ()


# Executor: decoded HTTP request in, a DecodedResponse out (whose body may be
# a generator). Raising from the callable, or from iterating its response
# body, maps to an ErrorFrame.
Executor = Callable[[DecodedRequest], DecodedResponse]


# --- Stream frame sequence validation (mirrors sdk/egress/stream.go) -------


class _StreamValidator:
    def __init__(self, attempt: int, initial_credit: int, idle_timeout: float = FRAME_IDLE_TIMEOUT) -> None:
        self._attempt = attempt
        self._expected_seq = 1
        self._offset = 0
        self._credit = initial_credit
        self._terminal = False
        self._idle_timeout = idle_timeout
        self._last_activity = time.monotonic()

    def idle_expired(self) -> bool:
        return self._idle_timeout > 0 and not self._terminal and (time.monotonic() - self._last_activity) >= self._idle_timeout

    def accept(self, frame: "pb.StreamFrame") -> str:
        """Returns "accepted", "duplicate" (late/redundant, ignore), or
        raises ProtocolError for a gap, attempt mismatch, frame-after-
        terminal, or credit-exhausted violation.
        """
        if frame is None or frame.WhichOneof("payload") is None:
            raise ProtocolError("stream frame missing payload")

        if self._terminal:
            raise ProtocolError("frame received after terminal frame")

        if frame.stream_seq == 0:
            raise ProtocolError("stream frame missing stream_seq")

        if frame.attempt != self._attempt:
            raise ProtocolError(f"attempt mismatch: expected {self._attempt}, got {frame.attempt}")

        if frame.stream_seq < self._expected_seq:
            return "duplicate"
        if frame.stream_seq > self._expected_seq:
            raise ProtocolError(f"stream sequence gap: expected {self._expected_seq}, got {frame.stream_seq}")

        which = frame.WhichOneof("payload")
        if which == "data":
            self._accept_data(frame.data)

        self._expected_seq += 1
        self._last_activity = time.monotonic()

        if which in ("end", "error", "cancelled"):
            self._terminal = True

        return "accepted"

    def _accept_data(self, data: "pb.DataFrame") -> None:
        if data.offset != self._offset:
            raise ProtocolError(f"data offset mismatch: expected {self._offset}, got {data.offset}")

        n = len(data.data)
        if n > self._credit:
            raise ProtocolError("upload credit exhausted")

        self._credit -= n
        self._offset += n


# --- Message dispatch over one NATS connection ------------------------------


class _Dispatcher:
    """Routes inbound NATS messages by subscription id over one connection.

    The wire client only exposes "read the next message"; a worker has
    several subscriptions active at once (assignment subject, the reply
    inbox, a request's c2e subject). This buffers messages that do not match
    what the caller is waiting for so a later matching call still sees them.
    """

    def __init__(self, conn: NATSClient) -> None:
        self._conn = conn
        self._pending: Deque[NATSMessage] = deque()

    def subscribe(self, subject: str) -> str:
        return self._conn.subscribe(subject)

    def unsubscribe(self, sid: str) -> None:
        self._conn.unsubscribe(sid)

    def flush(self, timeout: float = 5.0) -> None:
        self._conn.flush(timeout=timeout)

    def publish(self, subject: str, payload: bytes, reply_to: str = "") -> None:
        self._conn.publish(subject, payload, reply_to=reply_to)

    def recv(self, timeout: Optional[float] = None) -> Optional[NATSMessage]:
        if self._pending:
            return self._pending.popleft()
        return self._conn.next_message(timeout=timeout)

    def recv_matching(self, predicate: Callable[[NATSMessage], bool], deadline: Optional[float]) -> Optional[NATSMessage]:
        """Blocks until a message matching predicate arrives, buffering
        every non-matching message for later, or returns None past deadline
        (a monotonic time, or None for no deadline).
        """
        still_pending: List[NATSMessage] = []
        while self._pending:
            msg = self._pending.popleft()
            if predicate(msg):
                self._pending.extendleft(reversed(still_pending))
                return msg
            still_pending.append(msg)
        self._pending.extend(still_pending)

        while True:
            remaining = None if deadline is None else max(0.0, deadline - time.monotonic())
            if remaining is not None and remaining <= 0 and deadline is not None:
                return None
            msg = self._conn.next_message(timeout=remaining)
            if msg is None:
                return None
            if predicate(msg):
                return msg
            self._pending.append(msg)


# --- Registration / heartbeat ------------------------------------------------


class Runtime:
    """Registers a worker identity with Control and keeps it heartbeating.

    Use ``register`` once, then ``run`` to heartbeat and serve assignments
    until ``stop`` is set (or the process is torn down).
    """

    def __init__(self, conn: NATSClient, identity: protocol.Identity, caps: Optional[protocol.Capabilities] = None) -> None:
        self._conn = conn
        self._dispatch = _Dispatcher(conn)
        self.identity = identity
        self.caps = caps or protocol.Capabilities()
        self._inbox_prefix = identity.inbox_prefix()
        self._inbox_sid = self._dispatch.subscribe(f"{self._inbox_prefix}.>")
        self._dispatch.flush()
        self._inbox_counter = 0
        self.session_id: Optional[str] = None

    def _next_inbox(self) -> str:
        self._inbox_counter += 1
        return f"{self._inbox_prefix}.{self._inbox_counter}"

    def _request(self, subject: str, payload: bytes, timeout: float) -> bytes:
        reply_to = self._next_inbox()
        self._dispatch.publish(subject, payload, reply_to=reply_to)
        deadline = time.monotonic() + timeout
        msg = self._dispatch.recv_matching(lambda m: m.subject == reply_to, deadline)
        if msg is None:
            raise NATSProtocolError(f"timed out waiting for reply on {reply_to}")
        return msg.data

    def register(self) -> str:
        """Sends a registration request and stores the returned session id."""
        req = protocol.build_register_request(self.identity, self.caps)
        raw = protocol.marshal_envelope(protocol.register_envelope(req))
        reply = self._request(protocol.registration_subject(), raw, REGISTER_TIMEOUT)
        env = protocol.unmarshal_envelope(reply)
        ack = env.register_ack
        if not ack.ok:
            raise RegistrationError(ack.error or "registration rejected")
        if not ack.session_id:
            raise RegistrationError("registration accepted without session id")
        self.session_id = ack.session_id
        return ack.session_id

    def heartbeat(self, active_requests: int = 0, draining: bool = False) -> None:
        if self.session_id is None:
            raise RegistrationError("heartbeat requires a registered session")
        max_concurrency = self.caps.max_concurrency
        available = 0 if max_concurrency == 0 else max(0, max_concurrency - active_requests)
        hb = protocol.build_heartbeat(
            self.identity,
            self.session_id,
            pb.WORKER_HEALTH_READY,
            active_requests,
            available,
            max_concurrency,
            draining,
        )
        raw = protocol.marshal_envelope(protocol.heartbeat_envelope(hb))
        reply = self._request(protocol.heartbeat_subject(), raw, HEARTBEAT_TIMEOUT)
        env = protocol.unmarshal_envelope(reply)
        ack = env.heartbeat_ack
        if not ack.ok:
            raise RegistrationError(ack.error or "heartbeat rejected")

    def worker(self, executor: Executor) -> "Worker":
        if self.session_id is None:
            raise RegistrationError("worker requires a registered session")
        return Worker(self._dispatch, self.identity, self.session_id, executor, self.caps.max_concurrency)


# --- Assignment worker --------------------------------------------------


class Worker:
    """Serves decoded-HTTP assignments for one registered session, one
    request at a time.
    """

    def __init__(self, dispatch: _Dispatcher, identity: protocol.Identity, session_id: str, executor: Executor, max_concurrency: int = 0) -> None:
        self._dispatch = dispatch
        self._identity = identity
        self._session_id = session_id
        self._executor = executor
        self._max_concurrency = max_concurrency
        self._draining = False
        self._active = 0
        self._assign_sid: Optional[str] = None

    def start(self) -> None:
        """Subscribes to the session's assignment subject and flushes it,
        per the Assignment Flow subscription-ordering rule."""
        subject = protocol.assignment_subject(self._identity.worker_id, self._session_id)
        self._assign_sid = self._dispatch.subscribe(subject)
        self._dispatch.flush()

    def active_requests(self) -> int:
        return self._active

    def serve_one(self, timeout: Optional[float] = None) -> bool:
        """Waits for and fully serves one assignment. Returns False on
        timeout with nothing to do."""
        if self._assign_sid is None:
            self.start()

        deadline = None if timeout is None else time.monotonic() + timeout
        msg = self._dispatch.recv_matching(lambda m: m.sid == self._assign_sid, deadline)
        if msg is None:
            return False

        self._handle_assign(msg)
        return True

    def _handle_assign(self, msg: NATSMessage) -> None:
        env = protocol.unmarshal_envelope(msg.data)
        req = env.assign_request

        code = self._evaluate(req)
        if code != pb.ASSIGN_ACK_ACCEPTED:
            self._reply(msg, env, code)
            return

        c2e_subject = protocol.stream_subject(env.request_id, self._identity.worker_id, self._session_id, protocol.DIRECTION_CONTROL_TO_EXECUTOR)
        e2c_subject = protocol.stream_subject(env.request_id, self._identity.worker_id, self._session_id, protocol.DIRECTION_EXECUTOR_TO_CONTROL)

        c2e_sid = self._dispatch.subscribe(c2e_subject)
        self._dispatch.flush()

        self._reply(msg, env, pb.ASSIGN_ACK_ACCEPTED)

        self._active += 1
        try:
            self._run_decoded_request(env, req, c2e_sid, e2c_subject)
        finally:
            self._dispatch.unsubscribe(c2e_sid)
            self._active -= 1

    def _evaluate(self, req: "pb.AssignRequest") -> int:
        if self._draining:
            return pb.ASSIGN_ACK_REJECTED_DRAINING
        if req.mode != pb.REQUEST_MODE_DECODED_HTTP:
            return pb.ASSIGN_ACK_REJECTED_UNSUPPORTED
        if self._max_concurrency and self._active >= self._max_concurrency:
            return pb.ASSIGN_ACK_REJECTED_CAPACITY
        return pb.ASSIGN_ACK_ACCEPTED

    def _reply(self, msg: NATSMessage, env: "pb.Envelope", code: int) -> None:
        reply = pb.Envelope(
            request_id=env.request_id,
            tenant_id=env.tenant_id,
            trace_id=env.trace_id,
            deadline_unix_ms=env.deadline_unix_ms,
            protocol_major=protocol.PROTOCOL_MAJOR,
            attempt=env.attempt,
            assign_ack=pb.AssignAck(code=code),
        )
        self._dispatch.publish(msg.reply_to, protocol.marshal_envelope(reply))

    def _run_decoded_request(self, env: "pb.Envelope", req: "pb.AssignRequest", c2e_sid: str, e2c_subject: str) -> None:
        validator = _StreamValidator(attempt=req.attempt, initial_credit=req.initial_upload_credit_bytes)
        seq_state = _SeqCounter()

        try:
            start, body = self._read_request_body(c2e_sid, validator, req.expected_upload_bytes)
        except ProtocolError as exc:
            self._publish_error(env, e2c_subject, seq_state, req.attempt, pb.ERROR_CODE_PROTOCOL_ERROR, str(exc))
            return

        try:
            decoded = DecodedRequest(
                method=start.method,
                url=start.url,
                headers=[(h.name, h.value) for h in start.headers],
                body=body,
                attempt=req.attempt,
            )
            response = self._executor(decoded)
            self._publish_response(env, e2c_subject, seq_state, req, c2e_sid, validator, response)
        except Exception as exc:  # noqa: BLE001 - any executor failure maps to an ErrorFrame
            self._publish_error(env, e2c_subject, seq_state, req.attempt, pb.ERROR_CODE_EXECUTOR_INTERNAL_ERROR, str(exc))

    def _read_request_body(self, c2e_sid: str, validator: _StreamValidator, expected_upload_bytes: int) -> Tuple["pb.RequestStart", bytes]:
        start: Optional["pb.RequestStart"] = None
        body = bytearray()
        expected = max(0, expected_upload_bytes)

        while start is None or len(body) < expected:
            deadline = time.monotonic() + FRAME_IDLE_TIMEOUT
            msg = self._dispatch.recv_matching(lambda m: m.sid == c2e_sid, deadline)
            if msg is None:
                raise ProtocolError("timed out waiting for request body frames")

            env = protocol.unmarshal_envelope(msg.data)
            frame = env.stream_frame
            outcome = validator.accept(frame)
            if outcome == "duplicate":
                continue

            which = frame.WhichOneof("payload")
            if which == "request_start":
                start = frame.request_start
            elif which == "data":
                body.extend(frame.data.data)
            elif which == "credit":
                continue
            elif which == "cancel":
                raise ProtocolError("request cancelled before body was fully read")
            else:
                raise ProtocolError(f"unexpected frame during request body read: {which}")

        return start, bytes(body)

    def _publish_response(
        self,
        env: "pb.Envelope",
        e2c_subject: str,
        seq_state: "_SeqCounter",
        req: "pb.AssignRequest",
        c2e_sid: str,
        validator: _StreamValidator,
        response: DecodedResponse,
    ) -> None:
        headers = [pb.Header(name=name, value=value) for name, value in response.headers]
        self._publish_frame(env, e2c_subject, seq_state, req.attempt, pb.StreamFrame(response_start=pb.ResponseStart(status=response.status, headers=headers)))

        credit = _CreditGate(req.initial_download_credit_bytes, self._dispatch, c2e_sid, validator)
        offset = 0
        for chunk in response.body:
            remaining = chunk
            while remaining:
                take = credit.take(min(len(remaining), RESPONSE_FRAME_DATA_BYTES))
                piece, remaining = remaining[:take], remaining[take:]
                self._publish_frame(env, e2c_subject, seq_state, req.attempt, pb.StreamFrame(data=pb.DataFrame(offset=offset, data=piece)))
                offset += len(piece)

        self._publish_frame(env, e2c_subject, seq_state, req.attempt, pb.StreamFrame(end=pb.EndFrame(success=True)))

    def _publish_error(self, env: "pb.Envelope", e2c_subject: str, seq_state: "_SeqCounter", attempt: int, code: int, message: str) -> None:
        self._publish_frame(
            env,
            e2c_subject,
            seq_state,
            attempt,
            pb.StreamFrame(error=pb.ErrorFrame(code=code, message=message, details={_ERROR_FACT_DETAIL_KEY: message})),
        )

    def _publish_frame(self, env: "pb.Envelope", subject: str, seq_state: "_SeqCounter", attempt: int, frame: "pb.StreamFrame") -> None:
        frame.stream_seq = seq_state.next()
        frame.attempt = attempt
        out = pb.Envelope(
            request_id=env.request_id,
            tenant_id=env.tenant_id,
            trace_id=env.trace_id,
            deadline_unix_ms=env.deadline_unix_ms,
            protocol_major=protocol.PROTOCOL_MAJOR,
            attempt=attempt,
            stream_frame=frame,
        )
        self._dispatch.publish(subject, protocol.marshal_envelope(out))


class _SeqCounter:
    def __init__(self) -> None:
        self._seq = 0

    def next(self) -> int:
        self._seq += 1
        return self._seq


class _CreditGate:
    """Byte-credit backpressure for the e2c (response) direction: blocks for
    a CreditFrame on c2e when the current grant is exhausted, matching
    ``docs/public/architecture.md``'s "Backpressure and Credit
    Semantics".
    """

    def __init__(self, initial: int, dispatch: _Dispatcher, c2e_sid: str, validator: _StreamValidator) -> None:
        self._credit = initial
        self._unbounded = initial <= 0
        self._dispatch = dispatch
        self._c2e_sid = c2e_sid
        self._validator = validator

    def take(self, want: int) -> int:
        if self._unbounded:
            return want

        while self._credit <= 0:
            self._await_grant()

        take = min(want, self._credit)
        self._credit -= take
        return take

    def _await_grant(self) -> None:
        deadline = time.monotonic() + FRAME_IDLE_TIMEOUT
        msg = self._dispatch.recv_matching(lambda m: m.sid == self._c2e_sid, deadline)
        if msg is None:
            raise ProtocolError("timed out waiting for download CreditFrame")

        env = protocol.unmarshal_envelope(msg.data)
        frame = env.stream_frame
        outcome = self._validator.accept(frame)
        if outcome == "duplicate":
            return

        which = frame.WhichOneof("payload")
        if which == "credit":
            self._credit += frame.credit.download_credit_bytes
        elif which == "cancel":
            raise ProtocolError("request cancelled while waiting for download credit")
        # other frame types (e.g. late request-body data) are accepted by
        # the validator and otherwise ignored here.
