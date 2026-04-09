package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/your-org/zoekt-mcp-wrapper/internal/app"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := app.Run(ctx, os.Stdin, os.Stdout); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}
