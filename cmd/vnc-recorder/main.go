package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/QuentinBtd/vnc-recorder/internal/recorder"
)

func main() {
	cfg, err := recorder.LoadConfig(os.Args[1:], os.Getenv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	run := recorder.NewRunner(cfg, log.Default())
	if err := run.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "recording error: %v\n", err)
		os.Exit(1)
	}
}
