package postgres

import (
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // Register pgx driver for database/sql
	"github.com/kwilabs/straw-proxy-server/internal/infra/postgres/migrations"
	"github.com/pressly/goose/v3"
)

// RunMigrations runs database migrations using goose.
// It uses the standard library sql.DB interface as required by goose.
func RunMigrations(dsn string, migrationsDir string) error {
	// goose requires database/sql connection
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open db for migrations: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to connect to db for migrations: %w", err)
	}

	// Set dialect
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	// Run migrations
	if err := goose.Up(db, migrationsDir); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// RunEmbeddedMigrations runs database migrations using the embedded filesystem.
func RunEmbeddedMigrations(dsn string) error {
	return RunMigrationsWithFS(dsn, migrations.FS, ".")
}

// RunMigrationsWithFS runs database migrations from an embed.FS.
func RunMigrationsWithFS(dsn string, fs embed.FS, dir string) error {
	goose.SetBaseFS(fs)
	return RunMigrations(dsn, dir)
}
