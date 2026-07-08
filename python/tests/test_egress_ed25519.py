import os
import sys
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from straw.egress import _ed25519  # noqa: E402

# Fixture generated once with `cryptography`'s OpenSSL-backed Ed25519
# (verified separately against Go's crypto/ed25519 during development) so
# this pure-Python implementation is checked against a known-correct,
# independent implementation rather than only against itself.
_SEED = bytes.fromhex("ce00e54b2ba5cabe26e5a8f94f884b1d21d6aa70bdc5eb8e4a5ce0293719e31e")
_MESSAGE = b"the quick brown fox"
_PUB = bytes.fromhex("657bfd2a45ccb905f426418ce5bddb081636bad3e419dcdcdcb421f7eace8d3f")
_SIG = bytes.fromhex("8d667c8edcdf1716ddc3e125e27eb28c5488c806de1b99fa4c8d5157ea3c47f42240e93cccff5fb5acf231ac2e3a1b7caa91c09c91b8f0db9559eb1eaccc920d")


class KnownVectorTests(unittest.TestCase):
    def test_known_vector(self):
        self.assertEqual(_ed25519.public_key(_SEED), _PUB)
        sig = _ed25519.sign(_SEED, _MESSAGE)
        self.assertEqual(sig, _SIG)
        self.assertTrue(_ed25519.verify(_PUB, _MESSAGE, sig))

    def test_verify_rejects_tampered_message(self):
        sig = _ed25519.sign(_SEED, _MESSAGE)
        self.assertFalse(_ed25519.verify(_PUB, _MESSAGE + b"x", sig))

    def test_sign_verify_round_trip_random(self):
        seed = os.urandom(32)
        pub = _ed25519.public_key(seed)
        msg = os.urandom(128)
        sig = _ed25519.sign(seed, msg)
        self.assertTrue(_ed25519.verify(pub, msg, sig))
        self.assertFalse(_ed25519.verify(pub, msg, bytes(64)))


if __name__ == "__main__":
    unittest.main()
