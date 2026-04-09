package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/404loopback/zukt/internal/search"
	"github.com/404loopback/zukt/internal/zoekt"
)

func TestServerSmokeInitializeListAndSearch(t *testing.T) {
	t.Parallel()

	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_repos","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"search_code","arguments":{"query":"main","limit":2}}}`,
	}, "\n"))
	var out strings.Builder

	svc := search.NewService(zoekt.NewMockSearcher(), nil)
	srv := NewServer("zoekt-mcp-wrapper", "0.1.0", svc, nil, nil)

	if err := srv.Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 responses, got %d", len(lines))
	}

	for i, line := range lines {
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("response %d is not valid JSON: %v", i+1, err)
		}
		if payload["jsonrpc"] != "2.0" {
			t.Fatalf("response %d missing jsonrpc=2.0: %s", i+1, line)
		}
	}
}
