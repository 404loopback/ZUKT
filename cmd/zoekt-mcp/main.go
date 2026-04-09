package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/404loopback/zukt/internal/app"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := app.Run(ctx, os.Stdin, os.Stdout); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}
