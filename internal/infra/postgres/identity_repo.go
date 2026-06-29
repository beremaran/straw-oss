package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/beremaran/straw/internal/domain"
)

var (
	// ErrIdentityNotFound is returned when an identity row is not found.
	ErrIdentityNotFound = errors.New("identity row not found")
	// ErrBuiltinRoleProtected is returned when attempting to delete a built-in role.
	ErrBuiltinRoleProtected = errors.New("built-in role cannot be deleted")
	// ErrPlaintextProviderSecret is returned when an identity provider config contains a plaintext secret.
	ErrPlaintextProviderSecret = errors.New("identity provider config must not contain secrets")
)

// IdentityRepository persists and retrieves admin identities (users, roles, sessions, identity providers).
type IdentityRepository struct {
	client *Client
}

// NewIdentityRepository creates a new IdentityRepository backed by the given client.
func NewIdentityRepository(client *Client) *IdentityRepository {
	return &IdentityRepository{client: client}
}

// CreateUser inserts a new admin user.
func (r *IdentityRepository) CreateUser(ctx context.Context, user *domain.AdminUser) error {
	now := time.Now()

	if user.ID == "" {
		user.ID = uuid.New().String()
	}

	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}

	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = now
	}

	query := `
		INSERT INTO admin_users (
			id, email, display_name, password_hash, is_active, is_super_admin,
			last_login_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	err := r.client.Execute(func() error {
		_, err := r.client.Pool.Exec(ctx, query,
			user.ID,
			user.Email,
			user.DisplayName,
			nilString(user.PasswordHash),
			user.IsActive,
			user.IsSuperAdmin,
			user.LastLoginAt,
			user.CreatedAt,
			user.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert user: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}

	return nil
}

// UpdateUser modifies an existing admin user.
func (r *IdentityRepository) UpdateUser(ctx context.Context, user *domain.AdminUser) error {
	user.UpdatedAt = time.Now()
	query := `
		UPDATE admin_users
		SET email = $2,
			display_name = $3,
			password_hash = $4,
			is_active = $5,
			is_super_admin = $6,
			last_login_at = $7,
			updated_at = $8
		WHERE id = $1
	`

	var rows int64

	err := r.client.Execute(func() error {
		res, err := r.client.Pool.Exec(ctx, query,
			user.ID,
			user.Email,
			user.DisplayName,
			nilString(user.PasswordHash),
			user.IsActive,
			user.IsSuperAdmin,
			user.LastLoginAt,
			user.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to execute update: %w", err)
		}

		rows = res.RowsAffected()

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to update admin user: %w", err)
	}

	if rows == 0 {
		return ErrIdentityNotFound
	}

	return nil
}

// GetUserByID returns the admin user with the given ID.
func (r *IdentityRepository) GetUserByID(ctx context.Context, id string) (*domain.AdminUser, error) {
	query := userSelectSQL() + ` WHERE id = $1`

	return r.getUser(ctx, query, id)
}

// GetUserByEmail returns the admin user with the given email.
func (r *IdentityRepository) GetUserByEmail(ctx context.Context, email string) (*domain.AdminUser, error) {
	query := userSelectSQL() + ` WHERE lower(email) = lower($1)`

	return r.getUser(ctx, query, email)
}

// ListUsers returns a paginated list of admin users.
func (r *IdentityRepository) ListUsers(ctx context.Context, limit, offset int) ([]domain.AdminUser, int, error) {
	var total int

	err := r.client.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM admin_users`).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count admin users: %w", err)
	}

	query := userSelectSQL() + `
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.client.Pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list admin users: %w", err)
	}
	defer rows.Close()

	var users []domain.AdminUser

	for rows.Next() {
		user, err := scanAdminUser(rows.Scan)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan admin user: %w", err)
		}

		users = append(users, user)
	}

	err = rows.Err()
	if err != nil {
		return nil, 0, fmt.Errorf("error iterating admin users: %w", err)
	}

	return users, total, nil
}

// SetUserRoles replaces all roles assigned to the given user.
func (r *IdentityRepository) SetUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	return r.client.Execute(func() error {
		tx, err := r.client.Pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin role assignment: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		_, err = tx.Exec(ctx, `DELETE FROM admin_user_roles WHERE user_id = $1`, userID)
		if err != nil {
			return fmt.Errorf("failed to clear user roles: %w", err)
		}

		for _, roleID := range roleIDs {
			_, err = tx.Exec(ctx, `INSERT INTO admin_user_roles (user_id, role_id) VALUES ($1, $2)`, userID, roleID)
			if err != nil {
				return fmt.Errorf("failed to assign user role: %w", err)
			}
		}

		return tx.Commit(ctx)
	})
}

// ListUserRoles returns all roles assigned to the given user.
func (r *IdentityRepository) ListUserRoles(ctx context.Context, userID string) ([]domain.AdminRole, error) {
	query := roleSelectSQL() + `
		JOIN admin_user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1
		GROUP BY r.id
		ORDER BY r.name ASC
	`

	return r.listRoles(ctx, query, userID)
}

// EffectivePermissions returns all permissions held by the given user, including those from super-admin status.
func (r *IdentityRepository) EffectivePermissions(ctx context.Context, userID string) ([]string, error) {
	var isSuperAdmin bool

	err := r.client.Pool.QueryRow(ctx,
		`SELECT is_super_admin FROM admin_users WHERE id = $1 AND is_active = true`,
		userID,
	).Scan(&isSuperAdmin)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrIdentityNotFound
		}

		return nil, fmt.Errorf("failed to load admin user permissions: %w", err)
	}

	if isSuperAdmin {
		return domain.AllPermissions(), nil
	}

	query := `
		SELECT COALESCE(
			array_agg(DISTINCT rp.permission ORDER BY rp.permission)
				FILTER (WHERE rp.permission IS NOT NULL),
			ARRAY[]::TEXT[]
		)
		FROM admin_user_roles ur
		JOIN admin_role_permissions rp ON rp.role_id = ur.role_id
		WHERE ur.user_id = $1
	`

	var permissions []string

	err = r.client.Pool.QueryRow(ctx, query, userID).Scan(&permissions)
	if err != nil {
		return nil, fmt.Errorf("failed to query effective permissions: %w", err)
	}

	return permissions, nil
}

// ActiveOwnerExists returns whether any active user has the owner role.
func (r *IdentityRepository) ActiveOwnerExists(ctx context.Context) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1
			FROM admin_users u
			JOIN admin_user_roles ur ON ur.user_id = u.id
			JOIN admin_roles r ON r.id = ur.role_id
			WHERE u.is_active = true AND r.name = $1
		)
	`

	var exists bool

	err := r.client.Pool.QueryRow(ctx, query, domain.RoleOwner).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check active owner: %w", err)
	}

	return exists, nil
}

