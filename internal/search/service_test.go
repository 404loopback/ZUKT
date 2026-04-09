package search

import (
	"context"
	"testing"

	"github.com/404loopback/zukt/internal/zoekt"
)

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
