// Package main is the entrypoint for the Straw control binary.
package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/server"
)

var Version = "dev"

func main() {
	fs := flag.NewFlagSet("control", flag.ExitOnError)

	cfg, err := config.LoadControlConfigWithFlags(fs)
	if err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}

	err = server.RunWithConfig(cfg, Version)
	if err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}
