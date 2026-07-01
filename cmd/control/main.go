// Package main is the entrypoint for the Straw control binary.
package main

import (
	"log/slog"
	"os"

	"github.com/beremaran/straw/internal/server"
)

var Version = "dev"

func main() {
	err := server.Run(Version)
	if err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}
