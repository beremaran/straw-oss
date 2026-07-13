import json
from importlib.metadata import version
from pathlib import Path
import unittest

import straw
from straw.egress import protocol
from straw_protos.signing import registration_signing_payload
from straw_protos.straw.v1 import straw_pb2 as pb


class TaggedPythonSDKIntegrationTests(unittest.TestCase):
    def test_exact_tagged_distributions_are_installed(self):
        self.assertEqual(version("straw-sdk"), "0.1.0")
        self.assertEqual(version("straw-protos"), "0.3.0")
        self.assertTrue(hasattr(straw, "Client"))

    def test_sdk_and_binding_share_current_wire_contract(self):
        fixture_path = Path(__file__).parents[2] / "conformance" / "fixtures" / "v1" / "envelope.json"
        fixture = json.loads(fixture_path.read_text())
        envelope = pb.Envelope.FromString(bytes.fromhex(fixture["unknown_field_wire_hex"]))
        self.assertEqual(envelope.request_id, fixture["request_id"])
        self.assertEqual(envelope.deployment_id, fixture["deployment_id"])
        self.assertEqual(protocol.PROTOCOL_MAJOR, fixture["protocol_major"])
        self.assertEqual(envelope.SerializeToString(deterministic=True), bytes.fromhex(fixture["unknown_field_wire_hex"]))

    def test_sdk_uses_binding_signing_helper(self):
        request = pb.RegisterRequest(worker_id="worker", credential_id="credential", executor_type="http")
        self.assertEqual(protocol.registration_signing_payload(request), registration_signing_payload(request))


if __name__ == "__main__":
    unittest.main()
