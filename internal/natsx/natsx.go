// Package natsx contains NATS helpers for Straw services.
package natsx

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

const payloadSafetyMargin = 64 * 1024

var (
	errUnsupportedStreamDirection = errors.New("unsupported stream direction")
	errMaxPayloadTooSmall         = errors.New("nats max payload must exceed safety margin")
	errFrameBodyLimitTooLarge     = errors.New("configured frame/body limit exceeds nats max payload")
	errNATSServersRequired        = errors.New("nats servers are required")
	errNATSServerEmpty            = errors.New("nats server is empty")
	errSubjectTokenRequired       = errors.New("subject token is required")
	errSubjectTokenUnsafe         = errors.New("subject token contains unsafe character")
)

// StreamDirection identifies the direction of a stream subject.
type StreamDirection string

const (
	// DirectionControlToExecutor marks control-to-executor stream subjects.
	DirectionControlToExecutor StreamDirection = "c2e"
	// DirectionExecutorToControl marks executor-to-control stream subjects.
	DirectionExecutorToControl StreamDirection = "e2c"
)

// RegistrationSubject returns the NATS subject for worker registration.
func RegistrationSubject() string {
	return "straw.v1.control.register"
}

// HeartbeatSubject returns the NATS subject for worker heartbeats.
func HeartbeatSubject() string {
	return "straw.v1.control.heartbeat"
}

// LogTelemetrySubject returns the transient Egress-to-Control log subject.
func LogTelemetrySubject() string {
	return "straw.v1.control.logs"
}

// ControlInboxPrefix returns the reply-inbox prefix used by control clients.
func ControlInboxPrefix() string {
	return "_INBOX.ctl"
}

// WorkerInboxPrefix returns the worker-specific reply inbox prefix.
func WorkerInboxPrefix(workerID string) (string, error) {
	err := validateSubjectToken(workerID)
	if err != nil {
		return "", fmt.Errorf("worker_id: %w", err)
	}

	return "_INBOX.wrk." + workerID, nil
}

// ValidateSubjectToken reports whether token is a safe NATS subject token.
func ValidateSubjectToken(token string) error {
	return validateSubjectToken(token)
}

// AssignmentSubject returns the assignment subject for a worker session.
func AssignmentSubject(workerID, sessionID string) (string, error) {
	err := validateSubjectToken(workerID)
	if err != nil {
		return "", fmt.Errorf("worker_id: %w", err)
	}

	err = validateSubjectToken(sessionID)
	if err != nil {
		return "", fmt.Errorf("session_id: %w", err)
	}

	return fmt.Sprintf("straw.v1.executor.%s.%s.assign", workerID, sessionID), nil
}

// StreamSubject returns the stream subject for a request, worker, and session.
func StreamSubject(requestID, workerID, sessionID string, direction StreamDirection) (string, error) {
	err := validateSubjectToken(requestID)
	if err != nil {
		return "", fmt.Errorf("request_id: %w", err)
	}

	err = validateSubjectToken(workerID)
	if err != nil {
		return "", fmt.Errorf("worker_id: %w", err)
	}

	err = validateSubjectToken(sessionID)
	if err != nil {
		return "", fmt.Errorf("session_id: %w", err)
	}

	switch direction {
	case DirectionControlToExecutor, DirectionExecutorToControl:
		return fmt.Sprintf("straw.v1.req.%s.%s.%s.%s", requestID, workerID, sessionID, direction), nil
	default:
		return "", fmt.Errorf("%w: %q", errUnsupportedStreamDirection, direction)
	}
}

// TerminalSubject returns the terminal subject for a request stream.
func TerminalSubject(requestID, workerID, sessionID string, direction StreamDirection) (string, error) {
	return StreamSubject(requestID, workerID, sessionID, direction)
}

// ValidateMaxPayload checks the configured NATS payload limits.
func ValidateMaxPayload(maxPayloadBytes *uint64, maxFrameDataBytes, maxInlineRequestBodyBytes, maxInlineResponseBodyBytes uint64) error {
	if maxPayloadBytes == nil {
		return nil
	}

	if *maxPayloadBytes <= payloadSafetyMargin {
		return fmt.Errorf("%w: %d <= %d", errMaxPayloadTooSmall, *maxPayloadBytes, payloadSafetyMargin)
	}

	limit := max(maxInlineRequestBodyBytes, maxFrameDataBytes)

	limit = max(limit, maxInlineResponseBodyBytes)

	if limit > *maxPayloadBytes-payloadSafetyMargin {
		return fmt.Errorf("%w: %d > %d - %d safety margin", errFrameBodyLimitTooLarge, limit, *maxPayloadBytes, payloadSafetyMargin)
	}

	return nil
}

// ValidateServers validates the configured NATS server list.
func ValidateServers(servers []string) error {
	if len(servers) == 0 {
		return errNATSServersRequired
	}

	for i, server := range servers {
		if strings.TrimSpace(server) == "" {
			return fmt.Errorf("%w: %d", errNATSServerEmpty, i)
		}
	}

	return nil
}

func validateSubjectToken(token string) error {
	if token == "" {
		return errSubjectTokenRequired
	}

	for _, r := range token {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
		default:
			return fmt.Errorf("%w: %q", errSubjectTokenUnsafe, r)
		}
	}

	return nil
}
