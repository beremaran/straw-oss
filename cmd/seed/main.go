package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/straw_proxy?sslmode=disable"
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	// Try multiple locations for seed.sql
	locations := []string{
		"scripts/seed.sql",
		"../scripts/seed.sql",
		"../../scripts/seed.sql",
		"seed.sql",
	}

	var content []byte
	var foundPath string
	for _, loc := range locations {
		data, err := os.ReadFile(loc)
		if err == nil {
			content = data
			foundPath = loc
			break
		}
	}

	if content == nil {
		// Try to find it relative to current working directory
		cwd, _ := os.Getwd()
		log.Fatalf("Could not find seed.sql in any of %v (current dir: %s)", locations, cwd)
	}

	fmt.Printf("Reading seed data from %s...\n", foundPath)

	_, err = db.Exec(string(content))
	if err != nil {
		log.Fatalf("Failed to execute seed SQL: %v", err)
	}

	fmt.Println("✅ Data seeded successfully!")
}
