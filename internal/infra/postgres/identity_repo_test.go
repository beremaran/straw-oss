package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdentityRepository(t *testing.T) {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("Skipping integration test: TEST_DB_DSN not set")
	}

	ctx := context.Background()
	require.NoError(t, RunEmbeddedMigrations(ctx, dsn))

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	_, err = pool.Exec(ctx, "TRUNCATE admin_identity_providers, admin_sessions, admin_user_roles, admin_users CASCADE")
	require.NoError(t, err)

	repo := NewIdentityRepository(&Client{Pool: pool})

	ownerRole, err := repo.GetRoleByName(ctx, domain.RoleOwner)
	require.NoError(t, err)
	require.NotNil(t, ownerRole)
	assert.True(t, ownerRole.IsBuiltin)
	assert.ElementsMatch(t, domain.AllPermissions(), ownerRole.Permissions)

	customRole := &domain.AdminRole{
		Name:        "Custom " + uuid.New().String(),
		Description: "Custom role",
		Permissions: []string{
			domain.PermissionReportsRead,
		},
	}
	require.NoError(t, repo.CreateRole(ctx, customRole))
	customRole.Permissions = append(customRole.Permissions, domain.PermissionReportsRun)
	require.NoError(t, repo.UpdateRole(ctx, customRole))
	gotRole, err := repo.GetRoleByID(ctx, customRole.ID)
	require.NoError(t, err)
	require.NotNil(t, gotRole)
	assert.ElementsMatch(t, customRole.Permissions, gotRole.Permissions)

	user := &domain.AdminUser{
		ID:           uuid.New().String(),
		Email:        "owner-" + uuid.New().String() + "@example.test",
		DisplayName:  "Owner User",
		PasswordHash: "bcrypt-hash",
		IsActive:     true,
	}
	require.NoError(t, repo.CreateUser(ctx, user))
	require.NoError(t, repo.SetUserRoles(ctx, user.ID, []string{ownerRole.ID}))

	gotUser, err := repo.GetUserByEmail(ctx, user.Email)
	require.NoError(t, err)
	require.NotNil(t, gotUser)
	assert.Equal(t, user.ID, gotUser.ID)

	perms, err := repo.EffectivePermissions(ctx, user.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, domain.AllPermissions(), perms)

	exists, err := repo.ActiveOwnerExists(ctx)
	require.NoError(t, err)
	assert.True(t, exists)

	user.DisplayName = "Updated Owner"
	require.NoError(t, repo.UpdateUser(ctx, user))
	users, total, err := repo.ListUsers(ctx, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, "Updated Owner", users[0].DisplayName)

	session := &domain.AdminSession{
		ID:               uuid.New().String(),
		UserID:           user.ID,
		RefreshTokenHash: "sha256:old",
		UserAgent:        "test",
		IP:               "127.0.0.1",
		ExpiresAt:        time.Now().Add(time.Hour),
	}
	require.NoError(t, repo.CreateSession(ctx, session))
	gotSession, err := repo.GetSessionByRefreshTokenHash(ctx, "sha256:old")
	require.NoError(t, err)
	require.NotNil(t, gotSession)
	assert.Equal(t, session.ID, gotSession.ID)

	require.NoError(t, repo.UpdateSessionRefreshHash(ctx, session.ID, "sha256:new"))
	oldSession, err := repo.GetSessionByRefreshTokenHash(ctx, "sha256:old")
	require.NoError(t, err)
	assert.Nil(t, oldSession)

	provider := &domain.AdminIdentityProvider{
		ID:              uuid.New().String(),
		Name:            "oidc-" + uuid.New().String(),
		Type:            "oidc",
		IssuerURL:       "https://idp.example.test",
		ClientID:        "client-id",
		ClientSecretRef: "vault://secret/idp",
		IsEnabled:       true,
		Config:          domain.ConfigMap{"prompt": "select_account"},
	}
	require.NoError(t, repo.CreateIdentityProvider(ctx, provider))
	gotProvider, err := repo.GetIdentityProviderByName(ctx, provider.Name)
	require.NoError(t, err)
	require.NotNil(t, gotProvider)
	assert.Equal(t, provider.ClientSecretRef, gotProvider.ClientSecretRef)
	assert.Empty(t, gotProvider.Config["client_secret"])

	err = repo.CreateIdentityProvider(ctx, &domain.AdminIdentityProvider{
		Name:      "bad-" + uuid.New().String(),
		Type:      "oidc",
		IsEnabled: true,
		Config:    domain.ConfigMap{"client_secret": "plaintext"},
	})
	assert.True(t, errors.Is(err, ErrPlaintextProviderSecret))

	require.NoError(t, repo.DeleteRole(ctx, customRole.ID))
	err = repo.DeleteRole(ctx, ownerRole.ID)
	assert.True(t, errors.Is(err, ErrBuiltinRoleProtected))
}
