package protocol

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

const DefaultMaxTaskAge = 60 * time.Second

func Sign(data []byte, secret []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func Verify(data []byte, signature string, secret []byte) bool {
	expected := Sign(data, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func NewSignedTask(req *Request, secret []byte) (*SignedTask, error) {

	payload, err := MarshalCompressed(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	timestamp := time.Now().Unix()

	signatureData := append(payload, []byte(fmt.Sprintf("%d", timestamp))...)
	signature := Sign(signatureData, secret)

	return &SignedTask{
		Payload:   payload,
		Signature: signature,
		Timestamp: timestamp,
	}, nil
}

func ValidateSignedTask(task *SignedTask, secret []byte, maxAge time.Duration) (*Request, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
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

	signatureData := append(task.Payload, []byte(fmt.Sprintf("%d", task.Timestamp))...)
	if !Verify(signatureData, task.Signature, secret) {
		return nil, &ValidationError{
			Code:    ErrCodeSignatureInvalid,
			Message: "signature verification failed",
		}
	}

	var req Request
	if err := UnmarshalCompressed(task.Payload, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request: %w", err)
	}

	return &req, nil
}

type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