// CountActiveOwners returns the number of active users with the owner role.
func (r *IdentityRepository) CountActiveOwners(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM admin_users u
		JOIN admin_user_roles ur ON ur.user_id = u.id
		JOIN admin_roles r ON r.id = ur.role_id
		WHERE u.is_active = true AND r.name = $1
	`

	var count int

	err := r.client.Pool.QueryRow(ctx, query, domain.RoleOwner).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count active owners: %w", err)
	}

	return count, nil
}

// CreateRole inserts a new admin role with its permissions.
func (r *IdentityRepository) CreateRole(ctx context.Context, role *domain.AdminRole) error {
	now := time.Now()

	if role.ID == "" {
		role.ID = uuid.New().String()
	}

	if role.CreatedAt.IsZero() {
		role.CreatedAt = now
	}

	if role.UpdatedAt.IsZero() {
		role.UpdatedAt = now
	}

	return r.client.Execute(func() error {
		tx, err := r.client.Pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin role create: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		_, err = tx.Exec(ctx, `
			INSERT INTO admin_roles (id, name, description, is_builtin, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, role.ID, role.Name, nilString(role.Description), role.IsBuiltin, role.CreatedAt, role.UpdatedAt)
		if err != nil {
			return fmt.Errorf("failed to create admin role: %w", err)
		}

		err = replaceRolePermissions(ctx, tx, role.ID, role.Permissions)
		if err != nil {
			return err
		}

		return tx.Commit(ctx)
	})
}

// UpdateRole modifies an existing admin role and its permissions.
func (r *IdentityRepository) UpdateRole(ctx context.Context, role *domain.AdminRole) error {
	role.UpdatedAt = time.Now()

	return r.client.Execute(func() error {
		tx, err := r.client.Pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin role update: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		res, err := tx.Exec(ctx, `
			UPDATE admin_roles
			SET name = $2, description = $3, updated_at = $4
			WHERE id = $1
		`, role.ID, role.Name, nilString(role.Description), role.UpdatedAt)
		if err != nil {
			return fmt.Errorf("failed to update admin role: %w", err)
		}

		if res.RowsAffected() == 0 {
			return ErrIdentityNotFound
		}

		err = replaceRolePermissions(ctx, tx, role.ID, role.Permissions)
		if err != nil {
			return err
		}

		return tx.Commit(ctx)
	})
}

