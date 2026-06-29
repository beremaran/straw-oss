package domain

import "time"

// Session represents a client session bound to an endpoint and routing rule.
type Session struct {
	ID             string    `json:"id"`
	EndpointID     string    `json:"endpoint_id"`
	Tags           []string  `json:"tags"`
	RuleID         string    `json:"rule_id"`
	MigrationCount int       `json:"migration_count"`
	CreatedAt      time.Time `json:"created_at"`
	LastUsedAt     time.Time `json:"last_used_at"`
	RequestCount   int       `json:"request_count"`
}

// DefaultSessionTTL is the default time-to-live for a session.
const DefaultSessionTTL = 10 * time.Minute

// MaxMigrationCount is the maximum number of times a session can migrate to a new endpoint.
const MaxMigrationCount = 3

// NewSession creates a new Session with the given id, endpoint ID, rule ID, and tags.
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

// IsExpired reports whether the session has been idle longer than the TTL.
func (s *Session) IsExpired(ttl time.Duration) bool {
	return time.Since(s.LastUsedAt) > ttl
}

// Touch records the current time as last used and increments the request count.
func (s *Session) Touch() {
	s.LastUsedAt = time.Now()
	s.RequestCount++
}

// CanMigrate reports whether the session can be migrated to a new endpoint.
func (s *Session) CanMigrate() bool {
	return s.MigrationCount < MaxMigrationCount
}

// Migrate moves the session to a new endpoint, incrementing the migration count.
func (s *Session) Migrate(newEndpointID string) bool {
	if !s.CanMigrate() {
		return false
	}

	s.EndpointID = newEndpointID
	s.MigrationCount++
	s.LastUsedAt = time.Now()

	return true
}
