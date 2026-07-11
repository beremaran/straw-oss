"""Python Egress SDK: protocol foundation plus the assignment runtime.

Lets a custom Python worker build wire-compatible registration/heartbeat
requests and NATS subjects for the Straw Core NATS protocol
(``docs/public/architecture.md``), and run a worker that registers,
heartbeats, and serves decoded-HTTP assignments (``runtime.py``).
"""

from .natsclient import NATSClient, NATSMessage, NATSProtocolError
from .protocol import (
    DIRECTION_CONTROL_TO_EXECUTOR,
    DIRECTION_EXECUTOR_TO_CONTROL,
    PROTOCOL_MAJOR,
    Capabilities,
    Identity,
    PoolRef,
    SubjectTokenError,
    assignment_subject,
    build_heartbeat,
    build_register_request,
    control_inbox_prefix,
    heartbeat_envelope,
    heartbeat_subject,
    marshal_envelope,
    register_envelope,
    registration_signing_payload,
    registration_subject,
    sign_registration,
    stream_subject,
    unmarshal_envelope,
    validate_subject_token,
    verify_registration_signature,
    worker_inbox_prefix,
)
from .runtime import (
    DecodedRequest,
    DecodedResponse,
    ProtocolError,
    RegistrationError,
    Runtime,
    Worker,
)

__all__ = [
    "DIRECTION_CONTROL_TO_EXECUTOR",
    "DIRECTION_EXECUTOR_TO_CONTROL",
    "PROTOCOL_MAJOR",
    "Capabilities",
    "DecodedRequest",
    "DecodedResponse",
    "Identity",
    "NATSClient",
    "NATSMessage",
    "NATSProtocolError",
    "PoolRef",
    "ProtocolError",
    "RegistrationError",
    "Runtime",
    "SubjectTokenError",
    "Worker",
    "assignment_subject",
    "build_heartbeat",
    "build_register_request",
    "control_inbox_prefix",
    "heartbeat_envelope",
    "heartbeat_subject",
    "marshal_envelope",
    "register_envelope",
    "registration_signing_payload",
    "registration_subject",
    "sign_registration",
    "stream_subject",
    "unmarshal_envelope",
    "validate_subject_token",
    "verify_registration_signature",
    "worker_inbox_prefix",
]
