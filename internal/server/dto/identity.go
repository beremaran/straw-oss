package dto

import (
	"time"

	"github.com/beremaran/straw/internal/domain"
)

// User DTOs

// CreateUserRequest is the request body for creating a new user.
type CreateUserRequest struct {
	Email        string   `json:"email"`
	DisplayName  string   `json:"display_name"`
	Password     string   `json:"password,omitempty"`
	IsActive     bool     `json:"is_active"`
	IsSuperAdmin bool     `json:"is_super_admin"`
	RoleIDs      []string `json:"role_ids"`
}

// UpdateUserRequest is the request body for updating a user.
type UpdateUserRequest struct {
	Email        *string   `json:"email,omitempty"`
	DisplayName  *string   `json:"display_name,omitempty"`
	Password     *string   `json:"password,omitempty"`
	IsActive     *bool     `json:"is_active,omitempty"`
	IsSuperAdmin *bool     `json:"is_super_admin,omitempty"`
	RoleIDs      *[]string `json:"role_ids,omitempty"`
}

// UserDetailResponse represents a user with full details in API responses.
type UserDetailResponse struct {
	ID           string              `json:"id"`
	Email        string              `json:"email"`
	DisplayName  string              `json:"display_name"`
	IsActive     bool                `json:"is_active"`
	IsSuperAdmin bool                `json:"is_super_admin"`
	LastLoginAt  *time.Time          `json:"last_login_at,omitempty"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	Roles        []AdminRoleResponse `json:"roles"`
}

// ListUsersResponse is a paginated list of users.
type ListUsersResponse struct {
	Data  []AdminUserResponse `json:"data"`
	Total int                 `json:"total"`
	Page  int                 `json:"page"`
	Limit int                 `json:"limit"`
}

// Role DTOs

// CreateRoleRequest is the request body for creating a new role.
type CreateRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Permissions []string `json:"permissions"`
}

// UpdateRoleRequest is the request body for updating a role.
type UpdateRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Permissions []string `json:"permissions"`
}

// AdminRoleResponse represents a role in API responses.
type AdminRoleResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	IsBuiltin   bool      `json:"is_builtin"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ListRolesResponse is a list of roles.
type ListRolesResponse struct {
	Data []AdminRoleResponse `json:"data"`
}

// Identity Provider DTOs

// CreateIdentityProviderRequest is the request body for creating a new identity provider.
type CreateIdentityProviderRequest struct {
	Name            string         `json:"name"`
	Type            string         `json:"type"`
	IssuerURL       string         `json:"issuer_url,omitempty"`
	ClientID        string         `json:"client_id,omitempty"`
	ClientSecretRef string         `json:"client_secret_ref,omitempty"`
	JWKSURL         string         `json:"jwks_url,omitempty"`
	Scopes          []string       `json:"scopes,omitempty"`
	RoleClaim       string         `json:"role_claim,omitempty"`
	DefaultRoleID   string         `json:"default_role_id,omitempty"`
	IsEnabled       bool           `json:"is_enabled"`
	Config          map[string]any `json:"config,omitempty"`
}

// UpdateIdentityProviderRequest is the request body for updating an identity provider.
type UpdateIdentityProviderRequest struct {
	Name            string         `json:"name"`
	Type            string         `json:"type"`
	IssuerURL       string         `json:"issuer_url,omitempty"`
	ClientID        string         `json:"client_id,omitempty"`
	ClientSecretRef string         `json:"client_secret_ref,omitempty"`
	JWKSURL         string         `json:"jwks_url,omitempty"`
	Scopes          []string       `json:"scopes,omitempty"`
	RoleClaim       string         `json:"role_claim,omitempty"`
	DefaultRoleID   string         `json:"default_role_id,omitempty"`
	IsEnabled       bool           `json:"is_enabled"`
	Config          map[string]any `json:"config,omitempty"`
}

// AdminIdentityProviderResponse represents an identity provider in API responses.
type AdminIdentityProviderResponse struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Type            string         `json:"type"`
	IssuerURL       string         `json:"issuer_url,omitempty"`
	ClientID        string         `json:"client_id,omitempty"`
	ClientSecretRef string         `json:"client_secret_ref,omitempty"`
	JWKSURL         string         `json:"jwks_url,omitempty"`
	Scopes          []string       `json:"scopes,omitempty"`
	RoleClaim       string         `json:"role_claim,omitempty"`
	DefaultRoleID   string         `json:"default_role_id,omitempty"`
	IsEnabled       bool           `json:"is_enabled"`
	Config          map[string]any `json:"config,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// ListIdentityProvidersResponse is a list of identity providers.
type ListIdentityProvidersResponse struct {
	Data []AdminIdentityProviderResponse `json:"data"`
}

// Mapper helpers

// FromDomainUser converts a domain user to a DTO.
func FromDomainUser(user domain.AdminUser) AdminUserResponse {
	return AdminUserResponse{
		ID:           user.ID,
		Email:        user.Email,
		DisplayName:  user.DisplayName,
		IsActive:     user.IsActive,
		IsSuperAdmin: user.IsSuperAdmin,
	}
}

// FromDomainRole converts a domain role to a DTO.
func FromDomainRole(role domain.AdminRole) AdminRoleResponse {
	return AdminRoleResponse{
		ID:          role.ID,
		Name:        role.Name,
		Description: role.Description,
		IsBuiltin:   role.IsBuiltin,
		Permissions: role.Permissions,
		CreatedAt:   role.CreatedAt,
		UpdatedAt:   role.UpdatedAt,
	}
}

// FromDomainIdentityProvider converts a domain identity provider to a DTO.
func FromDomainIdentityProvider(provider domain.AdminIdentityProvider) AdminIdentityProviderResponse {
	return AdminIdentityProviderResponse{
		ID:              provider.ID,
		Name:            provider.Name,
		Type:            provider.Type,
		IssuerURL:       provider.IssuerURL,
		ClientID:        provider.ClientID,
		ClientSecretRef: provider.ClientSecretRef,
		JWKSURL:         provider.JWKSURL,
		Scopes:          provider.Scopes,
		RoleClaim:       provider.RoleClaim,
		DefaultRoleID:   provider.DefaultRoleID,
		IsEnabled:       provider.IsEnabled,
		Config:          map[string]any(provider.Config),
		CreatedAt:       provider.CreatedAt,
		UpdatedAt:       provider.UpdatedAt,
	}
}
