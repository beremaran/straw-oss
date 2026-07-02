// Package main runs the Straw egress worker.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/egress"
	"github.com/beremaran/straw/v2/internal/natsx"
)

const exitUsage = 2

func main() {
	configPath := flag.String("config", "", "path to the egress config file")

	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "missing required -config flag")
		os.Exit(exitUsage)
	}

	egressConfig, err := config.LoadEgress(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	err = natsx.ValidateServers(egressConfig.NATS.Servers)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	_ = egress.NewExecutor(egress.ExecutorOptions{})
}
