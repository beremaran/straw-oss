package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultAdminAccessTTL  = 15 * time.Minute
	defaultAdminRefreshTTL = 7 * 24 * time.Hour
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidAccessToken = errors.New("invalid access token")
	ErrInvalidRefresh     = errors.New("invalid refresh token")
	ErrRefreshReuse       = errors.New("refresh token reused")
	ErrOwnerExists        = errors.New("active owner already exists")
	ErrWeakPassword       = errors.New("password must be at least 8 characters")
)

type AdminIdentityRepository interface {
	CreateUser(ctx context.Context, user *domain.AdminUser) error
	UpdateUser(ctx context.Context, user *domain.AdminUser) error
	GetUserByID(ctx context.Context, id string) (*domain.AdminUser, error)
	GetUserByEmail(ctx context.Context, email string) (*domain.AdminUser, error)
	SetUserRoles(ctx context.Context, userID string, roleIDs []string) error
	EffectivePermissions(ctx context.Context, userID string) ([]string, error)
	ActiveOwnerExists(ctx context.Context) (bool, error)
	GetRoleByName(ctx context.Context, name string) (*domain.AdminRole, error)
	CreateSession(ctx context.Context, session *domain.AdminSession) error
	GetSessionByID(ctx context.Context, id string) (*domain.AdminSession, error)
	UpdateSessionRefreshHash(ctx context.Context, id, hash string) error
	RevokeSession(ctx context.Context, id string) error
}

type AdminService struct {
	repo       AdminIdentityRepository
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

type TokenPair struct {
	AccessToken           string
	RefreshToken          string
	AccessTokenExpiresAt  time.Time
	RefreshTokenExpiresAt time.Time
	User                  *domain.AdminUser
	Session               *domain.AdminSession
	Permissions           []string
}

type AccessClaims struct {
	UserID      string   `json:"sub"`
	SessionID   string   `json:"sid"`
	Email       string   `json:"email"`
	DisplayName string   `json:"name"`
	Permissions []string `json:"perms"`
	ExpiresAt   int64    `json:"exp"`
}

func NewAdminService(repo AdminIdentityRepository, secret string, accessTTL, refreshTTL time.Duration) *AdminService {
	if accessTTL <= 0 {
		accessTTL = defaultAdminAccessTTL
	}
	if refreshTTL <= 0 {
		refreshTTL = defaultAdminRefreshTTL
	}

	return &AdminService{
		repo:       repo,
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

func HashAdminPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", ErrWeakPassword
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

func (s *AdminService) Login(ctx context.Context, email, password, userAgent, ip string) (*TokenPair, error) {
	user, err := s.repo.GetUserByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive || user.PasswordHash == "" {
		return nil, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}

	now := time.Now()
	user.LastLoginAt = &now
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}

	return s.issueSession(ctx, user, userAgent, ip, uuid.New().String())
}

func (s *AdminService) Refresh(ctx context.Context, rawRefreshToken string) (*TokenPair, error) {
	sessionID, ok := refreshSessionID(rawRefreshToken)
	if !ok {
		return nil, ErrInvalidRefresh
	}

	session, err := s.repo.GetSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil || session.RevokedAt != nil || time.Now().After(session.ExpiresAt) {
		return nil, ErrInvalidRefresh
	}

	if !sameHash(session.RefreshTokenHash, sha256Hash(rawRefreshToken)) {
		// ponytail: one admin_sessions row is the refresh family; add token history only if multi-device families need it.
		_ = s.repo.RevokeSession(ctx, session.ID)

		return nil, ErrRefreshReuse
	}

	user, err := s.repo.GetUserByID(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive {
		return nil, ErrInvalidRefresh
	}

	return s.rotateSession(ctx, user, session)
}

func (s *AdminService) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return ErrInvalidAccessToken
	}

	return s.repo.RevokeSession(ctx, sessionID)
}

func (s *AdminService) BootstrapOwner(ctx context.Context, email, displayName, password string) (*domain.AdminUser, error) {
	exists, err := s.repo.ActiveOwnerExists(ctx)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrOwnerExists
	}

	passwordHash, err := HashAdminPassword(password)
	if err != nil {
		return nil, err
	}

	role, err := s.repo.GetRoleByName(ctx, domain.RoleOwner)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, fmt.Errorf("owner role not found")
	}

	name := strings.TrimSpace(displayName)
	if name == "" {
		name = strings.TrimSpace(email)
	}

	user := &domain.AdminUser{
		ID:           uuid.New().String(),
		Email:        strings.TrimSpace(email),
		DisplayName:  name,
		PasswordHash: passwordHash,
		IsActive:     true,
	}
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}
	if err := s.repo.SetUserRoles(ctx, user.ID, []string{role.ID}); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *AdminService) VerifyAccessToken(ctx context.Context, rawToken string) (*AccessClaims, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 || len(s.secret) == 0 {
		return nil, ErrInvalidAccessToken
	}

	signed := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidAccessToken
	}
	if !hmac.Equal(sig, s.sign([]byte(signed))) {
		return nil, ErrInvalidAccessToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidAccessToken
	}

	var claims AccessClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidAccessToken
	}
	now := time.Now()
	if claims.UserID == "" || claims.SessionID == "" || now.Unix() >= claims.ExpiresAt {
		return nil, ErrInvalidAccessToken
	}

	session, err := s.repo.GetSessionByID(ctx, claims.SessionID)
	if err != nil {
		return nil, err
	}
	if session == nil || session.RevokedAt != nil || now.After(session.ExpiresAt) {
		return nil, ErrInvalidAccessToken
	}

	return &claims, nil
}

