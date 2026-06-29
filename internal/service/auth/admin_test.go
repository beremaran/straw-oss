package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

type fakeAdminRepo struct {
	usersByID    map[string]*domain.AdminUser
	usersByEmail map[string]*domain.AdminUser
	sessions     map[string]*domain.AdminSession
	role         *domain.AdminRole
	permissions  []string
	ownerExists  bool
}

func newFakeAdminRepo(t *testing.T) *fakeAdminRepo {
	t.Helper()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}

	user := &domain.AdminUser{
		ID:           "user-1",
		Email:        "owner@example.test",
		DisplayName:  "Owner",
		PasswordHash: string(passwordHash),
		IsActive:     true,
	}

	return &fakeAdminRepo{
		usersByID:    map[string]*domain.AdminUser{user.ID: user},
		usersByEmail: map[string]*domain.AdminUser{strings.ToLower(user.Email): user},
		sessions:     map[string]*domain.AdminSession{},
		role:         &domain.AdminRole{ID: "role-owner", Name: domain.RoleOwner},
		permissions:  []string{domain.PermissionAPIKeysRead, domain.PermissionUsersWrite},
	}
}

func (r *fakeAdminRepo) CreateUser(ctx context.Context, user *domain.AdminUser) error {
	r.usersByID[user.ID] = user
	r.usersByEmail[strings.ToLower(user.Email)] = user

	return nil
}

func (r *fakeAdminRepo) UpdateUser(ctx context.Context, user *domain.AdminUser) error {
	r.usersByID[user.ID] = user
	r.usersByEmail[strings.ToLower(user.Email)] = user

	return nil
}

func (r *fakeAdminRepo) GetUserByID(ctx context.Context, id string) (*domain.AdminUser, error) {
	return r.usersByID[id], nil
}

func (r *fakeAdminRepo) GetUserByEmail(ctx context.Context, email string) (*domain.AdminUser, error) {
	return r.usersByEmail[strings.ToLower(email)], nil
}

func (r *fakeAdminRepo) SetUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	if len(roleIDs) > 0 && roleIDs[0] == r.role.ID {
		r.ownerExists = true
	}

	return nil
}

func (r *fakeAdminRepo) EffectivePermissions(ctx context.Context, userID string) ([]string, error) {
	return append([]string(nil), r.permissions...), nil
}

func (r *fakeAdminRepo) ActiveOwnerExists(ctx context.Context) (bool, error) {
	return r.ownerExists, nil
}

func (r *fakeAdminRepo) GetRoleByName(ctx context.Context, name string) (*domain.AdminRole, error) {
	if name == r.role.Name {
		return r.role, nil
	}

	return nil, nil
}

func (r *fakeAdminRepo) GetIdentityProviderByName(ctx context.Context, name string) (*domain.AdminIdentityProvider, error) {
	return nil, nil
}

func (r *fakeAdminRepo) CreateSession(ctx context.Context, session *domain.AdminSession) error {
	r.sessions[session.ID] = session

	return nil
}

func (r *fakeAdminRepo) GetSessionByID(ctx context.Context, id string) (*domain.AdminSession, error) {
	return r.sessions[id], nil
}

func (r *fakeAdminRepo) UpdateSessionRefreshHash(ctx context.Context, id, hash string) error {
	session := r.sessions[id]
	if session == nil {
		return errors.New("missing session")
	}
	session.RefreshTokenHash = hash
	session.LastUsedAt = time.Now()

	return nil
}

func (r *fakeAdminRepo) RevokeSession(ctx context.Context, id string) error {
	session := r.sessions[id]
	if session == nil {
		return errors.New("missing session")
	}
	now := time.Now()
	session.RevokedAt = &now

	return nil
}

func TestAdminService_LoginFailure(t *testing.T) {
	repo := newFakeAdminRepo(t)
	service := NewAdminService(repo, "secret", time.Hour, 24*time.Hour)

	tokens, err := service.Login(context.Background(), "owner@example.test", "wrong-password", "", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want %v", err, ErrInvalidCredentials)
	}
	if tokens != nil {
		t.Fatal("Login() returned tokens for a bad password")
	}
	if len(repo.sessions) != 0 {
		t.Fatalf("Login() created %d sessions, want 0", len(repo.sessions))
	}
}

func TestAdminService_LoginSuccess(t *testing.T) {
	repo := newFakeAdminRepo(t)
	service := NewAdminService(repo, "secret", time.Hour, 24*time.Hour)

	tokens, err := service.Login(context.Background(), "owner@example.test", "correct-password", "ua", "127.0.0.1")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatal("Login() returned empty tokens")
	}
	if tokens.User.LastLoginAt == nil {
		t.Fatal("Login() did not update last login")
	}

	claims, err := service.VerifyAccessToken(context.Background(), tokens.AccessToken)
	if err != nil {
		t.Fatalf("VerifyAccessToken() error = %v", err)
	}
	if claims.UserID != "user-1" || claims.SessionID != tokens.Session.ID {
		t.Fatalf("claims = %+v, want user/session from login", claims)
	}
	if len(claims.Permissions) != len(repo.permissions) {
		t.Fatalf("permissions = %v, want %v", claims.Permissions, repo.permissions)
	}
}

func TestAdminService_RefreshRotationRevokesReuse(t *testing.T) {
	repo := newFakeAdminRepo(t)
	service := NewAdminService(repo, "secret", time.Hour, 24*time.Hour)
	tokens, err := service.Login(context.Background(), "owner@example.test", "correct-password", "", "")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	rotated, err := service.Refresh(context.Background(), tokens.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if rotated.RefreshToken == tokens.RefreshToken {
		t.Fatal("Refresh() did not rotate the refresh token")
	}
	if repo.sessions[tokens.Session.ID].RefreshTokenHash != sha256Hash(rotated.RefreshToken) {
		t.Fatal("Refresh() did not persist the new refresh token hash")
	}

	_, err = service.Refresh(context.Background(), tokens.RefreshToken)
	if !errors.Is(err, ErrRefreshReuse) {
		t.Fatalf("Refresh(old token) error = %v, want %v", err, ErrRefreshReuse)
	}
	if repo.sessions[tokens.Session.ID].RevokedAt == nil {
		t.Fatal("Refresh(old token) did not revoke the session family")
	}
}

func TestAdminService_LogoutRevokesSession(t *testing.T) {
	repo := newFakeAdminRepo(t)
	service := NewAdminService(repo, "secret", time.Hour, 24*time.Hour)
	tokens, err := service.Login(context.Background(), "owner@example.test", "correct-password", "", "")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	if err := service.Logout(context.Background(), tokens.Session.ID); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if repo.sessions[tokens.Session.ID].RevokedAt == nil {
		t.Fatal("Logout() did not revoke the session")
	}
	if _, err := service.VerifyAccessToken(context.Background(), tokens.AccessToken); !errors.Is(err, ErrInvalidAccessToken) {
		t.Fatalf("VerifyAccessToken() after logout error = %v, want %v", err, ErrInvalidAccessToken)
	}
}

func TestAdminService_BootstrapDisabledWhenOwnerExists(t *testing.T) {
	repo := newFakeAdminRepo(t)
	repo.ownerExists = true
	service := NewAdminService(repo, "secret", time.Hour, 24*time.Hour)

	user, err := service.BootstrapOwner(context.Background(), "new@example.test", "New Owner", "new-password")
	if !errors.Is(err, ErrOwnerExists) {
		t.Fatalf("BootstrapOwner() error = %v, want %v", err, ErrOwnerExists)
	}
	if user != nil {
		t.Fatal("BootstrapOwner() returned a user when an owner exists")
	}
}
