// Package session provides session management with a Redis backend.
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/beremaran/straw/internal/domain"
)

const randomIDBytes = 16

// Service manages session lifecycle operations.
type Service struct {
	store *RedisStore
}

// NewService creates a new Service with the given store.
func NewService(store *RedisStore) *Service {
	return &Service{
		store: store,
	}
}

// CreateSession creates a new session with the given endpoint, rule, and tags.
func (s *Service) CreateSession(ctx context.Context, endpointID string, ruleID string, tags []string) (*domain.Session, error) {
	id, err := generateRandomID(randomIDBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate session id: %w", err)
	}

	session := domain.NewSession(id, endpointID, ruleID, tags)

	err = s.store.Save(ctx, session, domain.DefaultSessionTTL)
	if err != nil {
		return nil, err
	}

	return session, nil
}

// GetSession retrieves a session by ID, returning ErrSessionExpired if it has expired.
func (s *Service) GetSession(ctx context.Context, id string) (*domain.Session, error) {
	session, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	if session.IsExpired(domain.DefaultSessionTTL) {
		_ = s.store.Delete(ctx, id)

		return nil, domain.ErrSessionExpired
	}

	return session, nil
}

// TouchSession refreshes the TTL of an existing session.
func (s *Service) TouchSession(ctx context.Context, id string) error {
	return s.store.Touch(ctx, id, domain.DefaultSessionTTL)
}

// MigrateSession migrates a session to a new endpoint, enforcing migration limits.
func (s *Service) MigrateSession(ctx context.Context, id string, newEndpointID string) (*domain.Session, error) {
	session, err := s.GetSession(ctx, id)
	if err != nil {
		return nil, err
	}

	if !session.Migrate(newEndpointID) {
		return nil, domain.ErrSessionMigrationLimit
	}

	err = s.store.Save(ctx, session, domain.DefaultSessionTTL)
	if err != nil {
		return nil, err
	}

	return session, nil
}

// EndSession permanently deletes a session.
func (s *Service) EndSession(ctx context.Context, id string) error {
	return s.store.Delete(ctx, id)
}

func generateRandomID(bytes int) (string, error) {
	b := make([]byte, bytes)

	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("failed to read random bytes: %w", err)
	}

	return hex.EncodeToString(b), nil
}
