package dto

import "time"

type AdminLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AdminRefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type BootstrapOwnerRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

type AdminUserResponse struct {
	ID           string   `json:"id"`
	Email        string   `json:"email,omitempty"`
	DisplayName  string   `json:"display_name"`
	IsActive     bool     `json:"is_active"`
	IsSuperAdmin bool     `json:"is_super_admin"`
	Permissions  []string `json:"permissions,omitempty"`
}

type AdminAuthResponse struct {
	AccessToken           string            `json:"access_token"`
	RefreshToken          string            `json:"refresh_token"`
	TokenType             string            `json:"token_type"`
	AccessTokenExpiresAt  time.Time         `json:"access_token_expires_at"`
	RefreshTokenExpiresAt time.Time         `json:"refresh_token_expires_at"`
	User                  AdminUserResponse `json:"user"`
}

type CurrentAdminUserResponse struct {
	User      AdminUserResponse `json:"user"`
	SessionID string            `json:"session_id"`
}
