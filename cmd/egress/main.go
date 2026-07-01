// Package main is the entrypoint for the Straw egress binary.
package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/egress"
)

var Version = "dev"

func main() {
	fs := flag.NewFlagSet("egress", flag.ExitOnError)

	cfg, err := config.LoadEgressConfigWithFlags(fs)
	if err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}

	egress.Version = Version

	err = egress.RunWithConfig(cfg)
	if err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}
