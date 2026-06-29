package handlers

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/beremaran/straw/internal/domain"
)

type MockAPIKeyRepo struct {
	mock.Mock
}

func (m *MockAPIKeyRepo) GetByID(ctx context.Context, id string) (*domain.APIKey, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.APIKey), args.Error(1)
}

func (m *MockAPIKeyRepo) Create(ctx context.Context, key *domain.APIKey) error {
	args := m.Called(ctx, key)

	return args.Error(0)
}

func (m *MockAPIKeyRepo) Update(ctx context.Context, key *domain.APIKey) error {
	args := m.Called(ctx, key)

	return args.Error(0)
}

func (m *MockAPIKeyRepo) List(ctx context.Context, limit, offset int) ([]domain.APIKey, int, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}

	return args.Get(0).([]domain.APIKey), args.Int(1), args.Error(2)
}

func (m *MockAPIKeyRepo) Exists(ctx context.Context) (bool, error) {
	args := m.Called(ctx)

	return args.Bool(0), args.Error(1)
}

func (m *MockAPIKeyRepo) Revoke(ctx context.Context, id string) error {
	args := m.Called(ctx, id)

	return args.Error(0)
}

func (m *MockAPIKeyRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.APIKey, error) {
	args := m.Called(ctx, tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.APIKey), args.Error(1)
}

type MockAPIKeyTokenRepo struct {
	mock.Mock
}

func (m *MockAPIKeyTokenRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.APIKeyToken, error) {
	args := m.Called(ctx, tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.APIKeyToken), args.Error(1)
}

func (m *MockAPIKeyTokenRepo) Create(ctx context.Context, token *domain.APIKeyToken) error {
	args := m.Called(ctx, token)

	return args.Error(0)
}

func (m *MockAPIKeyTokenRepo) ListByAPIKeyID(ctx context.Context, apiKeyID string) ([]domain.APIKeyToken, error) {
	args := m.Called(ctx, apiKeyID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).([]domain.APIKeyToken), args.Error(1)
}

func (m *MockAPIKeyTokenRepo) Rotate(ctx context.Context, apiKeyID string, token *domain.APIKeyToken, graceUntil *time.Time, revokeExisting bool) error {
	args := m.Called(ctx, apiKeyID, token, graceUntil, revokeExisting)

	return args.Error(0)
}

func (m *MockAPIKeyTokenRepo) UpdateStatus(ctx context.Context, id string, status domain.TokenStatus) error {
	args := m.Called(ctx, id, status)

	return args.Error(0)
}

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

func (m *MockRoutingRuleRepo) ListActiveRulesReferencingFingerprintPreset(ctx context.Context, presetID string) ([]domain.RoutingRuleReference, error) {
	args := m.Called(ctx, presetID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).([]domain.RoutingRuleReference), args.Error(1)
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

type MockIdentityRepo struct {
	mock.Mock
}

func (m *MockIdentityRepo) CreateUser(ctx context.Context, user *domain.AdminUser) error {
	args := m.Called(ctx, user)

	return args.Error(0)
}

func (m *MockIdentityRepo) UpdateUser(ctx context.Context, user *domain.AdminUser) error {
	args := m.Called(ctx, user)

	return args.Error(0)
}

func (m *MockIdentityRepo) GetUserByID(ctx context.Context, id string) (*domain.AdminUser, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.AdminUser), args.Error(1)
}

func (m *MockIdentityRepo) GetUserByEmail(ctx context.Context, email string) (*domain.AdminUser, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.AdminUser), args.Error(1)
}

func (m *MockIdentityRepo) ListUsers(ctx context.Context, limit, offset int) ([]domain.AdminUser, int, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}

	return args.Get(0).([]domain.AdminUser), args.Int(1), args.Error(2)
}

func (m *MockIdentityRepo) SetUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	args := m.Called(ctx, userID, roleIDs)

	return args.Error(0)
}

func (m *MockIdentityRepo) ListUserRoles(ctx context.Context, userID string) ([]domain.AdminRole, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).([]domain.AdminRole), args.Error(1)
}

func (m *MockIdentityRepo) EffectivePermissions(ctx context.Context, userID string) ([]string, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).([]string), args.Error(1)
}

func (m *MockIdentityRepo) ActiveOwnerExists(ctx context.Context) (bool, error) {
	args := m.Called(ctx)

	return args.Bool(0), args.Error(1)
}

