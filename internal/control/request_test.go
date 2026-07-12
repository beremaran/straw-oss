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

func TestValidateRequestRejectsRemovedEnterpriseHints(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string]string{
		"routing": `{"method":"GET","url":"https://example.com/","routing":{"country":"AU"}}`,
		"capture": `{"method":"GET","url":"https://example.com/","capture_hint":"all"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := ValidateRequest([]byte(raw), 1<<20, 5000)
			if err == nil {
				t.Fatal("ValidateRequest() error = nil, want unknown-field rejection")
			}
		})
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

func TestValidateRequestAcceptsReceiptBodyAndStoredResponseMode(t *testing.T) {
	t.Parallel()
	req, err := ValidateRequest([]byte(`{"method":"POST","url":"https://example.com/","body":{"mode":"receipt","receipt_id":"rcpt_1"},"response_body_mode":"receipt"}`), 4, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if req.BodyReceiptID != "rcpt_1" || req.ResponseBodyMode != bodyModeReceipt || len(req.BodyData) != 0 {
		t.Fatalf("request = %#v", req)
	}
}

func TestValidateRequestRejectsMixedReceiptAndInlineData(t *testing.T) {
	t.Parallel()
	_, err := ValidateRequest([]byte(`{"method":"POST","url":"https://example.com/","body":{"mode":"receipt","receipt_id":"rcpt_1","data_base64":"eA=="}}`), 4, 5000)
	if err == nil {
		t.Fatal("mixed receipt and inline body accepted")
	}
}
