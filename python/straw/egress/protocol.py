"""Wire-compatible protocol layer for the Python Egress SDK.

Mirrors the Go SDK's public contract (``sdk/egress/types.go``,
``api/proto/straw/v1/registration_sign.go``): subject construction, safe-token
validation, Envelope construction, and registration/heartbeat signing. See
``docs/public/architecture.md`` for the wire protocol this module
implements.
"""

from __future__ import annotations

import os
import time
from dataclasses import dataclass, field
from typing import List, Optional

from straw.egress import _ed25519
from straw.proto.straw.v1 import straw_pb2 as pb

# ProtocolMajor is the worker protocol major version this SDK speaks.
PROTOCOL_MAJOR = 1

_REGISTRATION_NONCE_BYTES = 16
_REGISTRATION_SIGNING_DOMAIN = b"straw.v1.register\n"

DIRECTION_CONTROL_TO_EXECUTOR = "c2e"
DIRECTION_EXECUTOR_TO_CONTROL = "e2c"
_STREAM_DIRECTIONS = (DIRECTION_CONTROL_TO_EXECUTOR, DIRECTION_EXECUTOR_TO_CONTROL)


class SubjectTokenError(ValueError):
    """Raised when a NATS subject token is empty or unsafe."""


def validate_subject_token(token: str) -> None:
    """Rejects empty tokens and any character outside [A-Za-z0-9_-],
    matching the Go SDK's ``ValidateSubjectToken``.
    """
    if not token:
        raise SubjectTokenError("subject token is required")
    for ch in token:
        if not (ch.isalnum() or ch in "-_"):
            raise SubjectTokenError(f"subject token contains unsafe character: {ch!r}")


def registration_subject() -> str:
    return "straw.v1.control.register"


def heartbeat_subject() -> str:
    return "straw.v1.control.heartbeat"


def control_inbox_prefix() -> str:
    return "_INBOX.ctl"


def worker_inbox_prefix(worker_id: str) -> str:
    validate_subject_token(worker_id)
    return f"_INBOX.wrk.{worker_id}"


def assignment_subject(worker_id: str, session_id: str) -> str:
    validate_subject_token(worker_id)
    validate_subject_token(session_id)
    return f"straw.v1.executor.{worker_id}.{session_id}.assign"


def stream_subject(request_id: str, worker_id: str, session_id: str, direction: str) -> str:
    validate_subject_token(request_id)
    validate_subject_token(worker_id)
    validate_subject_token(session_id)
    if direction not in _STREAM_DIRECTIONS:
        raise ValueError(f"unsupported stream direction: {direction!r}")
    return f"straw.v1.req.{request_id}.{worker_id}.{session_id}.{direction}"


@dataclass
class Identity:
    """The stable identity a worker registers with. ``private_key`` is the
    32-byte Ed25519 seed (matching ``ed25519.PrivateKey.Seed()`` in Go).
    """

    worker_id: str
    credential_id: str
    executor_type: str
    private_key: bytes

    def inbox_prefix(self) -> str:
        return worker_inbox_prefix(self.worker_id)


@dataclass
class PoolRef:
    deployment_id: str
    pool_id: str


@dataclass
class Capabilities:
    allowed_pools: List[PoolRef] = field(default_factory=list)
    tags: List[str] = field(default_factory=list)
    countries: List[str] = field(default_factory=list)
    regions: List[str] = field(default_factory=list)
    ip_types: List[str] = field(default_factory=list)
    supported_ingress_modes: List[str] = field(default_factory=list)
    max_concurrency: int = 0
    software_version: str = ""
    initial_draining: bool = False


def _now_unix_ms() -> int:
    return int(time.time() * 1000)


def registration_signing_payload(req: "pb.RegisterRequest") -> bytes:
    """Canonical bytes a worker signs to prove key possession, byte-identical
    to ``api/proto/straw/v1/registration_sign.go``'s ``RegistrationSigningPayload``.
    """
    parts = [
        _REGISTRATION_SIGNING_DOMAIN,
        req.worker_id.encode(),
        b"\n",
        req.credential_id.encode(),
        b"\n",
        req.executor_type.encode(),
        b"\n",
        str(req.protocol_major).encode(),
        b".",
        str(req.protocol_minor).encode(),
        b"\n",
        str(len(req.nonce)).encode(),
        b":",
        req.nonce,
        b"\n",
        str(req.issued_at_unix_ms).encode(),
    ]
    return b"".join(parts)


def sign_registration(private_key: bytes, req: "pb.RegisterRequest") -> bytes:
    return _ed25519.sign(private_key, registration_signing_payload(req))


def verify_registration_signature(public_key: bytes, req: "pb.RegisterRequest", signed_token: bytes) -> bool:
    if len(public_key) != 32:
        return False
    return _ed25519.verify(public_key, registration_signing_payload(req), signed_token)


def build_register_request(identity: Identity, caps: Optional[Capabilities] = None) -> "pb.RegisterRequest":
    """Assembles and signs a RegisterRequest, mirroring the Go SDK's
    ``BuildRegisterRequest``.
    """
    caps = caps or Capabilities()
    req = pb.RegisterRequest(
        worker_id=identity.worker_id,
        executor_type=identity.executor_type,
        credential_id=identity.credential_id,
        protocol_major=PROTOCOL_MAJOR,
        protocol_minor=0,
        software_version=caps.software_version,
        allowed_pools=[
            pb.RegisterRequest.PoolRef(tenant_id=p.deployment_id, pool_id=p.pool_id)
            for p in caps.allowed_pools
        ],
        tags=caps.tags,
        countries=caps.countries,
        regions=caps.regions,
        ip_types=caps.ip_types,
        supported_ingress_modes=caps.supported_ingress_modes,
        max_concurrency=caps.max_concurrency,
        initial_draining=caps.initial_draining,
        nonce=os.urandom(_REGISTRATION_NONCE_BYTES),
        issued_at_unix_ms=_now_unix_ms(),
    )
    req.signed_token = sign_registration(identity.private_key, req)
    return req


def build_heartbeat(
    identity: Identity,
    session_id: str,
    health: "pb.WorkerHealth",
    active_requests: int,
    available_capacity: int,
    max_concurrency: int,
    draining: bool,
) -> "pb.HeartbeatRequest":
    return pb.HeartbeatRequest(
        worker_id=identity.worker_id,
        session_id=session_id,
        health=health,
        active_requests=active_requests,
        available_capacity=available_capacity,
        max_concurrency=max_concurrency,
        draining=draining,
        worker_timestamp_ms=_now_unix_ms(),
    )


def register_envelope(req: "pb.RegisterRequest") -> "pb.Envelope":
    return pb.Envelope(protocol_major=PROTOCOL_MAJOR, protocol_minor=0, register_request=req)


def heartbeat_envelope(hb: "pb.HeartbeatRequest") -> "pb.Envelope":
    return pb.Envelope(protocol_major=PROTOCOL_MAJOR, protocol_minor=0, heartbeat_request=hb)


def marshal_envelope(env: "pb.Envelope") -> bytes:
    return env.SerializeToString()


def unmarshal_envelope(raw: bytes) -> "pb.Envelope":
    env = pb.Envelope()
    env.MergeFromString(raw)
    return env
