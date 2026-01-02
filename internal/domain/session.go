package domain

import "time"

// Session represents a sticky session for multi-step scraping workflows.
// Sessions bind a series of requests to a specific endpoint to maintain
// client state (cookies, CSRF tokens, etc.) on the target website.
type Session struct {
	// ID is the unique session identifier.
	ID string `json:"id"`

	// EndpointID identifies which endpoint is handling this session.
	EndpointID string `json:"endpoint_id"`

	// Tags are the original request tags used to create this session.
	Tags []string `json:"tags"`

	// RuleID is the routing rule that was matched for this session.
	RuleID string `json:"rule_id"`

	// MigrationCount tracks how many times this session has been migrated
	// to a different endpoint due to failures.
	MigrationCount int `json:"migration_count"`

	// CreatedAt is when the session was created.
	CreatedAt time.Time `json:"created_at"`

	// LastUsedAt is when the session was last accessed.
	LastUsedAt time.Time `json:"last_used_at"`

	// RequestCount is the total number of requests made in this session.
	RequestCount int `json:"request_count"`
}

// DefaultSessionTTL is the default time-to-live for sessions (10 minutes).
const DefaultSessionTTL = 10 * time.Minute

// MaxMigrationCount is the maximum number of migrations before a session is force-expired.
const MaxMigrationCount = 3

// IsExpired checks if the session has expired based on the given TTL.
// A session expires when LastUsedAt is older than the TTL.
func (s *Session) IsExpired(ttl time.Duration) bool {
	return time.Since(s.LastUsedAt) > ttl
}

// Touch updates the LastUsedAt timestamp and increments the request count.
// Call this on every request that uses the session.
func (s *Session) Touch() {
	s.LastUsedAt = time.Now()
	s.RequestCount++
}

// CanMigrate returns true if the session can be migrated to another endpoint.
// Sessions cannot be migrated if they've exceeded the maximum migration count.
func (s *Session) CanMigrate() bool {
	return s.MigrationCount < MaxMigrationCount
}

// Migrate updates the session to point to a new endpoint and increments
// the migration counter. Returns false if migration limit exceeded.
func (s *Session) Migrate(newEndpointID string) bool {
	if !s.CanMigrate() {
		return false
	}
	s.EndpointID = newEndpointID
	s.MigrationCount++
	s.LastUsedAt = time.Now()
	return true
}

// NewSession creates a new session with the given parameters.
func NewSession(id, endpointID, ruleID string, tags []string) *Session {
	now := time.Now()
	return &Session{
		ID:             id,
		EndpointID:     endpointID,
		Tags:           tags,
		RuleID:         ruleID,
		MigrationCount: 0,
		CreatedAt:      now,
		LastUsedAt:     now,
		RequestCount:   0,
	}
}
