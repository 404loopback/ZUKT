package search

import (
	"context"
	"reflect"
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

func TestSearchCodeWithModeSemanticNoRepoFallsBackToLexical(t *testing.T) {
	t.Parallel()

	searcher := staticSearcher{
		results: []zoekt.SearchResult{
			{Repo: "PITANCE", File: "backend/app/application.py", Line: 89, Snippet: "def create_app() -> FastAPI:"},
			{Repo: "PITANCE", File: "backend/app/config.py", Line: 87, Snippet: "def build_public_artist_url(public_slug: str) -> str:"},
		},
	}
	svc := NewService(searcher, nil, nil)

	lexical, err := svc.SearchCode(context.Background(), "create_app", "", 10)
	if err != nil {
		t.Fatalf("SearchCode lexical failed: %v", err)
	}

	semantic, err := svc.SearchCodeWithMode(context.Background(), "create_app", "", 10, "semantic")
	if err != nil {
		t.Fatalf("SearchCodeWithMode semantic failed: %v", err)
	}

	if !reflect.DeepEqual(lexical, semantic) {
		t.Fatalf("semantic mode without repo should match lexical mode\nlexical=%#v\nsemantic=%#v", lexical, semantic)
	}
}

func TestSearchCodeWithModeRejectsHybridAlias(t *testing.T) {
	t.Parallel()

	svc := NewService(zoekt.NewMockSearcher(), nil, nil)
	_, err := svc.SearchCodeWithMode(context.Background(), "create_app", "PITANCE", 10, "hybrid")
	if err == nil {
		t.Fatalf("expected hybrid alias to be rejected")
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
	if stats.FilesIndexed == 0 || stats.Chunks == 0 {
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
