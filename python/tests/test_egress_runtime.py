import os
import socket
import sys
import threading
import time
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from straw.egress import Capabilities, Identity, NATSClient  # noqa: E402
from straw.egress.protocol import DIRECTION_CONTROL_TO_EXECUTOR, DIRECTION_EXECUTOR_TO_CONTROL  # noqa: E402
from straw.egress.runtime import DecodedRequest, DecodedResponse, Runtime  # noqa: E402
from straw.egress import protocol  # noqa: E402
from straw.proto.straw.v1 import straw_pb2 as pb  # noqa: E402


def _identity(worker_id="worker-1") -> Identity:
    return Identity(worker_id=worker_id, credential_id="cred-1", executor_type="http", private_key=os.urandom(32))


class _FakeControl:
    """A scripted loopback Core NATS server. Parses SUB/PUB/UNSUB lines off
    the wire into an ordered record list (so tests can assert ordering, e.g.
    "subscribed before AssignAck was sent") and lets the test push MSG
    frames to the client on demand.
    """

    def __init__(self):
        listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        listener.bind(("127.0.0.1", 0))
        listener.listen(1)
        self.port = listener.getsockname()[1]
        self._listener = listener
        self.conn = None
        self.records = []
        self._lock = threading.Lock()
        accept_thread = threading.Thread(target=self._accept, daemon=True)
        accept_thread.start()

    def _accept(self):
        conn, _ = self._listener.accept()
        self.conn = conn
        conn.sendall(b'INFO {"server_id":"fake","max_payload":1048576}\r\n')
        leftover = self._wait_for_connect()
        reader_thread = threading.Thread(target=self._read_loop, args=(leftover,), daemon=True)
        reader_thread.start()

    def _wait_for_connect(self):
        buf = b""
        while b"\r\n" not in buf:
            chunk = self.conn.recv(65536)
            if not chunk:
                raise AssertionError("client closed before CONNECT")
            buf += chunk
        line, rest = buf.split(b"\r\n", 1)
        assert line.startswith(b"CONNECT ")
        return rest

    def _read_loop(self, leftover):
        try:
            self._read_loop_body(leftover)
        except OSError:
            return  # tearDown closed the socket while this daemon thread was blocked in recv()

    def _read_loop_body(self, leftover):
        buf = leftover

        def read_line():
            nonlocal buf
            while b"\r\n" not in buf:
                chunk = self.conn.recv(65536)
                if not chunk:
                    return None
                buf += chunk
            line, buf2 = buf.split(b"\r\n", 1)
            buf = buf2
            return line

        def read_exact(n):
            nonlocal buf
            while len(buf) < n:
                chunk = self.conn.recv(65536)
                if not chunk:
                    return None
                buf += chunk
            data, buf2 = buf[:n], buf[n:]
            buf = buf2
            return data

        while True:
            line = read_line()
            if line is None:
                return
            if line.startswith(b"PUB "):
                fields = line.split(b" ")
                if len(fields) == 3:
                    subject, reply_to, n = fields[1].decode(), "", int(fields[2])
                else:
                    subject, reply_to, n = fields[1].decode(), fields[2].decode(), int(fields[3])
                payload = read_exact(n)
                read_exact(2)
                self._append(("PUB", subject, reply_to, payload))
            elif line.startswith(b"SUB "):
                fields = line.split(b" ")
                subject = fields[1].decode()
                sid = fields[-1].decode()
                self._append(("SUB", subject, sid))
            elif line.startswith(b"UNSUB "):
                fields = line.split(b" ")
                self._append(("UNSUB", fields[1].decode()))
            elif line == b"PING":
                self.conn.sendall(b"PONG\r\n")
            elif line in (b"+OK", b"PONG"):
                continue

    def _append(self, record):
        with self._lock:
            self.records.append(record)

    def snapshot(self):
        with self._lock:
            return list(self.records)

    def wait_for(self, predicate, timeout=5.0):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            for record in self.snapshot():
                if predicate(record):
                    return record
            time.sleep(0.01)
        raise AssertionError(f"timed out waiting for a matching record; got {self.snapshot()}")

    def wait_for_frame_count(self, subject, count, timeout=5.0):
        """Waits until at least `count` StreamFrames have been published on
        `subject` (the reader thread parses PUB lines asynchronously, so
        checking self.records immediately after a client-side call races
        it)."""
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            frames = _envelope_frames(self.snapshot(), subject)
            if len(frames) >= count:
                return frames
            time.sleep(0.01)
        raise AssertionError(f"timed out waiting for {count} frames on {subject}; got {_envelope_frames(self.snapshot(), subject)}")

    def sid_for_subject(self, subject, timeout=5.0):
        record = self.wait_for(lambda r: r[0] == "SUB" and r[1] == subject, timeout)
        return record[2]

    def send_msg(self, subject, sid, payload, reply_to=""):
        if reply_to:
            header = f"MSG {subject} {sid} {reply_to} {len(payload)}\r\n".encode()
        else:
            header = f"MSG {subject} {sid} {len(payload)}\r\n".encode()
        self.conn.sendall(header + payload + b"\r\n")

    def close(self):
        if self.conn:
            self.conn.close()
        self._listener.close()


