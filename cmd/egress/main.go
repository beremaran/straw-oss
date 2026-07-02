package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/natsx"
)

func main() {
	configPath := flag.String("config", "", "path to the egress config file")

	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "missing required -config flag")
		os.Exit(2)
	}

	egressConfig, err := config.LoadEgress(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := natsx.ValidateServers(egressConfig.NATS.Servers); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
