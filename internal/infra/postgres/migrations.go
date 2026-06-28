package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/beremaran/straw/internal/infra/postgres/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

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
