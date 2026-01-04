package handlers

import (
	"context"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/stretchr/testify/mock"
)

// MockApiKeyRepo is a mock implementation of domain.ApiKeyRepository
type MockApiKeyRepo struct {
	mock.Mock
}

func (m *MockApiKeyRepo) GetByID(ctx context.Context, id string) (*domain.ApiKey, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ApiKey), args.Error(1)
}

func (m *MockApiKeyRepo) Create(ctx context.Context, key *domain.ApiKey) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *MockApiKeyRepo) List(ctx context.Context, limit, offset int) ([]domain.ApiKey, int, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]domain.ApiKey), args.Int(1), args.Error(2)
}

func (m *MockApiKeyRepo) Revoke(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockApiKeyRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.ApiKey, error) {
	args := m.Called(ctx, tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ApiKey), args.Error(1)
}

// MockRoutingRuleRepo is a mock implementation of domain.RoutingRuleRepository
type MockRoutingRuleRepo struct {
	mock.Mock
}

func (m *MockRoutingRuleRepo) GetActiveRules(ctx context.Context) ([]domain.RoutingRule, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.RoutingRule), args.Error(1)
}

func (m *MockRoutingRuleRepo) CreateRule(ctx context.Context, rule *domain.RoutingRule) error {
	args := m.Called(ctx, rule)
	return args.Error(0)
}

func (m *MockRoutingRuleRepo) GetRuleByID(ctx context.Context, id string) (*domain.RoutingRule, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RoutingRule), args.Error(1)
}

func (m *MockRoutingRuleRepo) UpdateRule(ctx context.Context, rule *domain.RoutingRule) error {
	args := m.Called(ctx, rule)
	return args.Error(0)
}

func (m *MockRoutingRuleRepo) DeleteRule(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockRoutingRuleRepo) ListRules(ctx context.Context, limit, offset int) ([]domain.RoutingRule, int, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]domain.RoutingRule), args.Int(1), args.Error(2)
}
