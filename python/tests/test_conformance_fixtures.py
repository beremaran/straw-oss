import json
from pathlib import Path
import unittest

from straw.egress import _ed25519
from straw.egress.protocol import registration_signing_payload, sign_registration, verify_registration_signature
from straw.proto.straw.v1 import straw_pb2 as pb


FIXTURES = Path(__file__).parents[2] / "conformance" / "fixtures" / "v1"


class ConformanceFixtureTests(unittest.TestCase):
    def load(self, name):
        return json.loads((FIXTURES / name).read_text())

    def test_registration_signing(self):
        fixture = self.load("registration-signing.json")
        request = fixture["request"]
        req = pb.RegisterRequest(
            worker_id=request["worker_id"], credential_id=request["credential_id"],
            executor_type=request["executor_type"], protocol_major=request["protocol_major"],
            protocol_minor=request["protocol_minor"], nonce=bytes.fromhex(request["nonce_hex"]),
            issued_at_unix_ms=request["issued_at_unix_ms"],
            supported_fingerprint_profiles=request["supported_fingerprint_profiles"],
        )
        seed = bytes.fromhex(fixture["seed_hex"])
        self.assertEqual(registration_signing_payload(req), bytes.fromhex(fixture["payload_hex"]))
        self.assertEqual(_ed25519.public_key(seed), bytes.fromhex(fixture["public_key_hex"]))
        signature = sign_registration(seed, req)
        self.assertEqual(signature, bytes.fromhex(fixture["signature_hex"]))
        self.assertTrue(verify_registration_signature(_ed25519.public_key(seed), req, signature))

    def test_envelope_unknown_field_and_enum(self):
        fixture = self.load("envelope.json")
        env = pb.Envelope(
            request_id=fixture["request_id"], deployment_id=fixture["deployment_id"],
            protocol_major=fixture["protocol_major"], protocol_minor=fixture["protocol_minor"],
            attempt=fixture["attempt"], assign_ack=pb.AssignAck(code=pb.ASSIGN_ACK_ACCEPTED),
        )
        self.assertEqual(env.SerializeToString(deterministic=True), bytes.fromhex(fixture["deterministic_wire_hex"]))
        decoded = pb.Envelope.FromString(bytes.fromhex(fixture["unknown_field_wire_hex"]))
        self.assertEqual(decoded.SerializeToString(deterministic=True), bytes.fromhex(fixture["unknown_field_wire_hex"]))
        unknown_ack = pb.AssignAck(code=fixture["unknown_enum_number"])
        self.assertEqual(unknown_ack.code, fixture["unknown_enum_number"])


if __name__ == "__main__":
    unittest.main()
