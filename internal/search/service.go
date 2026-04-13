package search

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/404loopback/zukt/internal/zoekt"
)

type Service struct {
	searcher    zoekt.Searcher
	allowedDirs map[string]struct{}
	excludeDirs map[string]struct{}
}

func NewService(searcher zoekt.Searcher, allowedDirs []string, excludeDirs []string) *Service {
	allowed := make(map[string]struct{}, len(allowedDirs))
	for _, dir := range allowedDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		allowed[dir] = struct{}{}
	}

	exclude := make(map[string]struct{}, len(excludeDirs))
	for _, name := range excludeDirs {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		exclude[name] = struct{}{}
	}
	return &Service{searcher: searcher, allowedDirs: allowed, excludeDirs: exclude}
}

func (s *Service) SearchCode(ctx context.Context, query, repo string, limit int) ([]zoekt.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	if limit < 0 {
		return nil, fmt.Errorf("limit cannot be negative")
	}

	// Validate repo parameter against allowed directories if configured
	if repo != "" && len(s.allowedDirs) > 0 {
		if !s.isRepoAllowed(repo) {
			return nil, fmt.Errorf("repo %q is not in allowed list", repo)
		}
	}

	results, err := s.searcher.Search(ctx, query, strings.TrimSpace(repo), limit*2)
	if err != nil {
		return nil, err
	}

	filtered := make([]zoekt.SearchResult, 0, len(results))
	seen := make(map[string]struct{}, len(results))
	for _, r := range results {
		// Filter by allowed repository roots if configured
		if !s.isRepoAllowed(r.Repo) {
			continue
		}
		// Filter by excluded directory names
		if s.shouldExcludePath(r.File) {
			continue
		}
		key := r.Repo + "\x00" + r.File + "\x00" + fmt.Sprintf("%d", r.Line) + "\x00" + r.Snippet
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, r)
		if limit > 0 && len(filtered) >= limit {
			break
		}
	}

	return filtered, nil
}

func (s *Service) ListRepos(ctx context.Context) ([]string, error) {
	return s.searcher.ListRepos(ctx)
}

// isRepoAllowed checks if a repository is within the configured allowed directories.
// If no allowed directories are configured, all repos are allowed.
func (s *Service) isRepoAllowed(repo string) bool {
	// If no allowed directories configured, permit all
	if len(s.allowedDirs) == 0 {
		return true
	}

	// Check if repo matches or is within any allowed directory
	repo = strings.TrimSpace(repo)
	for allowed := range s.allowedDirs {
		// Exact match
		if repo == allowed {
			return true
		}
		// Repo is a child of allowed directory (e.g., allowed=/home/user, repo=/home/user/project)
		if strings.HasPrefix(repo, allowed+"/") || strings.HasPrefix(repo, allowed+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func (s *Service) shouldExcludePath(file string) bool {
	if len(s.excludeDirs) == 0 {
		return false
	}
	file = filepath.ToSlash(file)
	for _, segment := range strings.Split(file, "/") {
		if _, ok := s.excludeDirs[segment]; ok {
			return true
		}
	}
	return false
}
