package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"wx_channel/internal/poc"
)

const (
	exitSucceeded         = 0
	exitPartial           = 2
	exitNeedsVerification = 3
	exitFailed            = 4
	exitRequestInvalid    = 64
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout))
}

type capabilityResponse struct {
	SchemaVersion    int      `json:"schema_version"`
	RuntimeProtocols []string `json:"runtime_protocols"`
	RouterKinds      []string `json:"router_kinds"`
}

func run(ctx context.Context, args []string, output io.Writer) int {
	if len(args) == 0 {
		return exitRequestInvalid
	}
	switch args[0] {
	case "capabilities":
		if len(args) != 1 || writeCapabilities(output) != nil {
			return exitFailed
		}
		return exitSucceeded
	case "collect":
		return runCollect(ctx, args[1:])
	case "finalize":
		return runFinalize(args[1:])
	default:
		return exitRequestInvalid
	}
}

func writeCapabilities(output io.Writer) error {
	return json.NewEncoder(output).Encode(capabilityResponse{
		SchemaVersion:    1,
		RuntimeProtocols: []string{"wechat-channels-local-runtime-v2"},
		RouterKinds:      []string{"mihomo"},
	})
}

func runCollect(ctx context.Context, args []string) int {
	flags := flag.NewFlagSet("collect", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	requestPath := flags.String("request", "", "")
	runRoot := flags.String("run-root", "", "")
	apiBase := flags.String("api-base", "", "")
	if flags.Parse(args) != nil || flags.NArg() != 0 || *requestPath == "" || *runRoot == "" || *apiBase == "" {
		return exitRequestInvalid
	}
	request, err := poc.LoadBatchRequest(*requestPath, *runRoot)
	if err != nil {
		return exitRequestInvalid
	}
	client, err := poc.NewLtaooClient(*apiBase, 30*time.Second)
	if err != nil {
		return exitRequestInvalid
	}
	result, err := poc.RunLtaooBatch(ctx, request, client, *runRoot)
	if err != nil {
		return exitFailed
	}
	return exitCodeForStatus(result.Status)
}

func runFinalize(args []string) int {
	flags := flag.NewFlagSet("finalize", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	requestPath := flags.String("request", "", "")
	runRoot := flags.String("run-root", "", "")
	cleanupReceipt := flags.String("cleanup-receipt", "", "")
	if flags.Parse(args) != nil || flags.NArg() != 0 || *requestPath == "" || *runRoot == "" || *cleanupReceipt == "" {
		return exitRequestInvalid
	}
	request, err := poc.LoadBatchRequest(*requestPath, *runRoot)
	if err != nil {
		return exitRequestInvalid
	}
	manifest, err := poc.FinalizeLtaooBatch(request, *runRoot, *cleanupReceipt)
	if err != nil {
		return exitFailed
	}
	return exitCodeForStatus(manifest.Status)
}

func exitCodeForStatus(status poc.BatchStatus) int {
	switch status {
	case poc.BatchSucceeded:
		return exitSucceeded
	case poc.BatchPartial:
		return exitPartial
	case poc.BatchNeedsVerification:
		return exitNeedsVerification
	case poc.BatchFailed:
		return exitFailed
	default:
		return exitFailed
	}
}
