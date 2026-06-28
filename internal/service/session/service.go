package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/beremaran/straw/internal/domain"
)

type Service struct {
	store *RedisStore
}

func NewService(store *RedisStore) *Service {
	return &Service{
		store: store,
	}
}

func (s *Service) CreateSession(ctx context.Context, endpointID string, ruleID string, tags []string) (*domain.Session, error) {
	id, err := generateRandomID(16)
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

func (s *Service) TouchSession(ctx context.Context, id string) error {
	return s.store.Touch(ctx, id, domain.DefaultSessionTTL)
}

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

func (s *Service) EndSession(ctx context.Context, id string) error {
	return s.store.Delete(ctx, id)
}

func generateRandomID(bytes int) (string, error) {
	b := make([]byte, bytes)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(b), nil
}