// DeleteRole removes a non-builtin admin role by ID.
func (r *IdentityRepository) DeleteRole(ctx context.Context, id string) error {
	res, err := r.client.Pool.Exec(ctx, `DELETE FROM admin_roles WHERE id = $1 AND is_builtin = false`, id)
	if err != nil {
		return fmt.Errorf("failed to delete admin role: %w", err)
	}

	if res.RowsAffected() == 0 {
		var builtin bool

		err = r.client.Pool.QueryRow(ctx, `SELECT is_builtin FROM admin_roles WHERE id = $1`, id).Scan(&builtin)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrIdentityNotFound
		}

		if err != nil {
			return fmt.Errorf("failed to check admin role: %w", err)
		}

		if builtin {
			return ErrBuiltinRoleProtected
		}
	}

	return nil
}

// GetRoleByID returns the admin role with the given ID.
func (r *IdentityRepository) GetRoleByID(ctx context.Context, id string) (*domain.AdminRole, error) {
	query := roleSelectSQL() + `
		WHERE r.id = $1
		GROUP BY r.id
	`

	return r.getRole(ctx, query, id)
}

// GetRoleByName returns the admin role with the given name.
func (r *IdentityRepository) GetRoleByName(ctx context.Context, name string) (*domain.AdminRole, error) {
	query := roleSelectSQL() + `
		WHERE r.name = $1
		GROUP BY r.id
	`

	return r.getRole(ctx, query, name)
}

// ListRoles returns all admin roles ordered by builtin status and name.
func (r *IdentityRepository) ListRoles(ctx context.Context) ([]domain.AdminRole, error) {
	query := roleSelectSQL() + `
		GROUP BY r.id
		ORDER BY r.is_builtin DESC, r.name ASC
	`

	return r.listRoles(ctx, query)
}

