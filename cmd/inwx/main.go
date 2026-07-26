package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/k2b-dev/inwx-cli/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	os.Exit(cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, cli.Options{
		Version: version,
		Commit:  commit,
		Date:    date,
	}))
}