def _envelope_frames(records, subject):
    out = []
    for record in records:
        if record[0] == "PUB" and record[1] == subject:
            env = protocol.unmarshal_envelope(record[3])
            out.append(env.stream_frame)
    return out


class RuntimeAssignmentTests(unittest.TestCase):
    def setUp(self):
        self.control = _FakeControl()
        self.client = NATSClient("127.0.0.1", self.control.port)
        self.identity = _identity()
        self.caps = Capabilities(max_concurrency=1)
        self.runtime = Runtime(self.client, self.identity, self.caps)

        register_thread = threading.Thread(target=self._do_register)
        register_thread.start()
        record = self.control.wait_for(lambda r: r[0] == "PUB" and r[1] == protocol.registration_subject())
        env = protocol.unmarshal_envelope(record[3])
        self.assertEqual(env.register_request.worker_id, "worker-1")
        ack = pb.Envelope(register_ack=pb.RegisterAck(ok=True, session_id="sess-1"))
        self.control.send_msg(record[2], self.control.sid_for_subject(f"{self.identity.inbox_prefix()}.>"), protocol.marshal_envelope(ack))
        register_thread.join(timeout=5.0)
        self.assertEqual(self.runtime.session_id, "sess-1")

    def _do_register(self):
        self.runtime.register()

    def tearDown(self):
        self.client.close()
        self.control.close()

    def _accept_assignment(self, worker, request_id, expected_upload_bytes, initial_upload_credit_bytes=1 << 20, initial_download_credit_bytes=1 << 20):
        """Sends an AssignRequest, waits for the c2e subscription and the
        AssignAck, and returns (c2e_subject, c2e_sid, e2c_subject, reply_to).
        """
        reply_to = "_INBOX.ctl.assign.1"
        env = pb.Envelope(
            request_id=request_id,
            deployment_id="tenant-1",
            deadline_unix_ms=0,
            protocol_major=protocol.PROTOCOL_MAJOR,
            attempt=1,
            assign_request=pb.AssignRequest(
                mode=pb.REQUEST_MODE_DECODED_HTTP,
                expected_upload_bytes=expected_upload_bytes,
                attempt=1,
                initial_upload_credit_bytes=initial_upload_credit_bytes,
                initial_download_credit_bytes=initial_download_credit_bytes,
            ),
        )
        assign_sid = self.control.sid_for_subject(protocol.assignment_subject("worker-1", "sess-1"))
        self.control.send_msg(protocol.assignment_subject("worker-1", "sess-1"), assign_sid, protocol.marshal_envelope(env), reply_to=reply_to)

        c2e_subject = protocol.stream_subject(request_id, "worker-1", "sess-1", DIRECTION_CONTROL_TO_EXECUTOR)
        e2c_subject = protocol.stream_subject(request_id, "worker-1", "sess-1", DIRECTION_EXECUTOR_TO_CONTROL)
        c2e_sub_record = self.control.wait_for(lambda r: r[0] == "SUB" and r[1] == c2e_subject)

        # The AssignAck must not appear before the c2e subscription: that is
        # the subscription-before-ack ordering rule this test proves.
        ack_record = self.control.wait_for(lambda r: r[0] == "PUB" and r[1] == reply_to)
        self.assertLess(self.control.records.index(c2e_sub_record), self.control.records.index(ack_record))
        ack_env = protocol.unmarshal_envelope(ack_record[3])
        self.assertEqual(ack_env.assign_ack.code, pb.ASSIGN_ACK_ACCEPTED)

        return c2e_subject, c2e_sub_record[2], e2c_subject

    def _send_c2e(self, subject, sid, seq, attempt, **frame_kwargs):
        frame = pb.StreamFrame(stream_seq=seq, attempt=attempt, **frame_kwargs)
        env = pb.Envelope(request_id="req-1", deployment_id="tenant-1", attempt=attempt, stream_frame=frame)
        self.control.send_msg(subject, sid, protocol.marshal_envelope(env))

    def test_streams_decoded_response_without_buffering(self):
        chunks_published_before_second_yield = []

        def body():
            yield b"hello "
            # Prove the runtime already published the first chunk instead of
            # buffering the whole response before sending anything: block
            # until it shows up on the wire (response_start + first data).
            e2c = protocol.stream_subject("req-1", "worker-1", "sess-1", DIRECTION_EXECUTOR_TO_CONTROL)
            frames = self.control.wait_for_frame_count(e2c, 2)
            chunks_published_before_second_yield.append([f.data.data for f in frames if f.WhichOneof("payload") == "data"])
            yield b"world"

        def executor(request: DecodedRequest) -> DecodedResponse:
            self.assertEqual(request.method, "GET")
            self.assertEqual(request.body, b"body")
            return DecodedResponse(status=200, headers=[("X-Test", b"1")], body=body())

        worker = self.runtime.worker(executor)
        serve_thread = threading.Thread(target=worker.serve_one, kwargs={"timeout": 5.0})
        serve_thread.start()

        c2e_subject, c2e_sid, e2c_subject = self._accept_assignment(worker, "req-1", expected_upload_bytes=4)
        self._send_c2e(c2e_subject, c2e_sid, 1, 1, request_start=pb.RequestStart(mode=pb.REQUEST_MODE_DECODED_HTTP, method="GET", url="http://example.test/"))
        self._send_c2e(c2e_subject, c2e_sid, 2, 1, data=pb.DataFrame(offset=0, data=b"body"))

        serve_thread.join(timeout=5.0)
        self.assertFalse(serve_thread.is_alive())

        self.assertEqual(chunks_published_before_second_yield, [[b"hello "]])

        frames = self.control.wait_for_frame_count(e2c_subject, 4)
        kinds = [f.WhichOneof("payload") for f in frames]
        self.assertEqual(kinds, ["response_start", "data", "data", "end"])
        self.assertEqual(frames[0].response_start.status, 200)
        self.assertEqual(frames[1].data.data, b"hello ")
        self.assertEqual(frames[2].data.data, b"world")
        self.assertTrue(frames[3].end.success)
        self.assertEqual([f.stream_seq for f in frames], [1, 2, 3, 4])

    def test_executor_error_maps_to_error_frame(self):
        def executor(request: DecodedRequest) -> DecodedResponse:
            raise ValueError("boom")

        worker = self.runtime.worker(executor)
        serve_thread = threading.Thread(target=worker.serve_one, kwargs={"timeout": 5.0})
        serve_thread.start()

        c2e_subject, c2e_sid, e2c_subject = self._accept_assignment(worker, "req-1", expected_upload_bytes=0)
        self._send_c2e(c2e_subject, c2e_sid, 1, 1, request_start=pb.RequestStart(mode=pb.REQUEST_MODE_DECODED_HTTP, method="GET", url="http://example.test/"))

        serve_thread.join(timeout=5.0)
        self.assertFalse(serve_thread.is_alive())

        frames = self.control.wait_for_frame_count(e2c_subject, 1)
        self.assertEqual(len(frames), 1)
        self.assertEqual(frames[0].WhichOneof("payload"), "error")
        self.assertEqual(frames[0].error.code, pb.ERROR_CODE_EXECUTOR_INTERNAL_ERROR)
        self.assertIn("boom", frames[0].error.message)

    def test_credit_backpressure_stops_and_resumes(self):
        def executor(request: DecodedRequest) -> DecodedResponse:
            return DecodedResponse(status=200, body=[b"hello world"])

        worker = self.runtime.worker(executor)
        serve_thread = threading.Thread(target=worker.serve_one, kwargs={"timeout": 5.0})
        serve_thread.start()

        c2e_subject, c2e_sid, e2c_subject = self._accept_assignment(
            worker, "req-1", expected_upload_bytes=0, initial_download_credit_bytes=5
        )
        self._send_c2e(c2e_subject, c2e_sid, 1, 1, request_start=pb.RequestStart(mode=pb.REQUEST_MODE_DECODED_HTTP, method="GET", url="http://example.test/"))

        # Only 5 bytes of credit: the runtime must send "hello" and then stop.
        frames = self.control.wait_for_frame_count(e2c_subject, 2)
        kinds = [f.WhichOneof("payload") for f in frames]
        self.assertEqual(kinds, ["response_start", "data"])
        self.assertEqual(frames[1].data.data, b"hello")
        time.sleep(0.2)
        self.assertTrue(serve_thread.is_alive(), "runtime must still be blocked on download credit")
        frames = _envelope_frames(self.control.snapshot(), e2c_subject)
        self.assertEqual(len(frames), 2, "no further DataFrame should be sent until credit is granted")

        # Grant more credit; the runtime should resume and send the rest, then EndFrame.
        self._send_c2e(c2e_subject, c2e_sid, 2, 1, credit=pb.CreditFrame(download_credit_bytes=6))

        serve_thread.join(timeout=5.0)
        self.assertFalse(serve_thread.is_alive())

        frames = self.control.wait_for_frame_count(e2c_subject, 4)
        kinds = [f.WhichOneof("payload") for f in frames]
        self.assertEqual(kinds, ["response_start", "data", "data", "end"])
        self.assertEqual(frames[1].data.data, b"hello")
        self.assertEqual(frames[2].data.data, b" world")


if __name__ == "__main__":
    unittest.main()
