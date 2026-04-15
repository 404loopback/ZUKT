package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

	svc := search.NewService(zoekt.NewMockSearcher(), nil, nil)
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
			names := make(map[string]struct{}, len(tools))
			for _, tool := range tools {
				name := tool.(map[string]any)["name"].(string)
				names[name] = struct{}{}
			}
			for _, required := range []string{"get_status", "get_file", "get_context", "prepare_semantic_index"} {
				if _, ok := names[required]; !ok {
					t.Fatalf("tools/list does not include %s", required)
				}
			}
			toolsListFoundStatus = true
		}
		if id == "4" {
			result := payload["result"].(map[string]any)
			content := result["content"].([]any)
			text := content[0].(map[string]any)["text"].(string)
			if text != "health=up" {
				t.Fatalf("unexpected get_status summary: %q", text)
			}
			status := result["structuredContent"].(map[string]any)
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

func TestServerGetFileAndGetContext(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(filePath, []byte("a\nb\nc\nd\n"), 0o644); err != nil {
		t.Fatalf("write sample file: %v", err)
	}

	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_file","arguments":{"path":%q,"start_line":2,"end_line":3}}}`, filePath),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_context","arguments":{"path":%q,"line":3,"before":1,"after":1}}}`, filePath),
	}, "\n"))
	var out strings.Builder

	svc := search.NewService(zoekt.NewMockSearcher(), nil, nil)
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
	if len(lines) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(lines))
	}

	for _, line := range lines {
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("invalid response json: %v", err)
		}

		id := fmt.Sprintf("%v", payload["id"])
		if id != "2" && id != "3" {
			continue
		}
		result := payload["result"].(map[string]any)
		content := result["content"].([]any)
		text := content[0].(map[string]any)["text"].(string)
		structured := result["structuredContent"].(map[string]any)

		if id == "2" {
			if text != "b\nc" {
				t.Fatalf("unexpected get_file text content: %#v", text)
			}
			if _, ok := structured["content"]; ok {
				t.Fatalf("get_file structured content should not duplicate text payload")
			}
		}
		if id == "3" {
			if text != "b\nc\nd" {
				t.Fatalf("unexpected get_context text content: %#v", text)
			}
			if _, ok := structured["content"]; ok {
				t.Fatalf("get_context structured content should not duplicate text payload")
			}
		}
	}
}
