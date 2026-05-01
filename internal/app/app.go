package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/404loopback/zukt/internal/backend"
	"github.com/404loopback/zukt/internal/config"
	"github.com/404loopback/zukt/internal/mcp"
	"github.com/404loopback/zukt/internal/search"
)

func Run(ctx context.Context, in io.Reader, out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Info("starting zukt runtime", "backend_url", cfg.ZoektHTTPURL, "timeout", cfg.ZoektTimeout.String())
	for _, warning := range cfg.Warnings {
		logger.Warn("deprecated configuration", "warning", warning)
	}

	searchBackend, err := backend.NewSearcher(cfg)
	if err != nil {
		return fmt.Errorf("build zoekt backend: %w", err)
	}

	if err := ensureSearchBackendHealthy(ctx, cfg, searchBackend, logger.With("component", "startup_health")); err != nil {
		return err
	}

	svc := search.NewService(searchBackend, cfg.ZoektAllowedDirs, cfg.ZoektExcludeDirs, search.WithSemanticConfig(search.SemanticRuntimeConfig{
		Backend:           cfg.Semantic.Backend,
		QdrantURL:         cfg.Semantic.QdrantURL,
		CollectionPrefix:  cfg.Semantic.CollectionPrefix,
		EmbeddingProvider: cfg.Semantic.EmbeddingProvider,
		OllamaURL:         cfg.Semantic.OllamaURL,
		EmbeddingModel:    cfg.Semantic.EmbeddingModel,
		EmbeddingDim:      cfg.Semantic.EmbeddingDim,
		ChunkLines:        cfg.Semantic.ChunkLines,
		ChunkOverlap:      cfg.Semantic.ChunkOverlap,
		MaxFileBytes:      cfg.Semantic.MaxFileBytes,
	}))
	server := mcp.NewServer(cfg.ServerName, cfg.ServerVersion, svc, logger.With("component", "mcp"), mcp.StatusConfig{
		BackendURL: cfg.ZoektHTTPURL,
		Timeout:    cfg.ZoektTimeout,
		HealthCheck: func(checkCtx context.Context) error {
			_, err := searchBackend.ListRepos(checkCtx)
			return err
		},
	})

	return server.Serve(ctx, in, out)
}

func ensureSearchBackendHealthy(ctx context.Context, cfg config.Config, searchBackend searchBackend, logger *slog.Logger) error {
	timeout := cfg.ZoektTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	logger.Info("performing zoekt startup health check", "backend_url", cfg.ZoektHTTPURL, "timeout", timeout.String())
	healthCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if _, err := searchBackend.ListRepos(healthCtx); err != nil {
		logger.Error("zoekt startup health check failed", "backend_url", cfg.ZoektHTTPURL, "timeout", timeout.String(), "error", err.Error())
		return fmt.Errorf("startup aborted: zoekt backend unreachable at %s within %s: %w (check service status and try: curl -fsS %s/api/list)", cfg.ZoektHTTPURL, timeout.String(), err, cfg.ZoektHTTPURL)
	}
	logger.Info("zoekt startup health check succeeded", "backend_url", cfg.ZoektHTTPURL)
	return nil
}

type searchBackend interface {
	ListRepos(ctx context.Context) ([]string, error)
}
