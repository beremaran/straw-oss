package postgres

import (
	"context"
	"database/sql"
	"fmt"

	// Required by goose to register the pgx driver with database/sql.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/beremaran/straw/internal/infra/postgres/migrations"
)

// RunEmbeddedMigrations runs all embedded SQL migrations against the database at the given DSN.
func RunEmbeddedMigrations(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open db for migrations: %w", err)
	}
	defer func() { _ = db.Close() }()

	err = db.PingContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to db for migrations: %w", err)
	}

	err = goose.SetDialect("postgres")
	if err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	goose.SetBaseFS(migrations.FS)

	err = goose.Up(db, ".")
	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}
