package protocol

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// DefaultMaxTaskAge is the default maximum age for a SignedTask.
// Tasks older than this are rejected to prevent replay attacks.
const DefaultMaxTaskAge = 60 * time.Second

// Sign creates an HMAC-SHA256 signature for the given data.
// Returns the signature as a hex-encoded string.
func Sign(data []byte, secret []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// Verify checks if the provided signature matches the data.
// Uses constant-time comparison to prevent timing attacks.
func Verify(data []byte, signature string, secret []byte) bool {
	expected := Sign(data, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// NewSignedTask creates a SignedTask from a Request.
// The request is serialized to JSON, compressed with LZMA, and signed.
func NewSignedTask(req *Request, secret []byte) (*SignedTask, error) {
	// Serialize and compress the request
	payload, err := MarshalCompressed(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	timestamp := time.Now().Unix()

	// Sign the payload + timestamp to bind them together
	signatureData := append(payload, []byte(fmt.Sprintf("%d", timestamp))...)
	signature := Sign(signatureData, secret)

	return &SignedTask{
		Payload:   payload,
		Signature: signature,
		Timestamp: timestamp,
	}, nil
}

// ValidateSignedTask verifies the signature and timestamp of a SignedTask.
// Returns the decompressed and deserialized Request if valid.
//
// Validation checks:
// 1. Timestamp is within maxAge of current time (prevents replay attacks)
// 2. Signature matches HMAC-SHA256(payload + timestamp, secret)
func ValidateSignedTask(task *SignedTask, secret []byte, maxAge time.Duration) (*Request, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}

	// Check timestamp for replay protection
	taskTime := time.Unix(task.Timestamp, 0)
	age := time.Since(taskTime)
	if age < 0 {
		age = -age // Handle clock skew in either direction
	}
	if age > maxAge {
		return nil, &ValidationError{
			Code:    ErrCodeReplayAttack,
			Message: fmt.Sprintf("task timestamp too old: %v (max age: %v)", age, maxAge),
		}
	}

	// Verify signature
	signatureData := append(task.Payload, []byte(fmt.Sprintf("%d", task.Timestamp))...)
	if !Verify(signatureData, task.Signature, secret) {
		return nil, &ValidationError{
			Code:    ErrCodeSignatureInvalid,
			Message: "signature verification failed",
		}
	}

	// Decompress and deserialize
	var req Request
	if err := UnmarshalCompressed(task.Payload, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request: %w", err)
	}

	return &req, nil
}

// ValidationError represents a task validation failure.
type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
