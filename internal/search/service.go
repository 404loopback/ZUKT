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
	excludeDirs map[string]struct{}
}

func NewService(searcher zoekt.Searcher, excludeDirs []string) *Service {
	exclude := make(map[string]struct{}, len(excludeDirs))
	for _, name := range excludeDirs {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		exclude[name] = struct{}{}
	}
	return &Service{searcher: searcher, excludeDirs: exclude}
}

func (s *Service) SearchCode(ctx context.Context, query, repo string, limit int) ([]zoekt.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	if limit < 0 {
		return nil, fmt.Errorf("limit cannot be negative")
	}

	results, err := s.searcher.Search(ctx, query, strings.TrimSpace(repo), limit*2)
	if err != nil {
		return nil, err
	}

	filtered := make([]zoekt.SearchResult, 0, len(results))
	for _, r := range results {
		if s.shouldExcludePath(r.File) {
			continue
		}
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
