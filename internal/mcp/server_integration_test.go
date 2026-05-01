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
			searchCodeFoundVerbosity := false
			listReposFoundVerbosity := false
			getStatusFoundVerbosity := false
			for _, tool := range tools {
				toolMap := tool.(map[string]any)
				name := toolMap["name"].(string)
				names[name] = struct{}{}
				if name == "search_code" {
					props := toolMap["inputSchema"].(map[string]any)["properties"].(map[string]any)
					mode, ok := props["mode"].(map[string]any)
					if !ok {
						t.Fatalf("search_code input schema missing mode property")
					}
					modeEnum, ok := mode["enum"].([]any)
					if !ok || len(modeEnum) != 3 {
						t.Fatalf("search_code mode enum is invalid: %#v", mode["enum"])
					}
					verbosity, ok := props["verbosity"].(map[string]any)
					if !ok {
						t.Fatalf("search_code input schema missing verbosity property")
					}
					enumValues, ok := verbosity["enum"].([]any)
					if !ok || len(enumValues) != 3 {
						t.Fatalf("search_code verbosity enum is invalid: %#v", verbosity["enum"])
					}
					searchCodeFoundVerbosity = true
				}
				if name == "list_repos" || name == "get_status" {
					props := toolMap["inputSchema"].(map[string]any)["properties"].(map[string]any)
					verbosity, ok := props["verbosity"].(map[string]any)
					if !ok {
						t.Fatalf("%s input schema missing verbosity property", name)
					}
					enumValues, ok := verbosity["enum"].([]any)
					if !ok || len(enumValues) != 2 {
						t.Fatalf("%s verbosity enum is invalid: %#v", name, verbosity["enum"])
					}
					if name == "list_repos" {
						listReposFoundVerbosity = true
					}
					if name == "get_status" {
						getStatusFoundVerbosity = true
					}
				}
			}
			for _, required := range []string{"get_status", "get_file", "get_context", "prepare_semantic_index"} {
				if _, ok := names[required]; !ok {
					t.Fatalf("tools/list does not include %s", required)
				}
			}
			if !searchCodeFoundVerbosity {
				t.Fatalf("tools/list search_code schema missing verbosity enum")
			}
			if !listReposFoundVerbosity {
				t.Fatalf("tools/list list_repos schema missing verbosity enum")
			}
			if !getStatusFoundVerbosity {
				t.Fatalf("tools/list get_status schema missing verbosity enum")
			}
			toolsListFoundStatus = true
		}
		if id == "4" {
			result := payload["result"].(map[string]any)
			content := result["content"].([]any)
			text := content[0].(map[string]any)["text"].(string)
			if text != "health=up verbosity=compact" {
				t.Fatalf("unexpected get_status summary: %q", text)
			}
			status := result["structuredContent"].(map[string]any)
			statusCallHealth, _ = status["health"].(string)
			if _, ok := status["backend_url"]; ok {
				t.Fatalf("compact get_status should not include backend_url")
			}
		}
	}
	if !toolsListFoundStatus {
		t.Fatalf("tools/list does not include get_status")
	}
	if statusCallHealth != "up" {
		t.Fatalf("expected get_status health=up, got %q", statusCallHealth)
	}
}

func TestGetStatusVerbosityFullIncludesBackendDetails(t *testing.T) {
	t.Parallel()

	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_status","arguments":{"verbosity":"full"}}}`,
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
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(lines))
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &payload); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	result := payload["result"].(map[string]any)
	content := result["content"].([]any)
	summary := content[0].(map[string]any)["text"].(string)
	if summary != "health=up verbosity=full" {
		t.Fatalf("unexpected get_status full summary: %q", summary)
	}
	status := result["structuredContent"].(map[string]any)
	if _, ok := status["backend_url"]; !ok {
		t.Fatalf("full get_status should include backend_url")
	}
	if _, ok := status["timeout"]; !ok {
		t.Fatalf("full get_status should include timeout")
	}
}

