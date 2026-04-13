package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/404loopback/zukt/internal/search"
	"github.com/404loopback/zukt/internal/zoekt"
)

func TestServerSmokeInitializeListAndSearch(t *testing.T) {
	t.Parallel()

	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_repos","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_status","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"search_code","arguments":{"query":"main","limit":2}}}`,
	}, "\n"))
	var out strings.Builder

	svc := search.NewService(zoekt.NewMockSearcher(), nil)
	srv := NewServer("zukt", "0.1.0", svc, nil, StatusConfig{
		BackendURL: "http://127.0.0.1:6070",
		Timeout:    200 * time.Millisecond,
		HealthCheck: func(context.Context) error {
			return nil
		},
	})

	if err := srv.Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 responses, got %d", len(lines))
	}

	toolsListFoundStatus := false
	statusCallHealth := ""
	for i, line := range lines {
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("response %d is not valid JSON: %v", i+1, err)
		}
		if payload["jsonrpc"] != "2.0" {
			t.Fatalf("response %d missing jsonrpc=2.0: %s", i+1, line)
		}
		id := fmt.Sprintf("%v", payload["id"])
		if id == "2" {
			tools := payload["result"].(map[string]any)["tools"].([]any)
			for _, tool := range tools {
				name := tool.(map[string]any)["name"].(string)
				if name == "get_status" {
					toolsListFoundStatus = true
				}
			}
		}
		if id == "4" {
			content := payload["result"].(map[string]any)["content"].([]any)
			text := content[0].(map[string]any)["text"].(string)
			var status map[string]any
			if err := json.Unmarshal([]byte(text), &status); err != nil {
				t.Fatalf("failed to decode get_status payload: %v", err)
			}
			statusCallHealth, _ = status["health"].(string)
		}
	}
	if !toolsListFoundStatus {
		t.Fatalf("tools/list does not include get_status")
	}
	if statusCallHealth != "up" {
		t.Fatalf("expected get_status health=up, got %q", statusCallHealth)
	}
}