func (s *AdminService) issueSession(ctx context.Context, user *domain.AdminUser, userAgent, ip, sessionID string) (*TokenPair, error) {
	permissions, err := s.repo.EffectivePermissions(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	refreshToken, err := newRefreshToken(sessionID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	session := &domain.AdminSession{
		ID:               sessionID,
		UserID:           user.ID,
		RefreshTokenHash: sha256Hash(refreshToken),
		UserAgent:        userAgent,
		IP:               ip,
		ExpiresAt:        now.Add(s.refreshTTL),
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, err
	}

	accessToken, accessExpiresAt, err := s.newAccessToken(user, session.ID, permissions)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:           accessToken,
		RefreshToken:          refreshToken,
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshTokenExpiresAt: session.ExpiresAt,
		User:                  user,
		Session:               session,
		Permissions:           permissions,
	}, nil
}

func (s *AdminService) rotateSession(ctx context.Context, user *domain.AdminUser, session *domain.AdminSession) (*TokenPair, error) {
	permissions, err := s.repo.EffectivePermissions(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	refreshToken, err := newRefreshToken(session.ID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateSessionRefreshHash(ctx, session.ID, sha256Hash(refreshToken)); err != nil {
		return nil, err
	}

	accessToken, accessExpiresAt, err := s.newAccessToken(user, session.ID, permissions)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:           accessToken,
		RefreshToken:          refreshToken,
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshTokenExpiresAt: session.ExpiresAt,
		User:                  user,
		Session:               session,
		Permissions:           permissions,
	}, nil
}

func (s *AdminService) newAccessToken(user *domain.AdminUser, sessionID string, permissions []string) (string, time.Time, error) {
	if len(s.secret) == 0 {
		return "", time.Time{}, ErrInvalidAccessToken
	}

	expiresAt := time.Now().Add(s.accessTTL)
	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", time.Time{}, err
	}
	payload, err := json.Marshal(AccessClaims{
		UserID:      user.ID,
		SessionID:   sessionID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Permissions: permissions,
		ExpiresAt:   expiresAt.Unix(),
	})
	if err != nil {
		return "", time.Time{}, err
	}

	signed := encodeSegment(header) + "." + encodeSegment(payload)
	signature := base64.RawURLEncoding.EncodeToString(s.sign([]byte(signed)))

	return signed + "." + signature, expiresAt, nil
}

func (s *AdminService) sign(data []byte) []byte {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write(data)

	return mac.Sum(nil)
}

func newRefreshToken(sessionID string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return sessionID + "." + hex.EncodeToString(b), nil
}

func refreshSessionID(raw string) (string, bool) {
	sessionID, secret, ok := strings.Cut(raw, ".")

	return sessionID, ok && sessionID != "" && secret != ""
}

func encodeSegment(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func sameHash(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}
