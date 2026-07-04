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
// payload binds the worker identity, credential, executor type, protocol
// version, a per-registration nonce, and an issued-at timestamp so a
// signature captured for one worker cannot authorize another and a captured
// request cannot be replayed outside its nonce/skew window
// (docs/planning/27-security-controls.md "Worker Credential Signing"). The
// nonce travels inside the signed token itself, so Core NATS request/reply
// needs no extra out-of-band channel; Control enforces replay protection via
// a Redis-backed nonce store (see internal/control/worker_nonce.go).
func RegistrationSigningPayload(req *RegisterRequest) []byte {
	if req == nil {
		return []byte(registrationSigningDomain)
	}
	// Length-prefixing is unnecessary because the fields are joined by a
	// byte ('\n') that is rejected in worker_id / credential_id subject
	// tokens; executor_type, the protocol digits, and issued-at digits
	// cannot contain it either. The nonce is length-prefixed with its byte
	// count before the trailing issued-at field since raw nonce bytes could
	// otherwise contain '\n'.
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
	b = append(b, '\n')
	nonce := req.GetNonce()
	b = appendUint64(b, uint64(len(nonce)))
	b = append(b, ':')
	b = append(b, nonce...)
	b = append(b, '\n')
	b = appendInt64(b, req.GetIssuedAtUnixMs())

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
	return appendUint64(b, uint64(v))
}

// appendInt64 renders v in decimal, prefixing a '-' for negative values so
// the sign is unambiguous in the joined payload.
func appendInt64(b []byte, v int64) []byte {
	if v < 0 {
		b = append(b, '-')

		return appendUint64(b, uint64(-v))
	}

	return appendUint64(b, uint64(v))
}

func appendUint64(b []byte, v uint64) []byte {
	if v == 0 {
		return append(b, '0')
	}

	var tmp [20]byte

	i := len(tmp)
	for v > 0 {
		i--
		tmp[i] = byte('0' + v%10)
		v /= 10
	}

	return append(b, tmp[i:]...)
}
