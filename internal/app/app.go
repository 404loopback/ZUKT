package app

import (
	"context"
	"fmt"
	"io"

	"github.com/your-org/zoekt-mcp-wrapper/internal/config"
	"github.com/your-org/zoekt-mcp-wrapper/internal/mcp"
	"github.com/your-org/zoekt-mcp-wrapper/internal/search"
	"github.com/your-org/zoekt-mcp-wrapper/internal/zoekt"
)

func Run(ctx context.Context, in io.Reader, out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	var backend zoekt.Searcher
	switch cfg.ZoektBackend {
	case "http":
		httpSearcher, err := zoekt.NewHTTPSearcher(cfg.ZoektHTTPURL, cfg.ZoektTimeout)
		if err != nil {
			return fmt.Errorf("build zoekt http backend: %w", err)
		}
		backend = httpSearcher
	case "mock":
		backend = zoekt.NewMockSearcher()
	default:
		return fmt.Errorf("unsupported backend: %s", cfg.ZoektBackend)
	}

	svc := search.NewService(backend)
	server := mcp.NewServer(cfg.ServerName, cfg.ServerVersion, svc)

	return server.Serve(ctx, in, out)
}
