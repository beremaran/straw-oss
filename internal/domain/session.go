package domain

import "time"

type Session struct {
	ID string `json:"id"`

	EndpointID string `json:"endpoint_id"`

	Tags []string `json:"tags"`

	RuleID string `json:"rule_id"`

	MigrationCount int `json:"migration_count"`

	CreatedAt time.Time `json:"created_at"`

	LastUsedAt time.Time `json:"last_used_at"`

	RequestCount int `json:"request_count"`
}

const DefaultSessionTTL = 10 * time.Minute

const MaxMigrationCount = 3

func (s *Session) IsExpired(ttl time.Duration) bool {
	return time.Since(s.LastUsedAt) > ttl
}

func (s *Session) Touch() {
	s.LastUsedAt = time.Now()
	s.RequestCount++
}

func (s *Session) CanMigrate() bool {
	return s.MigrationCount < MaxMigrationCount
}

func (s *Session) Migrate(newEndpointID string) bool {
	if !s.CanMigrate() {
		return false
	}
	s.EndpointID = newEndpointID
	s.MigrationCount++
	s.LastUsedAt = time.Now()
	return true
}

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
