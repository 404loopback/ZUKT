package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/your-org/zoekt-mcp-wrapper/internal/zoekt"
)

type Service struct {
	searcher zoekt.Searcher
}

func NewService(searcher zoekt.Searcher) *Service {
	return &Service{searcher: searcher}
}

func (s *Service) SearchCode(ctx context.Context, query, repo string, limit int) ([]zoekt.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	if limit < 0 {
		return nil, fmt.Errorf("limit cannot be negative")
	}

	return s.searcher.Search(ctx, query, strings.TrimSpace(repo), limit)
}

func (s *Service) ListRepos(ctx context.Context) ([]string, error) {
	return s.searcher.ListRepos(ctx)
}
