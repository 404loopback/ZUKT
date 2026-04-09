package search

import (
	"context"
	"testing"

	"github.com/your-org/zoekt-mcp-wrapper/internal/zoekt"
)

func TestSearchCodeRequiresQuery(t *testing.T) {
	t.Parallel()

	svc := NewService(zoekt.NewMockSearcher())
	_, err := svc.SearchCode(context.Background(), "   ", "", 10)
	if err == nil {
		t.Fatalf("expected error when query is empty")
	}
}

func TestSearchCodeRejectsNegativeLimit(t *testing.T) {
	t.Parallel()

	svc := NewService(zoekt.NewMockSearcher())
	_, err := svc.SearchCode(context.Background(), "needle", "", -1)
	if err == nil {
		t.Fatalf("expected error when limit is negative")
	}
}
