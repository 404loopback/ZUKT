# Architecture

## Layers

1. `cmd/zukt`
- Application entrypoint.

2. `internal/app`
- Dependency wiring.
- Selects backend implementation based on config.
- Triggers autopilot bootstrap when enabled.

3. `internal/autopilot`
- Ensures Zoekt local readiness.
- Probes local HTTP endpoint.
- Starts `zoekt-web` container when down.
- Indexes configured repositories before serving MCP.

4. `internal/admin`
- MCP administrative use-cases.
- Manages repositories through `.zukt` marker files at repo root.
- Supports workspace discovery and indexing workflows.

5. `internal/mcp`
- MCP JSON-RPC transport.
- Handles `initialize`, `tools/list`, `tools/call`.
- Emits structured logs with request correlation.

6. `internal/search`
- Business service for code search use-cases.
- Input validation and orchestration.
- Applies default directory exclusion filters for noisy paths.

7. `internal/zoekt`
- Backend contract (`Searcher`) and concrete adapters.
- Current: mock adapter.
- HTTP adapter with retry and payload-shape resilience.

## Extension points

- Add repository metadata endpoint normalization for more Zoekt variants.
- Add optional auth/token mode for non-local deployments.
