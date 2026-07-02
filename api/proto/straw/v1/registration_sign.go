// Package strawpb contains the Straw protobuf API.
package strawpb

import (
	"crypto/ed25519"
)

// registrationSigningDomain is a fixed domain-separation prefix so a
// registration signature can never be replayed as a signature over some
// other Straw message type.
const registrationSigningDomain = "straw.v1.register\n"

// RegistrationSigningPayload returns the canonical bytes a worker signs to
// prove possession of the private key bound to its worker credential. The
// payload binds the worker identity, credential, executor type, and protocol
// version so a signature captured for one worker cannot authorize another.
//
// P0 has no per-registration nonce channel (Core NATS request/reply only),
// so this signature proves credential possession but is not replay-proof on
// its own; NATS subject ACLs and credential status remain the outer
// controls. See docs/planning/11-worker-discovery-and-health.md.
func RegistrationSigningPayload(req *RegisterRequest) []byte {
	if req == nil {
		return []byte(registrationSigningDomain)
	}
	// Length-prefixing is unnecessary because the fields are joined by a
	// byte ('\n') that is rejected in worker_id / credential_id subject
	// tokens; executor_type and the protocol digits cannot contain it
	// either, so the concatenation is unambiguous.
	var b []byte

	b = append(b, registrationSigningDomain...)
	b = append(b, req.GetWorkerId()...)
	b = append(b, '\n')
	b = append(b, req.GetCredentialId()...)
	b = append(b, '\n')
	b = append(b, req.GetExecutorType()...)
	b = append(b, '\n')
	b = appendUint32(b, req.GetProtocolMajor())
	b = append(b, '.')
	b = appendUint32(b, req.GetProtocolMinor())

	return b
}

// SignRegistration produces the signed token for req using priv. The caller
// places the result in RegisterRequest.SignedToken.
func SignRegistration(priv ed25519.PrivateKey, req *RegisterRequest) []byte {
	return ed25519.Sign(priv, RegistrationSigningPayload(req))
}

// VerifyRegistrationSignature reports whether signedToken is a valid ed25519
// signature over req's canonical payload for the given public key.
func VerifyRegistrationSignature(pub ed25519.PublicKey, req *RegisterRequest, signedToken []byte) bool {
	if len(pub) != ed25519.PublicKeySize {
		return false
	}

	return ed25519.Verify(pub, RegistrationSigningPayload(req), signedToken)
}

func appendUint32(b []byte, v uint32) []byte {
	if v == 0 {
		return append(b, '0')
	}

	var tmp [10]byte

	i := len(tmp)
	for v > 0 {
		i--
		tmp[i] = byte('0' + v%10)
		v /= 10
	}

	return append(b, tmp[i:]...)
}
