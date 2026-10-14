package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/MMH-4ALL/StackDoctor/internal/cli"
)

var version = "dev"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, version)
	cancel()
	os.Exit(code)
}
