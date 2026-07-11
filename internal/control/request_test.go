package control

import (
	"errors"
	"testing"
)

func TestValidateRequestRejectsDuplicateFingerprintProfileMembers(t *testing.T) {
	t.Parallel()

	_, err := ValidateRequest([]byte(`{
		"method":"GET",
		"url":"https://example.com/",
		"fingerprint_profile":"chrome_120",
		"fingerprint_profile":"default",
		"timeout_ms":5000
	}`), 1<<20, 5000)
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != errorCodeInvalidRequest {
		t.Fatalf("ValidateRequest() error = %#v, want invalid_request for duplicate fingerprint_profile members", err)
	}
}

func TestValidateRequestRejectsLiteralBaselineFingerprintProfile(t *testing.T) {
	t.Parallel()

	_, err := ValidateRequest([]byte(`{
		"method":"GET",
		"url":"https://example.com/",
		"fingerprint_profile":"baseline",
		"timeout_ms":5000
	}`), 1<<20, 5000)
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != errorCodeUnsupportedFingerprint {
		t.Fatalf("ValidateRequest() error = %#v, want unsupported_fingerprint for literal baseline", err)
	}
}

func TestValidateRequestRejectsMalformedUTF8FingerprintProfile(t *testing.T) {
	t.Parallel()

	raw := append([]byte(`{"method":"GET","url":"https://example.com/","fingerprint_profile":"`), 0xff)
	raw = append(raw, []byte(`","timeout_ms":5000}`)...)

	_, err := ValidateRequest(raw, 1<<20, 5000)
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != errorCodeInvalidRequest {
		t.Fatalf("ValidateRequest() error = %#v, want invalid_request for malformed UTF-8 fingerprint_profile", err)
	}
}

func TestValidateRequestAllowsDefaultFingerprintAlias(t *testing.T) {
	t.Parallel()

	req, err := ValidateRequest([]byte(`{
		"method":"GET",
		"url":"https://example.com/",
		"fingerprint_profile":"default",
		"timeout_ms":5000
	}`), 1<<20, 5000)
	if err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
	if req.Fingerprint != defaultFingerprintProfileName {
		t.Fatalf("validated fingerprint = %q, want %q", req.Fingerprint, defaultFingerprintProfileName)
	}
}
