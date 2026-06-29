package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrIdentityNotFound        = errors.New("identity row not found")
	ErrBuiltinRoleProtected    = errors.New("built-in role cannot be deleted")
	ErrPlaintextProviderSecret = errors.New("identity provider config must not contain secrets")
)

type IdentityRepository struct {
	client *Client
}

func NewIdentityRepository(client *Client) *IdentityRepository {
	return &IdentityRepository{client: client}
}

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

		return err
	})
	if err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}

	return nil
}

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
		rows = res.RowsAffected()

		return err
	})
	if err != nil {
		return fmt.Errorf("failed to update admin user: %w", err)
	}
	if rows == 0 {
		return ErrIdentityNotFound
	}

	return nil
}

func (r *IdentityRepository) GetUserByID(ctx context.Context, id string) (*domain.AdminUser, error) {
	query := userSelectSQL() + ` WHERE id = $1`

	return r.getUser(ctx, query, id)
}

func (r *IdentityRepository) GetUserByEmail(ctx context.Context, email string) (*domain.AdminUser, error) {
	query := userSelectSQL() + ` WHERE lower(email) = lower($1)`

	return r.getUser(ctx, query, email)
}

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
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating admin users: %w", err)
	}

	return users, total, nil
}

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

func (r *IdentityRepository) ListUserRoles(ctx context.Context, userID string) ([]domain.AdminRole, error) {
	query := roleSelectSQL() + `
		JOIN admin_user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1
		GROUP BY r.id
		ORDER BY r.name ASC
	`

	return r.listRoles(ctx, query, userID)
}

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

		if err := replaceRolePermissions(ctx, tx, role.ID, role.Permissions); err != nil {
			return err
		}

		return tx.Commit(ctx)
	})
}

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

		if err := replaceRolePermissions(ctx, tx, role.ID, role.Permissions); err != nil {
			return err
		}

		return tx.Commit(ctx)
	})
}

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

func (r *IdentityRepository) GetRoleByID(ctx context.Context, id string) (*domain.AdminRole, error) {
	query := roleSelectSQL() + `
		WHERE r.id = $1
		GROUP BY r.id
	`

	return r.getRole(ctx, query, id)
}

func (r *IdentityRepository) GetRoleByName(ctx context.Context, name string) (*domain.AdminRole, error) {
	query := roleSelectSQL() + `
		WHERE r.name = $1
		GROUP BY r.id
	`

	return r.getRole(ctx, query, name)
}

func (r *IdentityRepository) ListRoles(ctx context.Context) ([]domain.AdminRole, error) {
	query := roleSelectSQL() + `
		GROUP BY r.id
		ORDER BY r.is_builtin DESC, r.name ASC
	`

	return r.listRoles(ctx, query)
}

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

		return err
	})
	if err != nil {
		return fmt.Errorf("failed to create admin session: %w", err)
	}

	return nil
}

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

func (r *IdentityRepository) RevokeSession(ctx context.Context, id string) error {
	return r.revokeSessions(ctx, `id = $1`, id)
}

func (r *IdentityRepository) RevokeUserSessions(ctx context.Context, userID string) error {
	return r.revokeSessions(ctx, `user_id = $1`, userID)
}

func (r *IdentityRepository) CreateIdentityProvider(ctx context.Context, provider *domain.AdminIdentityProvider) error {
	if err := validateProviderConfig(provider.Config); err != nil {
		return err
	}
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

		return err
	})
	if err != nil {
		return fmt.Errorf("failed to create identity provider: %w", err)
	}

	return nil
}

func (r *IdentityRepository) UpdateIdentityProvider(ctx context.Context, provider *domain.AdminIdentityProvider) error {
	if err := validateProviderConfig(provider.Config); err != nil {
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating identity providers: %w", err)
	}

	return providers, nil
}

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

func (r *IdentityRepository) getUser(ctx context.Context, query string, args ...interface{}) (*domain.AdminUser, error) {
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

func (r *IdentityRepository) getRole(ctx context.Context, query string, args ...interface{}) (*domain.AdminRole, error) {
	roles, err := r.listRoles(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return nil, nil
	}

	return &roles[0], nil
}

func (r *IdentityRepository) listRoles(ctx context.Context, query string, args ...interface{}) ([]domain.AdminRole, error) {
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
	if err := rows.Err(); err != nil {
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

func scanAdminUser(scan func(dest ...interface{}) error) (domain.AdminUser, error) {
	var user domain.AdminUser
	var passwordHash sql.NullString
	var lastLoginAt sql.NullTime

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

func scanAdminRole(scan func(dest ...interface{}) error) (domain.AdminRole, error) {
	var role domain.AdminRole
	var description sql.NullString

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

func scanAdminSession(scan func(dest ...interface{}) error) (domain.AdminSession, error) {
	var session domain.AdminSession
	var userAgent sql.NullString
	var ip sql.NullString
	var revokedAt sql.NullTime

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

func scanIdentityProvider(scan func(dest ...interface{}) error) (domain.AdminIdentityProvider, error) {
	var provider domain.AdminIdentityProvider
	var issuerURL sql.NullString
	var clientID sql.NullString
	var clientSecretRef sql.NullString
	var jwksURL sql.NullString
	var roleClaim sql.NullString
	var defaultRoleID sql.NullString
	var config []byte

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
		if err := json.Unmarshal(config, &provider.Config); err != nil {
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

func nilString(value string) interface{} {
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

		switch v := value.(type) {
		case map[string]interface{}:
			if err := validateProviderConfig(domain.ConfigMap(v)); err != nil {
				return err
			}
		case domain.ConfigMap:
			if err := validateProviderConfig(v); err != nil {
				return err
			}
		case []interface{}:
			for _, item := range v {
				nested, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				if err := validateProviderConfig(domain.ConfigMap(nested)); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
