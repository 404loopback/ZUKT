package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/404loopback/zukt/internal/admin"
	"github.com/404loopback/zukt/internal/autopilot"
	"github.com/404loopback/zukt/internal/config"
	"github.com/404loopback/zukt/internal/mcp"
	"github.com/404loopback/zukt/internal/repos"
	"github.com/404loopback/zukt/internal/search"
	"github.com/404loopback/zukt/internal/zoekt"
)

func Run(ctx context.Context, in io.Reader, out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	repoManager := repos.NewManager(func(repo string) error {
		return config.ValidateRepoPath(repo, cfg.ZoektAllowedDirs)
	})
	managedRepos, err := repoManager.ListManagedInRoots(cfg.ZoektAllowedDirs)
	if err != nil {
		return fmt.Errorf("load managed repos from .zukt markers: %w", err)
	}
	cfg.ZoektRepos = admin.MergeRepoSources(cfg.ZoektRepos, managedRepos)

	orchestrator := autopilot.New(cfg, logger.With("component", "autopilot"))

	if cfg.ZoektAutopilot {
		if len(cfg.ZoektRepos) == 0 {
			logger.Warn("autopilot enabled but no repositories configured yet; use repos_add or index_workspace MCP tools")
		}
		logger.Info("autopilot enabled", "repo_count", len(cfg.ZoektRepos), "index_dir", cfg.ZoektIndexDir)
		if err := orchestrator.EnsureReady(ctx); err != nil {
			// Keep MCP server available for admin tools even when backend bootstrap fails.
			// Search calls can still return explicit backend errors at call time.
			logger.Error("autopilot bootstrap failed; MCP will start in degraded mode", "error", err)
		}
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

	svc := search.NewService(backend, cfg.ZoektExcludeDirs)
	adminSvc := admin.NewService(cfg, logger.With("component", "admin"), orchestrator)
	server := mcp.NewServer(cfg.ServerName, cfg.ServerVersion, svc, adminSvc, logger.With("component", "mcp"))

	return server.Serve(ctx, in, out)
}
