// Command straw provides the Straw CLI.
package main

import (
	"context"
	"os"

	"github.com/beremaran/straw/v2/internal/cli"
)

func main() {
	os.Exit(cli.Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
