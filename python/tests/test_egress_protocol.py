import os
import sys
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from straw.egress import (  # noqa: E402
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
    registration_subject,
    stream_subject,
    unmarshal_envelope,
    validate_subject_token,
    verify_registration_signature,
    worker_inbox_prefix,
)
from straw.egress import _ed25519
from straw.proto.straw.v1 import straw_pb2 as pb


def _identity(worker_id="worker-1") -> Identity:
    seed = os.urandom(32)
    return Identity(worker_id=worker_id, credential_id="cred-1", executor_type="http", private_key=seed)


class SubjectTokenTests(unittest.TestCase):
    def test_rejects_empty(self):
        with self.assertRaises(SubjectTokenError):
            validate_subject_token("")

    def test_rejects_dot(self):
        with self.assertRaises(SubjectTokenError):
            validate_subject_token("worker.1")

    def test_accepts_letters_digits_dash_underscore(self):
        validate_subject_token("worker-1_ABC9")


class CanonicalSubjectTests(unittest.TestCase):
    def test_control_subjects(self):
        self.assertEqual(registration_subject(), "straw.v1.control.register")
        self.assertEqual(heartbeat_subject(), "straw.v1.control.heartbeat")
        self.assertEqual(log_telemetry_subject(), "straw.v1.control.logs")
        self.assertEqual(control_inbox_prefix(), "_INBOX.ctl")

    def test_worker_inbox_prefix(self):
        self.assertEqual(worker_inbox_prefix("worker-1"), "_INBOX.wrk.worker-1")
        with self.assertRaises(SubjectTokenError):
            worker_inbox_prefix("worker.1")

    def test_assignment_subject(self):
        self.assertEqual(
            assignment_subject("worker-1", "sess-1"),
            "straw.v1.executor.worker-1.sess-1.assign",
        )

    def test_stream_subjects(self):
        self.assertEqual(
            stream_subject("req-1", "worker-1", "sess-1", "c2e"),
            "straw.v1.req.req-1.worker-1.sess-1.c2e",
        )
        self.assertEqual(
            stream_subject("req-1", "worker-1", "sess-1", "e2c"),
            "straw.v1.req.req-1.worker-1.sess-1.e2c",
        )

    def test_stream_subject_rejects_bad_direction(self):
        with self.assertRaises(ValueError):
            stream_subject("req-1", "worker-1", "sess-1", "sideways")

    def test_stream_subject_rejects_unsafe_token(self):
        with self.assertRaises(SubjectTokenError):
            stream_subject("req.1", "worker-1", "sess-1", "c2e")


class RegistrationSigningTests(unittest.TestCase):
    def test_register_request_is_signed_and_verifiable(self):
        identity = _identity()
        caps = Capabilities(tags=["pool-a"], max_concurrency=4, software_version="0.1.0")

        req = build_register_request(identity, caps)

        self.assertEqual(req.worker_id, "worker-1")
        self.assertEqual(req.credential_id, "cred-1")
        self.assertEqual(req.protocol_major, 1)
        self.assertTrue(req.signed_token)
        self.assertTrue(req.nonce)

        pub = _ed25519.public_key(identity.private_key)
        self.assertTrue(verify_registration_signature(pub, req, req.signed_token))

    def test_tampered_request_fails_verification(self):
        identity = _identity()
        req = build_register_request(identity, Capabilities())
        pub = _ed25519.public_key(identity.private_key)

        req.worker_id = "someone-else"

        self.assertFalse(verify_registration_signature(pub, req, req.signed_token))

    def test_pool_refs_carried_through(self):
        identity = _identity()
        caps = Capabilities(allowed_pools=[PoolRef(tenant_id="t1", pool_id="p1")])

        req = build_register_request(identity, caps)

        self.assertEqual(len(req.allowed_pools), 1)
        self.assertEqual(req.allowed_pools[0].tenant_id, "t1")
        self.assertEqual(req.allowed_pools[0].pool_id, "p1")


class HeartbeatTests(unittest.TestCase):
    def test_build_heartbeat_fields(self):
        identity = _identity()
        hb = build_heartbeat(
            identity,
            session_id="sess-1",
            health=pb.WorkerHealth.WORKER_HEALTH_READY,
            active_requests=2,
            available_capacity=3,
            max_concurrency=5,
            draining=False,
        )
        self.assertEqual(hb.worker_id, "worker-1")
        self.assertEqual(hb.session_id, "sess-1")
        self.assertEqual(hb.active_requests, 2)
        self.assertEqual(hb.max_concurrency, 5)
        self.assertGreater(hb.worker_timestamp_ms, 0)


class EnvelopeRoundTripTests(unittest.TestCase):
    def test_register_envelope_round_trip(self):
        identity = _identity()
        req = build_register_request(identity, Capabilities())
        env = register_envelope(req)

        raw = marshal_envelope(env)
        decoded = unmarshal_envelope(raw)

        self.assertEqual(decoded.protocol_major, 1)
        self.assertEqual(decoded.register_request.worker_id, "worker-1")
        self.assertEqual(decoded.register_request.signed_token, req.signed_token)

    def test_heartbeat_envelope_round_trip(self):
        identity = _identity()
        hb = build_heartbeat(
            identity, "sess-1", pb.WorkerHealth.WORKER_HEALTH_READY, 0, 5, 5, False
        )
        env = heartbeat_envelope(hb)

        decoded = unmarshal_envelope(marshal_envelope(env))

        self.assertEqual(decoded.heartbeat_request.session_id, "sess-1")

    def test_stream_frame_and_assign_request_round_trip(self):
        assign = pb.AssignRequest(
            mode=pb.RequestMode.REQUEST_MODE_DECODED_HTTP,
            deadline_unix_ms=123456,
            initial_upload_credit_bytes=8 * 1024 * 1024,
            initial_download_credit_bytes=8 * 1024 * 1024,
            max_inflight_upload_bytes=16 * 1024 * 1024,
            max_inflight_download_bytes=16 * 1024 * 1024,
            attempt=1,
        )
        env = pb.Envelope(
            request_id="req-1",
            tenant_id="tenant-1",
            deadline_unix_ms=123456,
            protocol_major=1,
            attempt=1,
            assign_request=assign,
        )

        decoded = unmarshal_envelope(marshal_envelope(env))

        self.assertEqual(decoded.request_id, "req-1")
        self.assertEqual(decoded.assign_request.initial_upload_credit_bytes, 8 * 1024 * 1024)

        frame = pb.StreamFrame(
            stream_seq=1,
            attempt=1,
            data=pb.DataFrame(offset=0, data=b"hello"),
        )
        frame_env = pb.Envelope(request_id="req-1", tenant_id="tenant-1", attempt=1, stream_frame=frame)
        decoded_frame = unmarshal_envelope(marshal_envelope(frame_env))

        self.assertEqual(decoded_frame.stream_frame.stream_seq, 1)
        self.assertEqual(decoded_frame.stream_frame.data.data, b"hello")


if __name__ == "__main__":
    unittest.main()
