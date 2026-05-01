package search

import (
	"context"
	"strings"
	"testing"

	"github.com/404loopback/zukt/internal/zoekt"
)

type staticSearcher struct {
	results []zoekt.SearchResult
}

func (s staticSearcher) Search(_ context.Context, query, repo string, limit int) ([]zoekt.SearchResult, error) {
	out := make([]zoekt.SearchResult, 0, len(s.results))
	for _, result := range s.results {
		if repo != "" && result.Repo != repo {
			continue
		}
		out = append(out, result)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s staticSearcher) ListRepos(_ context.Context) ([]string, error) {
	return []string{"PITANCE"}, nil
}

func TestSearchCodeWithModeSemanticRequiresRepo(t *testing.T) {
	t.Parallel()

	svc := NewService(staticSearcher{}, nil, nil)
	_, err := svc.SearchCodeWithMode(context.Background(), "create_app", "", 10, "semantic")
	if err == nil {
		t.Fatalf("expected semantic mode without repo to fail")
	}
	if !strings.Contains(err.Error(), "repo is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchCodeWithModeSupportsHybrid(t *testing.T) {
	t.Parallel()

	svc := NewService(zoekt.NewMockSearcher(), nil, nil)
	_, err := svc.SearchCodeWithMode(context.Background(), "create_app", "", 10, "hybrid")
	if err != nil {
		t.Fatalf("expected hybrid mode to be accepted, got: %v", err)
	}
}

func TestPrepareSemanticIndexAndSearchSemantic(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repoRoot := createRepoFixture(t, root, "PITANCE")
	writeFileFixture(t, repoRoot+"/backend/app/application.py", "from fastapi import FastAPI\n\ndef create_app() -> FastAPI:\n    return FastAPI()\n")
	writeFileFixture(t, repoRoot+"/frontend/src/schedule.ts", "export function useScheduleTimeline() {\n  return []\n}\n")

	svc := NewService(zoekt.NewMockSearcher(), []string{root}, []string{".git", "node_modules", "dist", "build"})
	stats, err := svc.PrepareSemanticIndex(context.Background(), "PITANCE")
	if err != nil {
		t.Fatalf("PrepareSemanticIndex failed: %v", err)
	}
	if stats.FilesIndexed == 0 || stats.ChunksIndexed == 0 {
		t.Fatalf("expected non-empty semantic index stats, got %#v", stats)
	}

	results, err := svc.SearchCodeWithMode(context.Background(), "fastapi app factory", "PITANCE", 5, "semantic")
	if err != nil {
		t.Fatalf("semantic search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected semantic results, got empty")
	}
	found := false
	for _, result := range results {
		if result.File == "backend/app/application.py" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected semantic hit for backend/app/application.py, got %#v", results)
	}
}

func TestNormalizeSemanticQuery(t *testing.T) {
	t.Parallel()

	got := normalizeSemanticQuery(`sym:create_app file:application.py r:PITANCE lang:python case:yes`)
	want := "create_app application.py"
	if got != want {
		t.Fatalf("normalizeSemanticQuery mismatch: got %q want %q", got, want)
	}
}
