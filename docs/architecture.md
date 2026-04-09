# Architecture

## Layers

1. `cmd/zoekt-mcp`
- Application entrypoint.

2. `internal/app`
- Dependency wiring.
- Selects backend implementation based on config.

3. `internal/mcp`
- MCP JSON-RPC transport.
- Handles `initialize`, `tools/list`, `tools/call`.

4. `internal/search`
- Business service for code search use-cases.
- Input validation and orchestration.

5. `internal/zoekt`
- Backend contract (`Searcher`) and concrete adapters.
- Current: mock adapter.
- Planned: HTTP adapter to real Zoekt service.

## Extension points

- Add `internal/zoekt/http.go` with real API calls.
- Add request-scoped timeouts and structured logging.
- Add auth middleware for multi-tenant scenarios.
