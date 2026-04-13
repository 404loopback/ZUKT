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
					"name":        "get_status",
					"description": "Return MCP backend status (url, timeout, health)",
					"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
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
		status := map[string]any{
			"backend_url": s.statusConf.BackendURL,
			"timeout":     s.statusConf.Timeout.String(),
			"health":      "unknown",
		}
		if s.statusConf.HealthCheck != nil {
			healthCtx, cancel := context.WithTimeout(ctx, s.statusConf.Timeout)
			defer cancel()
			if err := s.statusConf.HealthCheck(healthCtx); err != nil {
				status["health"] = "down"
				status["error"] = err.Error()
			} else {
				status["health"] = "up"
			}
		}
		return response{JSONRPC: "2.0", ID: req.ID, Result: toolResult(status)}
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
