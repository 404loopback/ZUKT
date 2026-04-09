package search

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/404loopback/zukt/internal/zoekt"
)

type duplicateSearcher struct{}

func (duplicateSearcher) Search(_ context.Context, query, repo string, limit int) ([]zoekt.SearchResult, error) {
	return []zoekt.SearchResult{
		{Repo: "repo", File: "a.go", Line: 12, Snippet: fmt.Sprintf("match %s", query)},
		{Repo: "repo", File: "a.go", Line: 12, Snippet: fmt.Sprintf("match %s", query)},
		{Repo: "repo", File: "b.go", Line: 7, Snippet: "other"},
	}, nil
}

func (duplicateSearcher) ListRepos(_ context.Context) ([]string, error) {
	return []string{"repo"}, nil
}

func TestSearchCodeRequiresQuery(t *testing.T) {
	t.Parallel()

	svc := NewService(zoekt.NewMockSearcher(), nil)
	_, err := svc.SearchCode(context.Background(), "   ", "", 10)
	if err == nil {
		t.Fatalf("expected error when query is empty")
	}
}

func TestSearchCodeRejectsNegativeLimit(t *testing.T) {
	t.Parallel()

	svc := NewService(zoekt.NewMockSearcher(), nil)
	_, err := svc.SearchCode(context.Background(), "needle", "", -1)
	if err == nil {
		t.Fatalf("expected error when limit is negative")
	}
}

func TestSearchCodeExcludesNoiseDirectories(t *testing.T) {
	t.Parallel()

	svc := NewService(zoekt.NewMockSearcher(), []string{"node_modules", ".venv"})
	results, err := svc.SearchCode(context.Background(), "main", "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range results {
		if r.File == "frontend/node_modules/x.js" || r.File == "backend/.venv/lib/y.py" {
			t.Fatalf("unexpected excluded file in results: %s", r.File)
		}
	}
}

func TestSearchCodeDeduplicatesResults(t *testing.T) {
	t.Parallel()

	svc := NewService(duplicateSearcher{}, nil)
	results, err := svc.SearchCode(context.Background(), "main", "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 deduplicated results, got %d (%#v)", len(results), results)
	}

	seen := make(map[string]struct{}, len(results))
	for _, r := range results {
		key := r.Repo + "|" + r.File + "|" + strconv.Itoa(r.Line) + "|" + r.Snippet
		if _, ok := seen[key]; ok {
			t.Fatalf("duplicate result found: %+v", r)
		}
		seen[key] = struct{}{}
	}
}
