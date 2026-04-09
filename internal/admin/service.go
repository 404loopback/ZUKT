package admin

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/404loopback/zukt/internal/autopilot"
	"github.com/404loopback/zukt/internal/config"
	"github.com/404loopback/zukt/internal/repos"
)

type Service struct {
	cfg          config.Config
	logger       *slog.Logger
	repoManager  *repos.Manager
	orchestrator *autopilot.Orchestrator
}

func NewService(cfg config.Config, logger *slog.Logger, orchestrator *autopilot.Orchestrator) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	mgr := repos.NewManager(func(repo string) error {
		return config.ValidateRepoPath(repo, cfg.ZoektAllowedDirs)
	})
	return &Service{
		cfg:          cfg,
		logger:       logger,
		repoManager:  mgr,
		orchestrator: orchestrator,
	}
}

func (s *Service) ListRepos() ([]string, error) {
	return s.repoManager.ListManagedInRoots(s.cfg.ZoektAllowedDirs)
}

func (s *Service) AddRepo(path string) ([]string, error) {
	if err := s.repoManager.Add(path); err != nil {
		return nil, err
	}
	s.logger.Info("repository added", "repo", path)
	return s.repoManager.ListManagedInRoots(s.cfg.ZoektAllowedDirs)
}

func (s *Service) RemoveRepo(path string) ([]string, error) {
	if err := s.repoManager.Remove(path); err != nil {
		return nil, err
	}
	s.logger.Info("repository removed", "repo", path)
	return s.repoManager.ListManagedInRoots(s.cfg.ZoektAllowedDirs)
}

func (s *Service) IndexManagedRepos(ctx context.Context, force bool) (map[string]any, error) {
	reposList, err := s.repoManager.ListManagedInRoots(s.cfg.ZoektAllowedDirs)
	if err != nil {
		return nil, err
	}
	if len(reposList) == 0 {
		return nil, fmt.Errorf("no managed repositories configured")
	}
	if err := s.orchestrator.IndexRepos(ctx, reposList, force); err != nil {
		return nil, err
	}
	return map[string]any{
		"indexed": reposList,
		"count":   len(reposList),
		"force":   force,
	}, nil
}

func (s *Service) IndexWorkspace(ctx context.Context, workspace string, force bool) (map[string]any, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = s.cfg.ProjectRoot
	}
	workspace = filepath.Clean(workspace)
	if err := config.ValidateRepoPath(workspace, s.cfg.ZoektAllowedDirs); err != nil {
		return nil, fmt.Errorf("workspace is not allowed: %w", err)
	}

	found, err := discoverGitRepos(workspace)
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return map[string]any{"indexed": []string{}, "count": 0, "workspace": workspace, "force": force}, nil
	}

	for _, repoPath := range found {
		if err := s.repoManager.Add(repoPath); err != nil {
			return nil, fmt.Errorf("add discovered repo %s: %w", repoPath, err)
		}
	}

	if err := s.orchestrator.IndexRepos(ctx, found, force); err != nil {
		return nil, err
	}
	return map[string]any{
		"indexed":   found,
		"count":     len(found),
		"workspace": workspace,
		"force":     force,
	}, nil
}

func MergeRepoSources(envRepos, managed []string) []string {
	seen := make(map[string]struct{}, len(envRepos)+len(managed))
	out := make([]string, 0, len(envRepos)+len(managed))
	for _, r := range append(envRepos, managed...) {
		r = filepath.Clean(strings.TrimSpace(r))
		if r == "" {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

func discoverGitRepos(workspace string) ([]string, error) {
	repos := make([]string, 0, 16)
	err := filepath.WalkDir(workspace, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == "node_modules" || name == ".venv" || name == ".git" {
			// If we reached ".git", record its parent repo then skip deeper walk.
			if name == ".git" {
				repos = append(repos, filepath.Dir(path))
			}
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover repos in %s: %w", workspace, err)
	}
	repos = MergeRepoSources(repos, nil)
	return repos, nil
}
