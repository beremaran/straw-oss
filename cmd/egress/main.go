package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/beremaran/straw/v2/internal/config"
)

func main() {
	configPath := flag.String("config", "", "path to the egress config file")
	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "missing required -config flag")
		os.Exit(2)
	}

	if _, err := config.LoadEgress(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