// CreateSession inserts a new admin session.
func (r *IdentityRepository) CreateSession(ctx context.Context, session *domain.AdminSession) error {
	now := time.Now()

	if session.ID == "" {
		session.ID = uuid.New().String()
	}

	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}

	if session.LastUsedAt.IsZero() {
		session.LastUsedAt = now
	}

	query := `
		INSERT INTO admin_sessions (
			id, user_id, refresh_token_hash, user_agent, ip, expires_at,
			revoked_at, created_at, last_used_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	err := r.client.Execute(func() error {
		_, err := r.client.Pool.Exec(ctx, query,
			session.ID,
			session.UserID,
			session.RefreshTokenHash,
			nilString(session.UserAgent),
			nilString(session.IP),
			session.ExpiresAt,
			session.RevokedAt,
			session.CreatedAt,
			session.LastUsedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert session: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create admin session: %w", err)
	}

	return nil
}

// GetSessionByID returns the admin session with the given ID.
func (r *IdentityRepository) GetSessionByID(ctx context.Context, id string) (*domain.AdminSession, error) {
	query := sessionSelectSQL() + ` WHERE id = $1`

	var session domain.AdminSession

	err := r.client.Execute(func() error {
		var scanErr error

		session, scanErr = scanAdminSession(r.client.Pool.QueryRow(ctx, query, id).Scan)

		return scanErr
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get admin session: %w", err)
	}

	return &session, nil
}

// GetSessionByRefreshTokenHash returns the admin session with the given refresh token hash.
func (r *IdentityRepository) GetSessionByRefreshTokenHash(ctx context.Context, hash string) (*domain.AdminSession, error) {
	query := sessionSelectSQL() + ` WHERE refresh_token_hash = $1`

	var session domain.AdminSession

	err := r.client.Execute(func() error {
		var scanErr error

		session, scanErr = scanAdminSession(r.client.Pool.QueryRow(ctx, query, hash).Scan)

		return scanErr
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get admin session: %w", err)
	}

	return &session, nil
}

// UpdateSessionRefreshHash updates the refresh token hash for a session.
func (r *IdentityRepository) UpdateSessionRefreshHash(ctx context.Context, id, hash string) error {
	res, err := r.client.Pool.Exec(ctx, `
		UPDATE admin_sessions
		SET refresh_token_hash = $2, last_used_at = $3
		WHERE id = $1 AND revoked_at IS NULL
	`, id, hash, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update admin session hash: %w", err)
	}

	if res.RowsAffected() == 0 {
		return ErrIdentityNotFound
	}

	return nil
}

// RevokeSession revokes a single admin session by ID.
func (r *IdentityRepository) RevokeSession(ctx context.Context, id string) error {
	return r.revokeSessions(ctx, `id = $1`, id)
}

// RevokeUserSessions revokes all sessions for a given user.
func (r *IdentityRepository) RevokeUserSessions(ctx context.Context, userID string) error {
	return r.revokeSessions(ctx, `user_id = $1`, userID)
}

// CreateIdentityProvider inserts a new identity provider after validating its config.
func (r *IdentityRepository) CreateIdentityProvider(ctx context.Context, provider *domain.AdminIdentityProvider) error {
	err := validateProviderConfig(provider.Config)
	if err != nil {
		return err
	}

	prepareIdentityProvider(provider)

	config, err := json.Marshal(provider.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal identity provider config: %w", err)
	}

	query := `
		INSERT INTO admin_identity_providers (
			id, name, type, issuer_url, client_id, client_secret_ref,
			jwks_url, scopes, role_claim, default_role_id, is_enabled,
			config, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`

	err = r.client.Execute(func() error {
		_, err := r.client.Pool.Exec(ctx, query,
			provider.ID,
			provider.Name,
			provider.Type,
			nilString(provider.IssuerURL),
			nilString(provider.ClientID),
			nilString(provider.ClientSecretRef),
			nilString(provider.JWKSURL),
			provider.Scopes,
			nilString(provider.RoleClaim),
			nilString(provider.DefaultRoleID),
			provider.IsEnabled,
			config,
			provider.CreatedAt,
			provider.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert identity provider: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create identity provider: %w", err)
	}

	return nil
}

func prepareIdentityProvider(provider *domain.AdminIdentityProvider) {
	now := time.Now()

	if provider.ID == "" {
		provider.ID = uuid.New().String()
	}

	if provider.CreatedAt.IsZero() {
		provider.CreatedAt = now
	}

	if provider.UpdatedAt.IsZero() {
		provider.UpdatedAt = now
	}

	if provider.Scopes == nil {
		provider.Scopes = []string{"openid", "email", "profile"}
	}
}

// UpdateIdentityProvider modifies an existing identity provider after validating its config.
func (r *IdentityRepository) UpdateIdentityProvider(ctx context.Context, provider *domain.AdminIdentityProvider) error {
	err := validateProviderConfig(provider.Config)
	if err != nil {
		return err
	}

	provider.UpdatedAt = time.Now()

	config, err := json.Marshal(provider.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal identity provider config: %w", err)
	}

	res, err := r.client.Pool.Exec(ctx, `
		UPDATE admin_identity_providers
		SET name = $2,
			type = $3,
			issuer_url = $4,
			client_id = $5,
			client_secret_ref = $6,
			jwks_url = $7,
			scopes = $8,
			role_claim = $9,
			default_role_id = $10,
			is_enabled = $11,
			config = $12,
			updated_at = $13
		WHERE id = $1
	`,
		provider.ID,
		provider.Name,
		provider.Type,
		nilString(provider.IssuerURL),
		nilString(provider.ClientID),
		nilString(provider.ClientSecretRef),
		nilString(provider.JWKSURL),
		provider.Scopes,
		nilString(provider.RoleClaim),
		nilString(provider.DefaultRoleID),
		provider.IsEnabled,
		config,
		provider.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update identity provider: %w", err)
	}

	if res.RowsAffected() == 0 {
		return ErrIdentityNotFound
	}

	return nil
}

// GetIdentityProviderByID returns the identity provider with the given ID.
func (r *IdentityRepository) GetIdentityProviderByID(ctx context.Context, id string) (*domain.AdminIdentityProvider, error) {
	query := providerSelectSQL() + ` WHERE id = $1`

	var provider domain.AdminIdentityProvider

	err := r.client.Execute(func() error {
		var scanErr error

		provider, scanErr = scanIdentityProvider(r.client.Pool.QueryRow(ctx, query, id).Scan)

		return scanErr
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get identity provider by ID: %w", err)
	}

	return &provider, nil
}

// GetIdentityProviderByName returns the identity provider with the given name.
func (r *IdentityRepository) GetIdentityProviderByName(ctx context.Context, name string) (*domain.AdminIdentityProvider, error) {
	query := providerSelectSQL() + ` WHERE name = $1`

	var provider domain.AdminIdentityProvider

	err := r.client.Execute(func() error {
		var scanErr error

		provider, scanErr = scanIdentityProvider(r.client.Pool.QueryRow(ctx, query, name).Scan)

		return scanErr
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get identity provider: %w", err)
	}

	return &provider, nil
}

// ListIdentityProviders returns all identity providers ordered by name.
func (r *IdentityRepository) ListIdentityProviders(ctx context.Context) ([]domain.AdminIdentityProvider, error) {
	rows, err := r.client.Pool.Query(ctx, providerSelectSQL()+` ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list identity providers: %w", err)
	}
	defer rows.Close()

	var providers []domain.AdminIdentityProvider

	for rows.Next() {
		provider, err := scanIdentityProvider(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to scan identity provider: %w", err)
		}

		providers = append(providers, provider)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("error iterating identity providers: %w", err)
	}

	return providers, nil
}

