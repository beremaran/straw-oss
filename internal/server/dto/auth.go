package dto

import "time"

// AdminLoginRequest is the request body for admin login.
type AdminLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AdminRefreshRequest is the request body for refreshing an admin session.
type AdminRefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// BootstrapOwnerRequest is the request body for bootstrapping the first owner user.
type BootstrapOwnerRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

// AdminUserResponse represents an admin user in API responses.
type AdminUserResponse struct {
	ID           string   `json:"id"`
	Email        string   `json:"email,omitempty"`
	DisplayName  string   `json:"display_name"`
	IsActive     bool     `json:"is_active"`
	IsSuperAdmin bool     `json:"is_super_admin"`
	Permissions  []string `json:"permissions,omitempty"`
}

// AdminAuthResponse is the response body for admin authentication.
type AdminAuthResponse struct {
	AccessToken           string            `json:"access_token"`
	RefreshToken          string            `json:"refresh_token"`
	TokenType             string            `json:"token_type"`
	AccessTokenExpiresAt  time.Time         `json:"access_token_expires_at"`
	RefreshTokenExpiresAt time.Time         `json:"refresh_token_expires_at"`
	User                  AdminUserResponse `json:"user"`
}

// CurrentAdminUserResponse represents the current authenticated admin user.
type CurrentAdminUserResponse struct {
	User      AdminUserResponse `json:"user"`
	SessionID string            `json:"session_id"`
}
