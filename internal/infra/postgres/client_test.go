package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/infra/postgres"
)

func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://straw:straw@localhost:5432/straw?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := postgres.NewClient(ctx, dsn, nil)
	if err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}
	defer client.Close()

	if err := client.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	var result int
	err = client.Pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if result != 1 {
		t.Errorf("Expected 1, got %d", result)
	}
}
