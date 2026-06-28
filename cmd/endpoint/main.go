package main

import (
	"log/slog"
	"os"

	"github.com/beremaran/straw/pkg/endpoint"
)

var (
	Version = "dev"
)

func main() {
	endpoint.Version = Version
	if err := endpoint.Run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}
