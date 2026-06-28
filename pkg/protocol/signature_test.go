package protocol

import (
	"testing"
	"time"
)

func TestSign_Deterministic(t *testing.T) {
	data := []byte("test data")
	secret := []byte("test-secret")

	sig1 := Sign(data, secret)
	sig2 := Sign(data, secret)

	if sig1 != sig2 {
		t.Errorf("signatures should be deterministic: %s != %s", sig1, sig2)
	}
}

func TestSign_DifferentSecrets(t *testing.T) {
	data := []byte("test data")

	sig1 := Sign(data, []byte("secret-1"))
	sig2 := Sign(data, []byte("secret-2"))

	if sig1 == sig2 {
		t.Error("different secrets should produce different signatures")
	}
}

func TestSign_DifferentData(t *testing.T) {
	secret := []byte("test-secret")

	sig1 := Sign([]byte("data-1"), secret)
	sig2 := Sign([]byte("data-2"), secret)

	if sig1 == sig2 {
		t.Error("different data should produce different signatures")
	}
}

func TestVerify_ValidSignature(t *testing.T) {
	data := []byte("test data")
	secret := []byte("test-secret")

	signature := Sign(data, secret)

	if !Verify(data, signature, secret) {
		t.Error("valid signature should verify")
	}
}

func TestVerify_InvalidSignature(t *testing.T) {
	data := []byte("test data")
	secret := []byte("test-secret")

	if Verify(data, "invalid-signature", secret) {
		t.Error("invalid signature should not verify")
	}
}

func TestVerify_WrongSecret(t *testing.T) {
	data := []byte("test data")
	signature := Sign(data, []byte("correct-secret"))

	if Verify(data, signature, []byte("wrong-secret")) {
		t.Error("signature with wrong secret should not verify")
	}
}

func TestVerify_TamperedData(t *testing.T) {
	originalData := []byte("original data")
	secret := []byte("test-secret")

	signature := Sign(originalData, secret)

	if Verify([]byte("tampered data"), signature, secret) {
		t.Error("tampered data should not verify")
	}
}

