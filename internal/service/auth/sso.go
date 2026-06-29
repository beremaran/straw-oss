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
	ErrProviderNotFound = errors.New("identity provider not found")
	ErrProviderDisabled = errors.New("identity provider is disabled")
	ErrInvalidState     = errors.New("invalid state")
	ErrInvalidIDToken   = errors.New("invalid id token")
)

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

	oauth2Config, oidcProvider, err := s.getOAuth2Config(ctx, provider, redirectURI)
	if err != nil {
		return nil, err
	}

	oauth2Token, err := oauth2Config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %w", err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, ErrInvalidIDToken
	}

	verifier := oidcProvider.Verifier(&oidc.Config{ClientID: provider.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify id token: %w", err)
	}

	if idToken.Nonce != expectedNonce {
		return nil, ErrInvalidIDToken
	}

	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Role          string `json:"role"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, err
	}

	if provider.RoleClaim != "" {
		var allClaims map[string]interface{}
		if err := idToken.Claims(&allClaims); err == nil {
			if val, ok := allClaims[provider.RoleClaim].(string); ok {
				claims.Role = val
			} else if val, ok := allClaims[provider.RoleClaim].([]interface{}); ok && len(val) > 0 {
				if sVal, ok := val[0].(string); ok {
					claims.Role = sVal
				}
			}
		}
	}

	if claims.Email == "" {
		return nil, errors.New("no email claim provided")
	}

	user, err := s.repo.GetUserByEmail(ctx, claims.Email)
	if err != nil {
		return nil, err
	}

	if user == nil {
		roleID := provider.DefaultRoleID
		
		user = &domain.AdminUser{
			ID:          uuid.New().String(),
			Email:       claims.Email,
			DisplayName: claims.Name,
			IsActive:    true,
		}
		if user.DisplayName == "" {
			user.DisplayName = user.Email
		}
		if err := s.repo.CreateUser(ctx, user); err != nil {
			return nil, err
		}
		
		var roleIDs []string
		if roleID != "" {
			roleIDs = append(roleIDs, roleID)
		}
		if err := s.repo.SetUserRoles(ctx, user.ID, roleIDs); err != nil {
			return nil, err
		}
	} else {
		if !user.IsActive {
			return nil, ErrInvalidCredentials
		}
		now := time.Now()
		user.LastLoginAt = &now
		if err := s.repo.UpdateUser(ctx, user); err != nil {
			return nil, err
		}
	}

	return s.issueSession(ctx, user, userAgent, ip, uuid.New().String())
}

func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
