package main

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	// Generate ID and Secret
	id := uuid.New().String()
	secret := "load-test-secret"

	// Hash the secret
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}

	// Print the raw key (ID:Secret format)
	fmt.Printf("API Key ID: %s\n", id)
	fmt.Printf("API Key Secret: %s\n", secret)
	fmt.Printf("API Key (ID:Secret): %s:%s\n", id, secret)
	fmt.Printf("Key Hash: %s\n", string(hash))
	_ = domain.NewApiKey(id, string(hash), "Load Test Key", []string{})
}