func TestNewSignedTask_Success(t *testing.T) {
	req := &Request{
		ID:          "req-123",
		Method:      "GET",
		URL:         "https://example.com",
		Fingerprint: "chrome-130",
	}
	secret := []byte("test-secret")

	task, err := NewSignedTask(req, secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(task.Payload) == 0 {
		t.Error("payload should not be empty")
	}
	if task.Signature == "" {
		t.Error("signature should not be empty")
	}
	if task.Timestamp == 0 {
		t.Error("timestamp should not be zero")
	}

	age := time.Since(time.Unix(task.Timestamp, 0))
	if age > time.Second {
		t.Errorf("timestamp should be recent, got age: %v", age)
	}
}

func TestValidateSignedTask_Success(t *testing.T) {
	req := &Request{
		ID:          "req-123",
		Method:      "POST",
		URL:         "https://example.com/api",
		Headers:     HeaderMap{{Key: "Content-Type", Value: "application/json"}},
		Fingerprint: "chrome-130",
	}
	secret := []byte("test-secret")

	task, err := NewSignedTask(req, secret)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	decoded, err := ValidateSignedTask(task, secret, DefaultMaxTaskAge)
	if err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	if decoded.ID != req.ID {
		t.Errorf("ID mismatch: %s != %s", decoded.ID, req.ID)
	}
	if decoded.Method != req.Method {
		t.Errorf("Method mismatch")
	}
	if decoded.URL != req.URL {
		t.Errorf("URL mismatch")
	}
	if decoded.Fingerprint != req.Fingerprint {
		t.Errorf("Fingerprint mismatch")
	}
}

func TestValidateSignedTask_NilTask(t *testing.T) {
	_, err := ValidateSignedTask(nil, []byte("secret"), DefaultMaxTaskAge)
	if err == nil {
		t.Error("expected error for nil task")
	}
}

func TestValidateSignedTask_ExpiredTimestamp(t *testing.T) {
	req := &Request{
		ID:     "req-123",
		Method: "GET",
		URL:    "https://example.com",
	}
	secret := []byte("test-secret")

	task, err := NewSignedTask(req, secret)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	task.Timestamp = time.Now().Add(-2 * time.Minute).Unix()

	signatureData := append(task.Payload, []byte(string(rune(task.Timestamp)))...)
	task.Signature = Sign(signatureData, secret)

	_, err = ValidateSignedTask(task, secret, DefaultMaxTaskAge)
	if err == nil {
		t.Error("expected error for expired timestamp")
	}

	if validErr, ok := err.(*ValidationError); ok {
		if validErr.Code != ErrCodeReplayAttack {
			t.Errorf("expected REPLAY_ATTACK error, got: %s", validErr.Code)
		}
	}
}

func TestValidateSignedTask_InvalidSignature(t *testing.T) {
	req := &Request{
		ID:     "req-123",
		Method: "GET",
		URL:    "https://example.com",
	}
	secret := []byte("test-secret")

	task, err := NewSignedTask(req, secret)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	task.Signature = "tampered-signature"

	_, err = ValidateSignedTask(task, secret, DefaultMaxTaskAge)
	if err == nil {
		t.Error("expected error for invalid signature")
	}

	if validErr, ok := err.(*ValidationError); ok {
		if validErr.Code != ErrCodeSignatureInvalid {
			t.Errorf("expected SIGNATURE_INVALID error, got: %s", validErr.Code)
		}
	}
}

func TestValidateSignedTask_WrongSecret(t *testing.T) {
	req := &Request{
		ID:     "req-123",
		Method: "GET",
		URL:    "https://example.com",
	}

	task, err := NewSignedTask(req, []byte("correct-secret"))
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	_, err = ValidateSignedTask(task, []byte("wrong-secret"), DefaultMaxTaskAge)
	if err == nil {
		t.Error("expected error for wrong secret")
	}
}

func TestValidateSignedTask_TamperedPayload(t *testing.T) {
	req := &Request{
		ID:     "req-123",
		Method: "GET",
		URL:    "https://example.com",
	}
	secret := []byte("test-secret")

	task, err := NewSignedTask(req, secret)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	task.Payload = []byte("tampered-payload")

	_, err = ValidateSignedTask(task, secret, DefaultMaxTaskAge)
	if err == nil {
		t.Error("expected error for tampered payload")
	}
}

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Code:    ErrCodeSignatureInvalid,
		Message: "signature verification failed",
	}

	errorString := err.Error()
	if errorString != "SIGNATURE_INVALID: signature verification failed" {
		t.Errorf("unexpected error string: %s", errorString)
	}
}

func BenchmarkSign(b *testing.B) {
	data := []byte("benchmark data for signing")
	secret := []byte("benchmark-secret")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Sign(data, secret)
	}
}

func BenchmarkVerify(b *testing.B) {
	data := []byte("benchmark data for verification")
	secret := []byte("benchmark-secret")
	signature := Sign(data, secret)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Verify(data, signature, secret)
	}
}

func BenchmarkNewSignedTask(b *testing.B) {
	req := &Request{
		ID:      "req-benchmark",
		Method:  "GET",
		URL:     "https://benchmark.example.com/path",
		Headers: HeaderMap{{Key: "User-Agent", Value: "Benchmark/1.0"}},
	}
	secret := []byte("benchmark-secret")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewSignedTask(req, secret)
	}
}

func BenchmarkValidateSignedTask(b *testing.B) {
	req := &Request{
		ID:      "req-benchmark",
		Method:  "GET",
		URL:     "https://benchmark.example.com/path",
		Headers: HeaderMap{{Key: "User-Agent", Value: "Benchmark/1.0"}},
	}
	secret := []byte("benchmark-secret")
	task, _ := NewSignedTask(req, secret)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ValidateSignedTask(task, secret, DefaultMaxTaskAge)
	}
}
