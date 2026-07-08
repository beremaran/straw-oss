"""Python Egress SDK protocol foundation.

Lets a custom Python worker build wire-compatible registration/heartbeat
requests and NATS subjects for the Straw Core NATS protocol
(``docs/planning/12-nats-protocol.md``). The assignment runtime (accepting
and serving one decoded HTTP assignment) is added by
``docs/tasks/p2/32b-python-egress-sdk-assignment-runtime.md``.
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
    log_telemetry_subject,
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

__all__ = [
    "DIRECTION_CONTROL_TO_EXECUTOR",
    "DIRECTION_EXECUTOR_TO_CONTROL",
    "PROTOCOL_MAJOR",
    "Capabilities",
    "Identity",
    "NATSClient",
    "NATSMessage",
    "NATSProtocolError",
    "PoolRef",
    "SubjectTokenError",
    "assignment_subject",
    "build_heartbeat",
    "build_register_request",
    "control_inbox_prefix",
    "heartbeat_envelope",
    "heartbeat_subject",
    "log_telemetry_subject",
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
