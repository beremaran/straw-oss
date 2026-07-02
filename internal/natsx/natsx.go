package natsx

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

const payloadSafetyMargin = 64 * 1024

type StreamDirection string

const (
	DirectionControlToExecutor StreamDirection = "c2e"
	DirectionExecutorToControl StreamDirection = "e2c"
)

func RegistrationSubject() string {
	return "straw.v1.control.register"
}

func HeartbeatSubject() string {
	return "straw.v1.control.heartbeat"
}

// ControlInboxPrefix is the custom reply-inbox prefix Control uses for
// request/reply clients so inbox permissions can be scoped in the NATS ACL
// table (docs/planning/12-nats-protocol.md). A concrete inbox subject is
// this prefix plus NATS-generated tokens; the ACL grants `_INBOX.ctl.>`.
func ControlInboxPrefix() string {
	return "_INBOX.ctl"
}

// WorkerInboxPrefix is the custom reply-inbox prefix a worker uses for its
// request/reply client. Each worker's inbox is scoped to its own worker_id
// (`_INBOX.wrk.<worker_id>.>` in the ACL table) so one worker cannot read
// another's replies.
func WorkerInboxPrefix(workerID string) (string, error) {
	if err := validateSubjectToken(workerID); err != nil {
		return "", fmt.Errorf("worker_id: %w", err)
	}
	return fmt.Sprintf("_INBOX.wrk.%s", workerID), nil
}

// ValidateSubjectToken reports whether token is a safe, dot-free NATS subject
// token usable for worker_id, session_id, or request_id.
func ValidateSubjectToken(token string) error {
	return validateSubjectToken(token)
}

func AssignmentSubject(workerID, sessionID string) (string, error) {
	if err := validateSubjectToken(workerID); err != nil {
		return "", fmt.Errorf("worker_id: %w", err)
	}
	if err := validateSubjectToken(sessionID); err != nil {
		return "", fmt.Errorf("session_id: %w", err)
	}
	return fmt.Sprintf("straw.v1.executor.%s.%s.assign", workerID, sessionID), nil
}

func StreamSubject(requestID, workerID, sessionID string, direction StreamDirection) (string, error) {
	if err := validateSubjectToken(requestID); err != nil {
		return "", fmt.Errorf("request_id: %w", err)
	}
	if err := validateSubjectToken(workerID); err != nil {
		return "", fmt.Errorf("worker_id: %w", err)
	}
	if err := validateSubjectToken(sessionID); err != nil {
		return "", fmt.Errorf("session_id: %w", err)
	}
	switch direction {
	case DirectionControlToExecutor, DirectionExecutorToControl:
		return fmt.Sprintf("straw.v1.req.%s.%s.%s.%s", requestID, workerID, sessionID, direction), nil
	default:
		return "", fmt.Errorf("unsupported stream direction %q", direction)
	}
}

func TerminalSubject(requestID, workerID, sessionID string, direction StreamDirection) (string, error) {
	return StreamSubject(requestID, workerID, sessionID, direction)
}

func ValidateMaxPayload(maxPayloadBytes *uint64, maxFrameDataBytes, maxInlineRequestBodyBytes, maxInlineResponseBodyBytes uint64) error {
	if maxPayloadBytes == nil {
		return nil
	}
	if *maxPayloadBytes <= payloadSafetyMargin {
		return fmt.Errorf("nats max payload %d must exceed %d bytes of safety margin", *maxPayloadBytes, payloadSafetyMargin)
	}

	limit := maxFrameDataBytes
	if maxInlineRequestBodyBytes > limit {
		limit = maxInlineRequestBodyBytes
	}
	if maxInlineResponseBodyBytes > limit {
		limit = maxInlineResponseBodyBytes
	}
	if limit > *maxPayloadBytes-payloadSafetyMargin {
		return fmt.Errorf("configured frame/body limit %d exceeds nats max payload %d minus %d-byte safety margin", limit, *maxPayloadBytes, payloadSafetyMargin)
	}
	return nil
}

func ValidateServers(servers []string) error {
	if len(servers) == 0 {
		return errors.New("nats servers are required")
	}
	for i, server := range servers {
		if strings.TrimSpace(server) == "" {
			return fmt.Errorf("nats server %d is empty", i)
		}
	}
	return nil
}

func validateSubjectToken(token string) error {
	if token == "" {
		return errors.New("subject token is required")
	}
	for _, r := range token {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
		default:
			return fmt.Errorf("subject token %q contains unsafe character %q", token, r)
		}
	}
	return nil
}
