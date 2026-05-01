package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/404loopback/zukt/internal/search"
	"github.com/404loopback/zukt/internal/zoekt"
)

type Server struct {
	name       string
	version    string
	search     *search.Service
	logger     *slog.Logger
	statusConf StatusConfig
}

type StatusConfig struct {
	BackendURL  string
	Timeout     time.Duration
	HealthCheck func(ctx context.Context) error
}

func NewServer(name, version string, searchSvc *search.Service, logger *slog.Logger, statusConf StatusConfig) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if statusConf.Timeout <= 0 {
		statusConf.Timeout = 5 * time.Second
	}
	return &Server{name: name, version: version, search: searchSvc, logger: logger, statusConf: statusConf}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const protocolVersion = "2024-11-05"

const (
	searchVerbosityCompact  = "compact"
	searchVerbosityStandard = "standard"
	searchVerbosityFull     = "full"
)

var compactFullVerbosityValues = []string{searchVerbosityCompact, searchVerbosityFull}

type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
}

func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	writer := bufio.NewWriter(out)
	defer writer.Flush()

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			s.logger.Error("invalid json-rpc payload", "error", err.Error())
			if err := writeJSON(writer, response{JSONRPC: "2.0", Error: &responseError{Code: -32700, Message: "parse error"}}); err != nil {
				return err
			}
			continue
		}

		reqID := normalizeReqID(req.ID)
		log := s.logger.With("request_id", reqID, "method", req.Method)
		log.Info("mcp request received")

		resp := s.handle(ctx, req)
		// JSON-RPC notifications do not expect a response body.
		if hasRequestID(req.ID) {
			if err := writeJSON(writer, resp); err != nil {
				return err
			}
		}
		if resp.Error != nil {
			log.Error("mcp request failed", "code", resp.Error.Code, "error", resp.Error.Message)
			continue
		}
		log.Info("mcp request completed")
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}
	return nil
}

func normalizeReqID(raw json.RawMessage) string {
	if !hasRequestID(raw) {
		return "notification"
	}
	trimmed := strings.TrimSpace(string(raw))
	return trimmed
}

func hasRequestID(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != ""
}

func (s *Server) handle(ctx context.Context, req request) response {
	if req.JSONRPC != "2.0" {
		return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32600, Message: "invalid request"}}
	}

	switch req.Method {
	case "initialize":
		selectedProtocol := protocolVersion
		if len(req.Params) > 0 {
			var params initializeParams
			if err := json.Unmarshal(req.Params, &params); err == nil && strings.TrimSpace(params.ProtocolVersion) != "" {
				// For now we negotiate by reflecting the client's proposal.
				selectedProtocol = strings.TrimSpace(params.ProtocolVersion)
			}
		}
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"protocolVersion": selectedProtocol,
			"serverInfo":      map[string]any{"name": s.name, "version": s.version},
			"capabilities":    map[string]any{"tools": map[string]any{}},
		}}
	case "initialized", "notifications/initialized":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"tools": []map[string]any{
				{
					"name":        "search_code",
					"description": "Search source code via lexical or semantic ranking (semantic mode uses vector index, hybrid combines lexical+semantic; supports Zoekt query syntax: r:, file:, sym:, lang:, case:)",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{"type": "string"},
							"repo": map[string]any{
								"type":        "string",
								"description": "Optional repo filter. Logical repo name (e.g. ZUKT) or absolute repo path.",
							},
							"limit": map[string]any{"type": "integer", "minimum": 1},
							"mode": map[string]any{
								"type":        "string",
								"description": "Search mode: lexical, semantic, or hybrid (default: hybrid).",
								"enum":        []string{"lexical", "semantic", "hybrid"},
							},
							"verbosity": map[string]any{
								"type":        "string",
								"description": "Payload verbosity: compact (default, token-lean), standard, or full.",
								"enum":        []string{searchVerbosityCompact, searchVerbosityStandard, searchVerbosityFull},
							},
						},
						"required": []string{"query"},
					},
				},
				{
					"name":        "prepare_semantic_index",
					"description": "Prepare or refresh semantic chunk index for one local repository",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"repo": map[string]any{
								"type":        "string",
								"description": "Repository logical name (e.g. ZUKT) or absolute path.",
							},
						},
						"required": []string{"repo"},
					},
				},
				{
					"name":        "list_repos",
					"description": "List indexed repositories",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"verbosity": map[string]any{
								"type":        "string",
								"description": "Payload verbosity: compact (default) or full.",
								"enum":        compactFullVerbosityValues,
							},
						},
					},
				},
				{
					"name":        "get_status",
					"description": "Return MCP backend status (url, timeout, health)",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"verbosity": map[string]any{
								"type":        "string",
								"description": "Payload verbosity: compact (default) or full.",
								"enum":        compactFullVerbosityValues,
							},
						},
					},
				},
				{
					"name":        "get_file",
					"description": "Read a file from an allowed local repository path",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"repo":       map[string]any{"type": "string", "description": "Logical repo name or absolute repo path. Required when path is relative."},
							"path":       map[string]any{"type": "string", "description": "Relative path inside repo, or absolute file path."},
							"start_line": map[string]any{"type": "integer", "minimum": 1},
							"end_line":   map[string]any{"type": "integer", "minimum": 1},
						},
						"required": []string{"path"},
					},
				},
				{
					"name":        "get_context",
					"description": "Read contextual lines around a file line from an allowed local repository path",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"repo":   map[string]any{"type": "string", "description": "Logical repo name or absolute repo path. Required when path is relative."},
							"path":   map[string]any{"type": "string", "description": "Relative path inside repo, or absolute file path."},
							"line":   map[string]any{"type": "integer", "minimum": 1},
							"before": map[string]any{"type": "integer", "minimum": 0},
							"after":  map[string]any{"type": "integer", "minimum": 0},
						},
						"required": []string{"path", "line"},
					},
				},
			},
		}}
	case "tools/call":
		return s.handleToolCall(ctx, req)
	default:
		return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32601, Message: "method not found"}}
	}
}

