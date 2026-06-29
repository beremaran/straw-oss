package handlers

import (
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	adminauth "github.com/beremaran/straw/internal/service/auth"
)

const (
	ssoStateCookieMaxAge = 300
	ssoStateParts        = 3
)

// AuthHandler manages admin authentication operations.
type AuthHandler struct {
	service *adminauth.AdminService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(service *adminauth.AdminService) *AuthHandler {
	return &AuthHandler{service: service}
}

// HandleLogin authenticates an admin user.
func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req dto.AdminLoginRequest

	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	if strings.TrimSpace(req.Email) == "" || req.Password == "" {
		helper.WriteError(w, http.StatusBadRequest, "email and password are required")

		return
	}

	tokens, err := h.service.Login(r.Context(), req.Email, req.Password, r.UserAgent(), clientIP(r))
	if errors.Is(err, adminauth.ErrInvalidCredentials) {
		helper.WriteError(w, http.StatusUnauthorized, "invalid credentials")

		return
	}

	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to login")

		return
	}

	helper.WriteJSON(w, http.StatusOK, authResponse(tokens))
}

// HandleRefresh refreshes an admin session.
func (h *AuthHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	var req dto.AdminRefreshRequest

	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	if req.RefreshToken == "" {
		helper.WriteError(w, http.StatusBadRequest, "refresh_token is required")

		return
	}

	tokens, err := h.service.Refresh(r.Context(), req.RefreshToken)
	if errors.Is(err, adminauth.ErrInvalidRefresh) || errors.Is(err, adminauth.ErrRefreshReuse) {
		helper.WriteError(w, http.StatusUnauthorized, "invalid refresh token")

		return
	}

	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to refresh token")

		return
	}

	helper.WriteJSON(w, http.StatusOK, authResponse(tokens))
}

// HandleLogout ends an admin session.
func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFromContext(r.Context())
	if !ok || actor.Type != middleware.ActorTypeUser {
		helper.WriteError(w, http.StatusUnauthorized, "session token required")

		return
	}

	err := h.service.Logout(r.Context(), actor.SessionID)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to logout")

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleMe returns the current admin user's details.
func (h *AuthHandler) HandleMe(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFromContext(r.Context())
	if !ok || actor.Type != middleware.ActorTypeUser {
		helper.WriteError(w, http.StatusUnauthorized, "session token required")

		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.CurrentAdminUserResponse{
		User: dto.AdminUserResponse{
			ID:          actor.ID,
			Email:       actor.Email,
			DisplayName: actor.DisplayName,
			IsActive:    true,
			Permissions: actor.Permissions,
		},
		SessionID: actor.SessionID,
	})
}

// HandleBootstrapOwner creates the initial owner user.
func (h *AuthHandler) HandleBootstrapOwner(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFromContext(r.Context())
	if !ok || actor.Type != middleware.ActorTypeLegacy {
		helper.WriteError(w, http.StatusUnauthorized, "legacy management token required")

		return
	}

	var req dto.BootstrapOwnerRequest

	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	if strings.TrimSpace(req.Email) == "" || req.Password == "" {
		helper.WriteError(w, http.StatusBadRequest, "email and password are required")

		return
	}

	user, err := h.service.BootstrapOwner(r.Context(), req.Email, req.DisplayName, req.Password)
	if errors.Is(err, adminauth.ErrOwnerExists) {
		helper.WriteError(w, http.StatusConflict, "bootstrap owner already exists")

		return
	}

	if errors.Is(err, adminauth.ErrWeakPassword) {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to bootstrap owner")

		return
	}

	helper.WriteJSON(w, http.StatusCreated, dto.AdminUserResponse{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		IsActive:    user.IsActive,
	})
}

func authResponse(tokens *adminauth.TokenPair) dto.AdminAuthResponse {
	return dto.AdminAuthResponse{
		AccessToken:           tokens.AccessToken,
		RefreshToken:          tokens.RefreshToken,
		TokenType:             "Bearer",
		AccessTokenExpiresAt:  tokens.AccessTokenExpiresAt,
		RefreshTokenExpiresAt: tokens.RefreshTokenExpiresAt,
		User:                  userResponse(tokens.User, tokens.Permissions),
	}
}

