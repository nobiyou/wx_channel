package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"wx_channel/internal/poc"
)

func main() {
	if len(os.Args) < 2 {
		fatal("usage: wx_channel_poc <preflight|cert-smoke|run|cleanup>")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	options := poc.DefaultOptions()
	switch os.Args[1] {
	case "preflight":
		os.Exit(poc.RunPreflightCLI(ctx, os.Stdout, options))
	case "run":
		os.Exit(poc.RunCLI(ctx, os.Stdin, os.Stdout, os.Args[2:], options))
	case "cert-smoke":
		os.Exit(poc.RunCertificateSmokeCLI(ctx, os.Stdin, os.Stdout, os.Args[2:], options))
	case "cleanup":
		os.Exit(poc.RunCleanupCLI(ctx, os.Stdout, os.Args[2:], options))
	default:
		fatal("unknown command")
	}
}

func fatal(message string) {
	_, _ = fmt.Fprintln(os.Stderr, message)
	os.Exit(2)
}
