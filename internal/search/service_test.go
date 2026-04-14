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

	svc := NewService(zoekt.NewMockSearcher(), nil, nil)
	_, err := svc.SearchCode(context.Background(), "   ", "", 10)
	if err == nil {
		t.Fatalf("expected error when query is empty")
	}
}

func TestSearchCodeRejectsNegativeLimit(t *testing.T) {
	t.Parallel()

	svc := NewService(zoekt.NewMockSearcher(), nil, nil)
	_, err := svc.SearchCode(context.Background(), "needle", "", -1)
	if err == nil {
		t.Fatalf("expected error when limit is negative")
	}
}

func TestSearchCodeExcludesNoiseDirectories(t *testing.T) {
	t.Parallel()

	svc := NewService(zoekt.NewMockSearcher(), nil, []string{"node_modules", ".venv"})
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

	svc := NewService(duplicateSearcher{}, nil, nil)
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

func TestSearchCodeAcceptsLogicalRepoNameWithAllowedRoots(t *testing.T) {
	t.Parallel()

	svc := NewService(zoekt.NewMockSearcher(), []string{"/home/alice"}, nil)
	_, err := svc.SearchCode(context.Background(), "needle", "ZUKT", 10)
	if err != nil {
		t.Fatalf("logical repo should not be blocked by allowed roots: %v", err)
	}
}

func TestSearchCodeRejectsAbsoluteRepoOutsideAllowedRoots(t *testing.T) {
	t.Parallel()

	svc := NewService(zoekt.NewMockSearcher(), []string{"/home/alice"}, nil)
	_, err := svc.SearchCode(context.Background(), "needle", "/home/bob/repo", 10)
	if err == nil {
		t.Fatalf("expected error for repo outside allowed roots")
	}
	if err.Error() != `repo "/home/bob/repo" is outside allowed roots` {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchCodeAllowsAbsoluteRepoInsideAllowedRoots(t *testing.T) {
	t.Parallel()

	svc := NewService(zoekt.NewMockSearcher(), []string{"/home/alice"}, nil)
	_, err := svc.SearchCode(context.Background(), "main", "/home/alice/myproject", 10)
	if err != nil {
		t.Fatalf("unexpected error for repo inside allowed roots: %v", err)
	}
}

func TestSearchCodeFiltersAbsoluteRepoResultsByAllowedRoots(t *testing.T) {
	t.Parallel()

	searcher := &mixedReposSearcher{}
	svc := NewService(searcher, []string{"/home/alice"}, nil)
	results, err := svc.SearchCode(context.Background(), "test", "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Repo == "/home/bob/project2" {
			t.Fatalf("unexpected repo in filtered results: %q", r.Repo)
		}
	}
}

// mixedReposSearcher returns results from multiple different repos
type mixedReposSearcher struct{}

func (m *mixedReposSearcher) Search(_ context.Context, query, repo string, limit int) ([]zoekt.SearchResult, error) {
	return []zoekt.SearchResult{
		{Repo: "/home/alice/project1", File: "main.go", Line: 1, Snippet: query},
		{Repo: "/home/bob/project2", File: "test.go", Line: 2, Snippet: query},
		{Repo: "/home/alice/project3", File: "app.go", Line: 3, Snippet: query},
		{Repo: "ZUKT", File: "cmd/zukt/main.go", Line: 9, Snippet: query},
	}, nil
}

func (m *mixedReposSearcher) ListRepos(_ context.Context) ([]string, error) {
	return []string{"/home/alice/project1", "/home/alice/project3", "/home/bob/project2", "ZUKT"}, nil
}