func (m *MockIdentityRepo) CountActiveOwners(ctx context.Context) (int, error) {
	args := m.Called(ctx)

	return args.Int(0), args.Error(1)
}

func (m *MockIdentityRepo) CreateRole(ctx context.Context, role *domain.AdminRole) error {
	args := m.Called(ctx, role)

	return args.Error(0)
}

func (m *MockIdentityRepo) UpdateRole(ctx context.Context, role *domain.AdminRole) error {
	args := m.Called(ctx, role)

	return args.Error(0)
}

func (m *MockIdentityRepo) DeleteRole(ctx context.Context, id string) error {
	args := m.Called(ctx, id)

	return args.Error(0)
}

func (m *MockIdentityRepo) GetRoleByID(ctx context.Context, id string) (*domain.AdminRole, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.AdminRole), args.Error(1)
}

func (m *MockIdentityRepo) GetRoleByName(ctx context.Context, name string) (*domain.AdminRole, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.AdminRole), args.Error(1)
}

func (m *MockIdentityRepo) ListRoles(ctx context.Context) ([]domain.AdminRole, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).([]domain.AdminRole), args.Error(1)
}

func (m *MockIdentityRepo) CreateSession(ctx context.Context, session *domain.AdminSession) error {
	args := m.Called(ctx, session)

	return args.Error(0)
}

func (m *MockIdentityRepo) GetSessionByID(ctx context.Context, id string) (*domain.AdminSession, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.AdminSession), args.Error(1)
}

func (m *MockIdentityRepo) GetSessionByRefreshTokenHash(ctx context.Context, hash string) (*domain.AdminSession, error) {
	args := m.Called(ctx, hash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.AdminSession), args.Error(1)
}

func (m *MockIdentityRepo) UpdateSessionRefreshHash(ctx context.Context, id, hash string) error {
	args := m.Called(ctx, id, hash)

	return args.Error(0)
}

func (m *MockIdentityRepo) RevokeSession(ctx context.Context, id string) error {
	args := m.Called(ctx, id)

	return args.Error(0)
}

func (m *MockIdentityRepo) RevokeUserSessions(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)

	return args.Error(0)
}

func (m *MockIdentityRepo) CreateIdentityProvider(ctx context.Context, provider *domain.AdminIdentityProvider) error {
	args := m.Called(ctx, provider)

	return args.Error(0)
}

func (m *MockIdentityRepo) UpdateIdentityProvider(ctx context.Context, provider *domain.AdminIdentityProvider) error {
	args := m.Called(ctx, provider)

	return args.Error(0)
}

func (m *MockIdentityRepo) GetIdentityProviderByID(ctx context.Context, id string) (*domain.AdminIdentityProvider, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.AdminIdentityProvider), args.Error(1)
}

func (m *MockIdentityRepo) GetIdentityProviderByName(ctx context.Context, name string) (*domain.AdminIdentityProvider, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.AdminIdentityProvider), args.Error(1)
}

func (m *MockIdentityRepo) ListIdentityProviders(ctx context.Context) ([]domain.AdminIdentityProvider, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).([]domain.AdminIdentityProvider), args.Error(1)
}

func (m *MockIdentityRepo) DisableIdentityProvider(ctx context.Context, id string) error {
	args := m.Called(ctx, id)

	return args.Error(0)
}

type MockManagementAuditRepo struct {
	mock.Mock
}

func (m *MockManagementAuditRepo) Create(ctx context.Context, event *domain.ManagementAuditEvent) error {
	args := m.Called(ctx, event)

	return args.Error(0)
}

func (m *MockManagementAuditRepo) GetEventByID(ctx context.Context, id int64) (*domain.ManagementAuditEvent, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.ManagementAuditEvent), args.Error(1)
}

func (m *MockManagementAuditRepo) ListEvents(ctx context.Context, filter domain.AuditEventFilter) ([]*domain.ManagementAuditEvent, int, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}

	return args.Get(0).([]*domain.ManagementAuditEvent), args.Int(1), args.Error(2)
}

func (m *MockManagementAuditRepo) ListRequests(ctx context.Context, filter domain.AuditEventFilter, includeBody bool) ([]*domain.ManagementAuditRequest, int, error) {
	args := m.Called(ctx, filter, includeBody)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}

	return args.Get(0).([]*domain.ManagementAuditRequest), args.Int(1), args.Error(2)
}
