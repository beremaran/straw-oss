// Package main is the entrypoint for the Straw egress binary.
package main

import (
	"log/slog"
	"os"

	"github.com/beremaran/straw/internal/egress"
)

var Version = "dev"

func main() {
	egress.Version = Version

	err := egress.Run()
	if err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}