func (s *Server) handleToolCall(ctx context.Context, req request) response {
	var payload struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &payload); err != nil {
		return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32602, Message: "invalid params"}}
	}

	switch payload.Name {
	case "get_status":
		verbosity, err := parseCompactOrFullVerbosity(payload.Arguments)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32602, Message: err.Error()}}
		}
		health := "unknown"
		healthErr := ""
		if s.statusConf.HealthCheck != nil {
			healthCtx, cancel := context.WithTimeout(ctx, s.statusConf.Timeout)
			defer cancel()
			if err := s.statusConf.HealthCheck(healthCtx); err != nil {
				health = "down"
				healthErr = err.Error()
			} else {
				health = "up"
			}
		}
		status := formatStatusPayload(verbosity, s.statusConf.BackendURL, s.statusConf.Timeout, health, healthErr)
		return response{JSONRPC: "2.0", ID: req.ID, Result: toolResultStructured(fmt.Sprintf("health=%s verbosity=%s", health, verbosity), status)}
	case "list_repos":
		verbosity, err := parseCompactOrFullVerbosity(payload.Arguments)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32602, Message: err.Error()}}
		}
		repos, err := s.search.ListRepos(ctx)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32000, Message: err.Error()}}
		}
		return response{JSONRPC: "2.0", ID: req.ID, Result: toolResultStructured(fmt.Sprintf("repositories=%d verbosity=%s", len(repos), verbosity), formatReposPayload(repos, verbosity))}
	case "search_code":
		query := stringArg(payload.Arguments, "query")
		repo := stringArg(payload.Arguments, "repo")
		mode := stringArg(payload.Arguments, "mode")
		if mode == "" {
			mode = "hybrid"
		}
		verbosity, err := parseSearchVerbosity(payload.Arguments)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32602, Message: err.Error()}}
		}
		limit, err := intArg(payload.Arguments, "limit", 10)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32602, Message: err.Error()}}
		}

		results, err := s.search.SearchCodeWithMode(ctx, query, repo, limit, mode)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32000, Message: err.Error()}}
		}

		payload := formatSearchResults(results, verbosity, repo)
		return response{JSONRPC: "2.0", ID: req.ID, Result: toolResultStructured(fmt.Sprintf("mode=%s verbosity=%s matches=%d", mode, verbosity, len(results)), payload)}
	case "prepare_semantic_index":
		repo := stringArg(payload.Arguments, "repo")
		stats, err := s.search.PrepareSemanticIndex(ctx, repo)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32000, Message: err.Error()}}
		}
		return response{JSONRPC: "2.0", ID: req.ID, Result: toolResultStructured(fmt.Sprintf("repo=%s files=%d chunks=%d", stats.Repo, stats.FilesIndexed, stats.ChunksIndexed), stats)}
	case "get_file":
		repo := stringArg(payload.Arguments, "repo")
		filePath := stringArg(payload.Arguments, "path")
		startLine, err := intArg(payload.Arguments, "start_line", 1)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32602, Message: err.Error()}}
		}
		endLine, err := intArg(payload.Arguments, "end_line", 0)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32602, Message: err.Error()}}
		}

		result, err := s.search.GetFile(ctx, repo, filePath, startLine, endLine)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32000, Message: err.Error()}}
		}
		return response{JSONRPC: "2.0", ID: req.ID, Result: toolResultTextWithStructured(result.Content, fileSliceMetadata(result))}
	case "get_context":
		repo := stringArg(payload.Arguments, "repo")
		filePath := stringArg(payload.Arguments, "path")
		line, err := intArg(payload.Arguments, "line", 0)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32602, Message: err.Error()}}
		}
		before, err := intArg(payload.Arguments, "before", 20)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32602, Message: err.Error()}}
		}
		after, err := intArg(payload.Arguments, "after", 20)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32602, Message: err.Error()}}
		}

		result, err := s.search.GetContext(ctx, repo, filePath, line, before, after)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32000, Message: err.Error()}}
		}
		return response{JSONRPC: "2.0", ID: req.ID, Result: toolResultTextWithStructured(result.Content, fileContextMetadata(result))}
	default:
		return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32602, Message: "unknown tool"}}
	}
}