func userResponse(user *domain.AdminUser, permissions []string) dto.AdminUserResponse {
	return dto.AdminUserResponse{
		ID:           user.ID,
		Email:        user.Email,
		DisplayName:  user.DisplayName,
		IsActive:     user.IsActive,
		IsSuperAdmin: user.IsSuperAdmin,
		Permissions:  permissions,
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}

func redirectURL(target string) (*url.URL, bool) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, false
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, false
	}

	if u.Host == "" {
		return nil, false
	}

	return u, true
}

func redirectWithTokens(target string, tokens *adminauth.TokenPair) (*url.URL, bool) {
	u, ok := redirectURL(target)
	if !ok {
		return nil, false
	}

	fragment := url.Values{}
	fragment.Set("access_token", tokens.AccessToken)
	fragment.Set("refresh_token", tokens.RefreshToken)
	u.Fragment = fragment.Encode()

	return u, true
}

func performRedirect(w http.ResponseWriter, u *url.URL) {
	w.Header().Set("Location", u.String())
	w.WriteHeader(http.StatusFound)
}

// HandleSSOStart begins the SSO OAuth flow.
func (h *AuthHandler) HandleSSOStart(w http.ResponseWriter, r *http.Request) {
	providerName := r.PathValue("provider")

	redirectURI := r.URL.Query().Get("redirect_uri")
	if redirectURI == "" {
		helper.WriteError(w, http.StatusBadRequest, "redirect_uri is required")

		return
	}

	authURL, state, nonce, err := h.service.StartSSO(r.Context(), providerName, redirectURI)
	if err != nil {
		if errors.Is(err, adminauth.ErrProviderNotFound) {
			helper.WriteError(w, http.StatusNotFound, "provider not found")

			return
		}

		if errors.Is(err, adminauth.ErrProviderDisabled) {
			helper.WriteError(w, http.StatusForbidden, "provider is disabled")

			return
		}

		helper.WriteError(w, http.StatusInternalServerError, "failed to start sso")

		return
	}

	cookiePayload := state + ":" + nonce + ":" + base64.RawURLEncoding.EncodeToString([]byte(redirectURI))

	http.SetCookie(w, &http.Cookie{
		Name:     "sso_state",
		Value:    cookiePayload,
		Path:     "/management/auth/sso",
		MaxAge:   ssoStateCookieMaxAge,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	target, ok := redirectURL(authURL)
	if !ok {
		helper.WriteError(w, http.StatusInternalServerError, "invalid auth url")

		return
	}

	performRedirect(w, target)
}

// HandleSSOCallback completes the SSO OAuth flow.
func (h *AuthHandler) HandleSSOCallback(w http.ResponseWriter, r *http.Request) {
	providerName := r.PathValue("provider")
	code := r.URL.Query().Get("code")
	actualState := r.URL.Query().Get("state")
	errDesc := r.URL.Query().Get("error_description")
	errStr := r.URL.Query().Get("error")

	if errStr != "" {
		helper.WriteError(w, http.StatusUnauthorized, "provider error: "+errDesc)

		return
	}

	cookie, err := r.Cookie("sso_state")
	if err != nil || cookie.Value == "" {
		helper.WriteError(w, http.StatusBadRequest, "missing or invalid state cookie")

		return
	}

	parts := strings.Split(cookie.Value, ":")
	if len(parts) != ssoStateParts {
		helper.WriteError(w, http.StatusBadRequest, "invalid state cookie format")

		return
	}

	expectedState := parts[0]
	expectedNonce := parts[1]

	redirectURIRaw, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid redirect_uri in cookie")

		return
	}

	redirectURI := string(redirectURIRaw)

	h.clearSSOStateCookie(w)

	h.processSSOCallback(w, r, providerName, redirectURI, code, expectedState, expectedNonce, actualState)
}

func (h *AuthHandler) clearSSOStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "sso_state",
		Value:    "",
		Path:     "/management/auth/sso",
		MaxAge:   -1,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandler) processSSOCallback(w http.ResponseWriter, r *http.Request, providerName, redirectURI, code, expectedState, expectedNonce, actualState string) {
	tokens, err := h.service.CallbackSSO(r.Context(), providerName, redirectURI, code, expectedState, expectedNonce, actualState, r.UserAgent(), clientIP(r))
	if err != nil {
		helper.WriteError(w, http.StatusUnauthorized, "sso failed: "+err.Error())

		return
	}

	target, ok := redirectWithTokens(redirectURI, tokens)
	if !ok {
		helper.WriteError(w, http.StatusBadRequest, "invalid redirect url")

		return
	}

	performRedirect(w, target)
}
