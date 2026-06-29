package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

var (
	ErrNoEmailClaim = errors.New("no email claim provided")
)

var (
	ErrProviderNotFound = errors.New("identity provider not found")
	ErrProviderDisabled = errors.New("identity provider is disabled")
	ErrInvalidState     = errors.New("invalid state")
	ErrInvalidIDToken   = errors.New("invalid id token")
)

type ssoClaims struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Role          string `json:"role"`
}

func (s *AdminService) getOAuth2Config(ctx context.Context, provider *domain.AdminIdentityProvider, redirectURI string) (*oauth2.Config, *oidc.Provider, error) {
	providerCtx := oidc.ClientContext(ctx, nil)
	var oidcProvider *oidc.Provider
	var err error
	if provider.IssuerURL != "" {
		oidcProvider, err = oidc.NewProvider(providerCtx, provider.IssuerURL)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get oidc provider: %w", err)
		}
	}

	scopes := provider.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}

	oauth2Config := &oauth2.Config{
		ClientID:     provider.ClientID,
		ClientSecret: provider.ClientSecretRef,
		Endpoint:     oidcProvider.Endpoint(),
		RedirectURL:  redirectURI,
		Scopes:       scopes,
	}

	return oauth2Config, oidcProvider, nil
}

func (s *AdminService) StartSSO(ctx context.Context, providerName string, redirectURI string) (string, string, string, error) {
	provider, err := s.repo.GetIdentityProviderByName(ctx, providerName)
	if err != nil {
		return "", "", "", err
	}
	if provider == nil {
		return "", "", "", ErrProviderNotFound
	}
	if !provider.IsEnabled {
		return "", "", "", ErrProviderDisabled
	}

	oauth2Config, _, err := s.getOAuth2Config(ctx, provider, redirectURI)
	if err != nil {
		return "", "", "", err
	}

	state, err := generateRandomString(32)
	if err != nil {
		return "", "", "", err
	}

	nonce, err := generateRandomString(32)
	if err != nil {
		return "", "", "", err
	}

	authURL := oauth2Config.AuthCodeURL(state, oidc.Nonce(nonce))

	return authURL, state, nonce, nil
}

func (s *AdminService) CallbackSSO(ctx context.Context, providerName string, redirectURI string, code string, expectedState string, expectedNonce string, actualState string, userAgent string, ip string) (*TokenPair, error) {
	if actualState != expectedState {
		return nil, ErrInvalidState
	}

	provider, err := s.enabledIdentityProvider(ctx, providerName)
	if err != nil {
		return nil, err
	}

	oauth2Config, oidcProvider, err := s.getOAuth2Config(ctx, provider, redirectURI)
	if err != nil {
		return nil, err
	}

	oauth2Token, err := oauth2Config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %w", err)
	}

	claims, err := readSSOClaims(ctx, oidcProvider, provider, oauth2Token, expectedNonce)
	if err != nil {
		return nil, err
	}

	user, err := s.userForSSO(ctx, provider, claims)
	if err != nil {
		return nil, err
	}

	return s.issueSession(ctx, user, userAgent, ip, uuid.New().String())
}

func (s *AdminService) enabledIdentityProvider(ctx context.Context, providerName string) (*domain.AdminIdentityProvider, error) {
	provider, err := s.repo.GetIdentityProviderByName(ctx, providerName)
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, ErrProviderNotFound
	}
	if !provider.IsEnabled {
		return nil, ErrProviderDisabled
	}

	return provider, nil
}

func readSSOClaims(ctx context.Context, oidcProvider *oidc.Provider, provider *domain.AdminIdentityProvider, oauth2Token *oauth2.Token, expectedNonce string) (ssoClaims, error) {
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return ssoClaims{}, ErrInvalidIDToken
	}

	verifier := oidcProvider.Verifier(&oidc.Config{ClientID: provider.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return ssoClaims{}, fmt.Errorf("failed to verify id token: %w", err)
	}

	if idToken.Nonce != expectedNonce {
		return ssoClaims{}, ErrInvalidIDToken
	}

	var claims ssoClaims
	err = idToken.Claims(&claims)
	if err != nil {
		return ssoClaims{}, err
	}
	claims.Role = roleFromIDToken(idToken, provider.RoleClaim, claims.Role)

	if claims.Email == "" {
		return ssoClaims{}, ErrNoEmailClaim
	}

	return claims, nil
}

func roleFromIDToken(idToken *oidc.IDToken, claimName string, fallback string) string {
	if claimName == "" {
		return fallback
	}

	var allClaims map[string]interface{}
	err := idToken.Claims(&allClaims)
	if err != nil {
		return fallback
	}

	role := stringClaim(allClaims[claimName])
	if role == "" {
		return fallback
	}

	return role
}

func stringClaim(value interface{}) string {
	if text, ok := value.(string); ok {
		return text
	}

	values, ok := value.([]interface{})
	if !ok || len(values) == 0 {
		return ""
	}

	text, _ := values[0].(string)

	return text
}

func (s *AdminService) userForSSO(ctx context.Context, provider *domain.AdminIdentityProvider, claims ssoClaims) (*domain.AdminUser, error) {
	user, err := s.repo.GetUserByEmail(ctx, claims.Email)
	if err != nil {
		return nil, err
	}

	if user == nil {
		return s.createSSOUser(ctx, provider, claims)
	}

	return s.updateSSOLogin(ctx, user)
}

func (s *AdminService) createSSOUser(ctx context.Context, provider *domain.AdminIdentityProvider, claims ssoClaims) (*domain.AdminUser, error) {
	user := &domain.AdminUser{
		ID:          uuid.New().String(),
		Email:       claims.Email,
		DisplayName: claims.Name,
		IsActive:    true,
	}
	if user.DisplayName == "" {
		user.DisplayName = user.Email
	}

	err := s.repo.CreateUser(ctx, user)
	if err != nil {
		return nil, err
	}

	err = s.repo.SetUserRoles(ctx, user.ID, defaultSSORoleIDs(provider))
	if err != nil {
		return nil, err
	}

	return user, nil
}

func defaultSSORoleIDs(provider *domain.AdminIdentityProvider) []string {
	if provider.DefaultRoleID == "" {
		return nil
	}

	return []string{provider.DefaultRoleID}
}

func (s *AdminService) updateSSOLogin(ctx context.Context, user *domain.AdminUser) (*domain.AdminUser, error) {
	if !user.IsActive {
		return nil, ErrInvalidCredentials
	}

	now := time.Now()
	user.LastLoginAt = &now
	err := s.repo.UpdateUser(ctx, user)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}
