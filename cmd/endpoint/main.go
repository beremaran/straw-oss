// Package main is the entrypoint for the Straw endpoint binary.
package main

import (
	"log/slog"
	"os"

	"github.com/beremaran/straw/internal/endpoint"
)

var Version = "dev"

func main() {
	endpoint.Version = Version

	err := endpoint.Run()
	if err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}
