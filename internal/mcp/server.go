package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/404loopback/zukt/internal/admin"
	"github.com/404loopback/zukt/internal/search"
)

type Server struct {
	name    string
	version string
	search  *search.Service
	admin   *admin.Service
	logger  *slog.Logger
}

func NewServer(name, version string, searchSvc *search.Service, adminSvc *admin.Service, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{name: name, version: version, search: searchSvc, admin: adminSvc, logger: logger}
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
			"serverInfo":   map[string]any{"name": s.name, "version": s.version},
			"capabilities": map[string]any{"tools": map[string]any{}},
		}}
	case "initialized", "notifications/initialized":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"tools": []map[string]any{
				{
					"name":        "search_code",
					"description": "Search source code via Zoekt backend",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{"type": "string"},
							"repo":  map[string]any{"type": "string"},
							"limit": map[string]any{"type": "integer", "minimum": 1},
						},
						"required": []string{"query"},
					},
				},
				{
					"name":        "list_repos",
					"description": "List indexed repositories",
					"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
				},
				{
					"name":        "repos_list",
					"description": "List repositories managed by .zukt marker files in workspace",
					"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
				},
				{
					"name":        "repos_add",
					"description": "Mark a repository as managed by creating a .zukt file at repository root",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{"type": "string"},
						},
						"required": []string{"path"},
					},
				},
				{
					"name":        "repos_remove",
					"description": "Unmark a managed repository by removing its .zukt marker",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{"type": "string"},
						},
						"required": []string{"path"},
					},
				},
				{
					"name":        "repos_index",
					"description": "Index all repositories currently managed by .zukt markers",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"force": map[string]any{"type": "boolean"},
						},
					},
				},
				{
					"name":        "index_workspace",
					"description": "Discover git repositories in a workspace, mark them with .zukt, and index them",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"workspace": map[string]any{"type": "string"},
							"force":     map[string]any{"type": "boolean"},
						},
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
	case "list_repos":
		repos, err := s.search.ListRepos(ctx)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32000, Message: err.Error()}}
		}
		return response{JSONRPC: "2.0", ID: req.ID, Result: toolResult(repos)}
	case "search_code":
		query, _ := payload.Arguments["query"].(string)
		repo, _ := payload.Arguments["repo"].(string)

		limit := 10
		if raw, ok := payload.Arguments["limit"].(float64); ok {
			limit = int(raw)
		}

		results, err := s.search.SearchCode(ctx, query, repo, limit)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32000, Message: err.Error()}}
		}

		return response{JSONRPC: "2.0", ID: req.ID, Result: toolResult(results)}
	case "repos_list":
		if s.admin == nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32603, Message: "admin service unavailable"}}
		}
		repos, err := s.admin.ListRepos()
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32000, Message: err.Error()}}
		}
		return response{JSONRPC: "2.0", ID: req.ID, Result: toolResult(repos)}
	case "repos_add":
		if s.admin == nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32603, Message: "admin service unavailable"}}
		}
		path, _ := payload.Arguments["path"].(string)
		repos, err := s.admin.AddRepo(path)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32000, Message: err.Error()}}
		}
		return response{JSONRPC: "2.0", ID: req.ID, Result: toolResult(repos)}
	case "repos_remove":
		if s.admin == nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32603, Message: "admin service unavailable"}}
		}
		path, _ := payload.Arguments["path"].(string)
		repos, err := s.admin.RemoveRepo(path)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32000, Message: err.Error()}}
		}
		return response{JSONRPC: "2.0", ID: req.ID, Result: toolResult(repos)}
	case "repos_index":
		if s.admin == nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32603, Message: "admin service unavailable"}}
		}
		force := false
		if raw, ok := payload.Arguments["force"].(bool); ok {
			force = raw
		}
		result, err := s.admin.IndexManagedRepos(ctx, force)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32000, Message: err.Error()}}
		}
		return response{JSONRPC: "2.0", ID: req.ID, Result: toolResult(result)}
	case "index_workspace":
		if s.admin == nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32603, Message: "admin service unavailable"}}
		}
		workspace, _ := payload.Arguments["workspace"].(string)
		force := false
		if raw, ok := payload.Arguments["force"].(bool); ok {
			force = raw
		}
		result, err := s.admin.IndexWorkspace(ctx, workspace, force)
		if err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32000, Message: err.Error()}}
		}
		return response{JSONRPC: "2.0", ID: req.ID, Result: toolResult(result)}
	default:
		return response{JSONRPC: "2.0", ID: req.ID, Error: &responseError{Code: -32602, Message: "unknown tool"}}
	}
}

func toolResult(payload any) map[string]any {
	body, err := json.Marshal(payload)
	if err != nil {
		body = []byte(`{"error":"failed to encode tool result"}`)
	}

	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": string(body),
			},
		},
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
