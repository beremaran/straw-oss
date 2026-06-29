package protocol

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// ErrTaskIsNil is returned when a nil SignedTask is passed to validation.
var ErrTaskIsNil = errors.New("task is nil")

// DefaultMaxTaskAge is the default maximum age for a signed task before it is
// considered expired.
const DefaultMaxTaskAge = 60 * time.Second

// Sign computes an HMAC-SHA256 signature over data using the given secret.
func Sign(data []byte, secret []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write(data)

	return hex.EncodeToString(h.Sum(nil))
}

// Verify checks that signature is a valid HMAC-SHA256 signature over data
// using the given secret.
func Verify(data []byte, signature string, secret []byte) bool {
	expected := Sign(data, secret)

	return hmac.Equal([]byte(expected), []byte(signature))
}

// NewSignedTask compresses and signs a request, returning a SignedTask.
func NewSignedTask(req *Request, secret []byte) (*SignedTask, error) {
	payload, err := MarshalCompressed(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	timestamp := time.Now().Unix()

	signatureData := make([]byte, 0, len(payload)+len(strconv.FormatInt(timestamp, 10)))
	signatureData = append(signatureData, payload...)
	signatureData = append(signatureData, []byte(strconv.FormatInt(timestamp, 10))...)
	signature := Sign(signatureData, secret)

	return &SignedTask{
		Payload:   payload,
		Signature: signature,
		Timestamp: timestamp,
	}, nil
}

// ValidateSignedTask verifies the timestamp, signature, and payload of a
// SignedTask, returning the original request if valid.
func ValidateSignedTask(task *SignedTask, secret []byte, maxAge time.Duration) (*Request, error) {
	if task == nil {
		return nil, ErrTaskIsNil
	}

	taskTime := time.Unix(task.Timestamp, 0)

	age := time.Since(taskTime)
	if age < 0 {
		age = -age
	}

	if age > maxAge {
		return nil, &ValidationError{
			Code:    ErrCodeReplayAttack,
			Message: fmt.Sprintf("task timestamp too old: %v (max age: %v)", age, maxAge),
		}
	}

	signatureData := make([]byte, 0, len(task.Payload)+len(strconv.FormatInt(task.Timestamp, 10)))
	signatureData = append(signatureData, task.Payload...)
	signatureData = append(signatureData, []byte(strconv.FormatInt(task.Timestamp, 10))...)

	if !Verify(signatureData, task.Signature, secret) {
		return nil, &ValidationError{
			Code:    ErrCodeSignatureInvalid,
			Message: "signature verification failed",
		}
	}

	var req Request

	err := UnmarshalCompressed(task.Payload, &req)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal request: %w", err)
	}

	return &req, nil
}

// ValidationError represents a signature validation failure with a machine-
// readable code and human-readable message.
type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
