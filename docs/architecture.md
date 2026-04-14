# Architecture

## Layers

1. `cmd/zukt`
- Application entrypoint.

2. `internal/app`
- Dependency wiring.
- Enforce runtime contract (`zukt` = MCP/search only).
- Performs startup health check against Zoekt HTTP backend with fail-fast.

3. `internal/mcp`
- MCP JSON-RPC transport.
- Handles `initialize`, `tools/list`, `tools/call`.
- Tools exposés: `list_repos`, `search_code`, `get_file`, `get_context`, `get_status`.
- Emits structured logs with request correlation.

4. `internal/search`
- Business service for code search use-cases.
- Input validation and orchestration.
- Applies default directory exclusion filters for noisy paths.
- Exposes local file navigation primitives (`GetFile`, `GetContext`) constrained by allowed roots.
- Resolves repo logical names to local paths for relative file access.

5. `internal/zoekt`
- Backend contract (`Searcher`) and concrete adapters.
- HTTP adapter with retry and payload-shape resilience.
- Supports repo scoping through Zoekt query syntax (`r:^...$`).

## Extension points

- Add repository metadata endpoint normalization for more Zoekt variants.
- Add optional auth/token mode for non-local deployments.
