package postgresx

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var errDSNEnvEmpty = errors.New("postgres: DSN environment variable is empty")

// Config holds the Postgres connection parameters derived from
// control.database.postgres.* in the config file.
type Config struct {
	DSNEnv            string
	MaxOpenConns      int
	MaxIdleConns      int
	ConnMaxLifetimeMS int
}

// ApplyDefaults fills in zero-valued fields with production-safe defaults.
func (c *Config) ApplyDefaults() {
	if c.DSNEnv == "" {
		c.DSNEnv = "STRAW_POSTGRES_DSN"
	}

	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = 20
	}

	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = 5
	}

	if c.ConnMaxLifetimeMS == 0 {
		c.ConnMaxLifetimeMS = 1_800_000 // 30 minutes
	}
}

// ResolveDSN reads the DSN from the environment variable named in cfg.DSNEnv,
// defaulting to STRAW_POSTGRES_DSN when unset. Returns an error if the variable
// is unset or empty.
func ResolveDSN(cfg Config) (string, error) {
	cfg.ApplyDefaults()

	dsn := os.Getenv(cfg.DSNEnv)

	if dsn == "" {
		return "", fmt.Errorf("%w: %s", errDSNEnvEmpty, cfg.DSNEnv)
	}

	return dsn, nil
}

// Connect builds a pgxpool.Pool from the given config and DSN.
// It verifies the connection is live before returning.
func Connect(ctx context.Context, cfg Config, dsn string) (*pgxpool.Pool, error) {
	cfg.ApplyDefaults()

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse connection config: %w", err)
	}

	poolConfig.MaxConns = safeInt32(cfg.MaxOpenConns)
	poolConfig.MinConns = safeInt32(cfg.MaxIdleConns)
	poolConfig.MaxConnLifetime = time.Duration(cfg.ConnMaxLifetimeMS) * time.Millisecond

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("postgres: create connection pool: %w", err)
	}

	err = pool.Ping(ctx)
	if err != nil {
		pool.Close()

		return nil, fmt.Errorf("postgres: ping connection: %w", err)
	}

	return pool, nil
}

// safeInt32 clamps a value to [0, maxInt32] to prevent pgxpool overflow.
func safeInt32(v int) int32 {
	const maxVal = 1_000_000

	if v < 0 {
		return 0
	}

	if v > maxVal {
		return maxVal
	}

	return int32(v)
}

// ApplyMigrations executes every .sql file under postgres/ in fsys, in the
// lexicographic order fs.ReadDir returns. Migrations use CREATE ... IF NOT
// EXISTS, so applying them on every startup is idempotent. Each file is run as
// one multi-statement batch: pgx uses the simple query protocol for parameterless
// Exec, which permits several statements per call.
func ApplyMigrations(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	entries, err := fs.ReadDir(fsys, "postgres")
	if err != nil {
		return fmt.Errorf("postgres: read migrations: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()

		if entry.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}

		migrationSQL, readErr := fs.ReadFile(fsys, "postgres/"+name)
		if readErr != nil {
			return fmt.Errorf("postgres: read migration %s: %w", name, readErr)
		}

		_, execErr := pool.Exec(ctx, string(migrationSQL))
		if execErr != nil {
			return fmt.Errorf("postgres: apply migration %s: %w", name, execErr)
		}
	}

	return nil
}