// DisableIdentityProvider disables an identity provider by ID.
func (r *IdentityRepository) DisableIdentityProvider(ctx context.Context, id string) error {
	res, err := r.client.Pool.Exec(ctx, `
		UPDATE admin_identity_providers
		SET is_enabled = false, updated_at = $2
		WHERE id = $1
	`, id, time.Now())
	if err != nil {
		return fmt.Errorf("failed to disable identity provider: %w", err)
	}

	if res.RowsAffected() == 0 {
		return ErrIdentityNotFound
	}

	return nil
}

func (r *IdentityRepository) getUser(ctx context.Context, query string, args ...any) (*domain.AdminUser, error) {
	var user domain.AdminUser

	err := r.client.Execute(func() error {
		var scanErr error

		user, scanErr = scanAdminUser(r.client.Pool.QueryRow(ctx, query, args...).Scan)

		return scanErr
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get admin user: %w", err)
	}

	return &user, nil
}

func (r *IdentityRepository) getRole(ctx context.Context, query string, args ...any) (*domain.AdminRole, error) {
	roles, err := r.listRoles(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	if len(roles) == 0 {
		return nil, nil
	}

	return &roles[0], nil
}

func (r *IdentityRepository) listRoles(ctx context.Context, query string, args ...any) ([]domain.AdminRole, error) {
	rows, err := r.client.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query admin roles: %w", err)
	}
	defer rows.Close()

	var roles []domain.AdminRole

	for rows.Next() {
		role, err := scanAdminRole(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to scan admin role: %w", err)
		}

		roles = append(roles, role)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("error iterating admin roles: %w", err)
	}

	return roles, nil
}

func (r *IdentityRepository) revokeSessions(ctx context.Context, where string, arg string) error {
	res, err := r.client.Pool.Exec(ctx, `
		UPDATE admin_sessions
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE `+where, arg, time.Now())
	if err != nil {
		return fmt.Errorf("failed to revoke admin session: %w", err)
	}

	if res.RowsAffected() == 0 {
		return ErrIdentityNotFound
	}

	return nil
}

func userSelectSQL() string {
	return `
		SELECT id, email, display_name, password_hash, is_active, is_super_admin,
			last_login_at, created_at, updated_at
		FROM admin_users
	`
}

func roleSelectSQL() string {
	return `
		SELECT r.id, r.name, r.description, r.is_builtin, r.created_at, r.updated_at,
			COALESCE(
				array_agg(rp.permission ORDER BY rp.permission)
					FILTER (WHERE rp.permission IS NOT NULL),
				ARRAY[]::TEXT[]
			) AS permissions
		FROM admin_roles r
		LEFT JOIN admin_role_permissions rp ON rp.role_id = r.id
	`
}

func sessionSelectSQL() string {
	return `
		SELECT id, user_id, refresh_token_hash, user_agent, ip, expires_at,
			revoked_at, created_at, last_used_at
		FROM admin_sessions
	`
}

func providerSelectSQL() string {
	return `
		SELECT id, name, type, issuer_url, client_id, client_secret_ref, jwks_url,
			scopes, role_claim, default_role_id, is_enabled, config, created_at, updated_at
		FROM admin_identity_providers
	`
}

func scanAdminUser(scan func(dest ...any) error) (domain.AdminUser, error) {
	var (
		user         domain.AdminUser
		passwordHash sql.NullString
		lastLoginAt  sql.NullTime
	)

	err := scan(
		&user.ID,
		&user.Email,
		&user.DisplayName,
		&passwordHash,
		&user.IsActive,
		&user.IsSuperAdmin,
		&lastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return domain.AdminUser{}, err
	}

	user.PasswordHash = passwordHash.String
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	return user, nil
}

func scanAdminRole(scan func(dest ...any) error) (domain.AdminRole, error) {
	var (
		role        domain.AdminRole
		description sql.NullString
	)

	err := scan(
		&role.ID,
		&role.Name,
		&description,
		&role.IsBuiltin,
		&role.CreatedAt,
		&role.UpdatedAt,
		&role.Permissions,
	)
	if err != nil {
		return domain.AdminRole{}, err
	}

	role.Description = description.String

	return role, nil
}

func scanAdminSession(scan func(dest ...any) error) (domain.AdminSession, error) {
	var (
		session   domain.AdminSession
		userAgent sql.NullString
		ip        sql.NullString
		revokedAt sql.NullTime
	)

	err := scan(
		&session.ID,
		&session.UserID,
		&session.RefreshTokenHash,
		&userAgent,
		&ip,
		&session.ExpiresAt,
		&revokedAt,
		&session.CreatedAt,
		&session.LastUsedAt,
	)
	if err != nil {
		return domain.AdminSession{}, err
	}

	session.UserAgent = userAgent.String

	session.IP = ip.String
	if revokedAt.Valid {
		session.RevokedAt = &revokedAt.Time
	}

	return session, nil
}

func scanIdentityProvider(scan func(dest ...any) error) (domain.AdminIdentityProvider, error) {
	var (
		provider        domain.AdminIdentityProvider
		issuerURL       sql.NullString
		clientID        sql.NullString
		clientSecretRef sql.NullString
		jwksURL         sql.NullString
		roleClaim       sql.NullString
		defaultRoleID   sql.NullString
		config          []byte
	)

	err := scan(
		&provider.ID,
		&provider.Name,
		&provider.Type,
		&issuerURL,
		&clientID,
		&clientSecretRef,
		&jwksURL,
		&provider.Scopes,
		&roleClaim,
		&defaultRoleID,
		&provider.IsEnabled,
		&config,
		&provider.CreatedAt,
		&provider.UpdatedAt,
	)
	if err != nil {
		return domain.AdminIdentityProvider{}, err
	}

	provider.IssuerURL = issuerURL.String
	provider.ClientID = clientID.String
	provider.ClientSecretRef = clientSecretRef.String
	provider.JWKSURL = jwksURL.String
	provider.RoleClaim = roleClaim.String
	provider.DefaultRoleID = defaultRoleID.String

	if len(config) > 0 {
		err := json.Unmarshal(config, &provider.Config)
		if err != nil {
			return domain.AdminIdentityProvider{}, fmt.Errorf("failed to unmarshal identity provider config: %w", err)
		}
	}

	if provider.Config == nil {
		provider.Config = domain.ConfigMap{}
	}

	return provider, nil
}

func replaceRolePermissions(ctx context.Context, tx pgx.Tx, roleID string, permissions []string) error {
	_, err := tx.Exec(ctx, `DELETE FROM admin_role_permissions WHERE role_id = $1`, roleID)
	if err != nil {
		return fmt.Errorf("failed to clear role permissions: %w", err)
	}

	for _, permission := range permissions {
		_, err = tx.Exec(ctx, `
			INSERT INTO admin_role_permissions (role_id, permission)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, roleID, permission)
		if err != nil {
			return fmt.Errorf("failed to save role permission: %w", err)
		}
	}

	return nil
}

func nilString(value string) any {
	if value == "" {
		return nil
	}

	return value
}

func validateProviderConfig(config domain.ConfigMap) error {
	for key, value := range config {
		if strings.Contains(strings.ToLower(key), "secret") {
			return ErrPlaintextProviderSecret
		}

		err := validateValue(value)
		if err != nil {
			return err
		}
	}

	return nil
}

func validateValue(value any) error {
	switch v := value.(type) {
	case map[string]any:
		return validateProviderConfig(domain.ConfigMap(v))
	case domain.ConfigMap:
		return validateProviderConfig(v)
	case []any:
		for _, item := range v {
			if nested, ok := item.(map[string]any); ok {
				err := validateProviderConfig(domain.ConfigMap(nested))
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}