func TestListReposVerbosityModes(t *testing.T) {
	t.Parallel()

	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_repos","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_repos","arguments":{"verbosity":"full"}}}`,
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

	var compactPayload map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &compactPayload); err != nil {
		t.Fatalf("invalid compact response json: %v", err)
	}
	compactResult := compactPayload["result"].(map[string]any)
	compactSummary := compactResult["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(compactSummary, "verbosity=compact") {
		t.Fatalf("unexpected compact list_repos summary: %q", compactSummary)
	}
	if _, ok := compactResult["structuredContent"].([]any); !ok {
		t.Fatalf("compact list_repos should return array payload")
	}

	var fullPayload map[string]any
	if err := json.Unmarshal([]byte(lines[2]), &fullPayload); err != nil {
		t.Fatalf("invalid full response json: %v", err)
	}
	fullResult := fullPayload["result"].(map[string]any)
	fullSummary := fullResult["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(fullSummary, "verbosity=full") {
		t.Fatalf("unexpected full list_repos summary: %q", fullSummary)
	}
	fullStructured := fullResult["structuredContent"].(map[string]any)
	if _, ok := fullStructured["count"]; !ok {
		t.Fatalf("full list_repos should include count")
	}
	if _, ok := fullStructured["repos"]; !ok {
		t.Fatalf("full list_repos should include repos")
	}
}

func TestListReposRejectsStandardVerbosity(t *testing.T) {
	t.Parallel()

	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_repos","arguments":{"verbosity":"standard"}}}`,
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
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(lines))
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &payload); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	errPayload, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error payload for invalid list_repos verbosity, got: %#v", payload)
	}
	code, _ := errPayload["code"].(float64)
	if int(code) != -32602 {
		t.Fatalf("expected error code -32602, got %v", errPayload["code"])
	}
}

func TestGetStatusRejectsStandardVerbosity(t *testing.T) {
	t.Parallel()

	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_status","arguments":{"verbosity":"standard"}}}`,
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
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(lines))
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &payload); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	errPayload, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error payload for invalid get_status verbosity, got: %#v", payload)
	}
	code, _ := errPayload["code"].(float64)
	if int(code) != -32602 {
		t.Fatalf("expected error code -32602, got %v", errPayload["code"])
	}
}

func TestSearchCodeVerbosityDefaultsToCompact(t *testing.T) {
	t.Parallel()

	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_code","arguments":{"query":"main","limit":1}}}`,
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
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(lines))
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &payload); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	result := payload["result"].(map[string]any)
	content := result["content"].([]any)
	summary := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(summary, "verbosity=compact") {
		t.Fatalf("expected compact summary, got %q", summary)
	}

	matches := result["structuredContent"].([]any)
	if len(matches) == 0 {
		t.Fatalf("expected at least one compact match")
	}
	first := matches[0].(map[string]any)
	if _, ok := first["file"]; !ok {
		t.Fatalf("compact result missing file: %#v", first)
	}
	if _, ok := first["line"]; !ok {
		t.Fatalf("compact result missing line: %#v", first)
	}
	if _, ok := first["snippet"]; ok {
		t.Fatalf("compact result should not include snippet: %#v", first)
	}
	if _, ok := first["repo"]; ok {
		t.Fatalf("compact result should not include repo: %#v", first)
	}
}

func TestSearchCodeVerbosityFullIncludesDetailedFields(t *testing.T) {
	t.Parallel()

	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_code","arguments":{"query":"main","limit":1,"verbosity":"full"}}}`,
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
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(lines))
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &payload); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	result := payload["result"].(map[string]any)
	matches := result["structuredContent"].([]any)
	if len(matches) == 0 {
		t.Fatalf("expected at least one full match")
	}
	first := matches[0].(map[string]any)
	for _, field := range []string{"repo", "file", "line", "snippet"} {
		if _, ok := first[field]; !ok {
			t.Fatalf("full result missing %s: %#v", field, first)
		}
	}
}

func TestSearchCodeVerbosityRejectsInvalidValue(t *testing.T) {
	t.Parallel()

	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_code","arguments":{"query":"main","verbosity":"verbose"}}}`,
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
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(lines))
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &payload); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	errPayload, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error payload for invalid verbosity, got: %#v", payload)
	}
	code, _ := errPayload["code"].(float64)
	if int(code) != -32602 {
		t.Fatalf("expected error code -32602, got %v", errPayload["code"])
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