func stringArg(args map[string]interface{}, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func intArg(args map[string]interface{}, key string, fallback int) (int, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return fallback, nil
	}

	switch value := raw.(type) {
	case float64:
		if value != float64(int(value)) {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
		return int(value), nil
	case int:
		return value, nil
	default:
		return 0, fmt.Errorf("%s must be an integer", key)
	}
}

func toolResultStructured(summary string, payload any) map[string]any {
	if strings.TrimSpace(summary) == "" {
		summary = "ok"
	}
	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": summary,
			},
		},
		"structuredContent": payload,
	}
}

func toolResultTextWithStructured(text string, payload any) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": text,
			},
		},
		"structuredContent": payload,
	}
}

type compactSearchResult struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

type standardSearchResult struct {
	Repo    string  `json:"repo,omitempty"`
	File    string  `json:"file"`
	Line    int     `json:"line"`
	EndLine int     `json:"end_line,omitempty"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score,omitempty"`
	Source  string  `json:"source,omitempty"`
}

type fullReposPayload struct {
	Count int      `json:"count"`
	Repos []string `json:"repos"`
}

func parseSearchVerbosity(args map[string]interface{}) (string, error) {
	raw := strings.ToLower(stringArg(args, "verbosity"))
	if raw == "" {
		return searchVerbosityCompact, nil
	}
	switch raw {
	case searchVerbosityCompact, searchVerbosityStandard, searchVerbosityFull:
		return raw, nil
	default:
		return "", fmt.Errorf("verbosity must be one of: compact, standard, full")
	}
}

func parseCompactOrFullVerbosity(args map[string]interface{}) (string, error) {
	raw := strings.ToLower(stringArg(args, "verbosity"))
	if raw == "" {
		return searchVerbosityCompact, nil
	}
	switch raw {
	case searchVerbosityCompact, searchVerbosityFull:
		return raw, nil
	default:
		return "", fmt.Errorf("verbosity must be one of: compact, full")
	}
}

func formatSearchResults(results []zoekt.SearchResult, verbosity string, requestedRepo string) any {
	switch verbosity {
	case searchVerbosityFull:
		return results
	case searchVerbosityStandard:
		out := make([]standardSearchResult, 0, len(results))
		for _, r := range results {
			item := standardSearchResult{
				File:    r.File,
				Line:    r.Line,
				EndLine: r.EndLine,
				Snippet: r.Snippet,
				Score:   r.Score,
				Source:  r.Source,
			}
			if strings.TrimSpace(requestedRepo) == "" {
				item.Repo = r.Repo
			}
			out = append(out, item)
		}
		return out
	default:
		out := make([]compactSearchResult, 0, len(results))
		for _, r := range results {
			out = append(out, compactSearchResult{
				File: r.File,
				Line: r.Line,
			})
		}
		return out
	}
}

func formatStatusPayload(verbosity, backendURL string, timeout time.Duration, health, healthErr string) map[string]any {
	payload := map[string]any{
		"health": health,
	}
	if strings.TrimSpace(healthErr) != "" {
		payload["error"] = healthErr
	}

	if verbosity == searchVerbosityFull {
		payload["backend_url"] = backendURL
		payload["timeout"] = timeout.String()
	}
	return payload
}

func formatReposPayload(repos []string, verbosity string) any {
	switch verbosity {
	case searchVerbosityFull:
		return fullReposPayload{
			Count: len(repos),
			Repos: repos,
		}
	default:
		return repos
	}
}

type fileSliceMeta struct {
	Repo       string `json:"repo,omitempty"`
	Path       string `json:"path"`
	AbsPath    string `json:"abs_path"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	TotalLines int    `json:"total_lines"`
	Truncated  bool   `json:"truncated"`
}

type fileContextMeta struct {
	Repo       string `json:"repo,omitempty"`
	Path       string `json:"path"`
	AbsPath    string `json:"abs_path"`
	Line       int    `json:"line"`
	Before     int    `json:"before"`
	After      int    `json:"after"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	TotalLines int    `json:"total_lines"`
	Truncated  bool   `json:"truncated"`
}

func fileSliceMetadata(result search.FileSlice) fileSliceMeta {
	return fileSliceMeta{
		Repo:       result.Repo,
		Path:       result.Path,
		AbsPath:    result.AbsPath,
		StartLine:  result.StartLine,
		EndLine:    result.EndLine,
		TotalLines: result.TotalLines,
		Truncated:  result.Truncated,
	}
}

func fileContextMetadata(result search.FileContext) fileContextMeta {
	return fileContextMeta{
		Repo:       result.Repo,
		Path:       result.Path,
		AbsPath:    result.AbsPath,
		Line:       result.Line,
		Before:     result.Before,
		After:      result.After,
		StartLine:  result.StartLine,
		EndLine:    result.EndLine,
		TotalLines: result.TotalLines,
		Truncated:  result.Truncated,
	}
}

func writeJSON(w *bufio.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	if err := w.WriteByte('\n'); err != nil {
		return err
	}
	return w.Flush()
}
