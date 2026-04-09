# Changelog

## [0.2.0] - 2026-04-09

### Added
- Autopilot mode for local-first orchestration (`probe -> start zoekt-web -> index repos -> serve MCP`)
- Config hardening for localhost-only HTTP backend and allowed repository roots
- MCP repo management tools (`repos_list`, `repos_add`, `repos_remove`, `repos_index`, `index_workspace`)
- Per-project management marker file `.zukt` at repository root
- Default noise directory filtering (`.git`, `node_modules`, `.venv`, `dist`, `build`)
- MCP integration smoke test and expanded HTTP adapter tests
- Release workflow for multi-platform binary artifacts

### Changed
- Default backend is now `http` with local URL `http://127.0.0.1:6070`
- `run-mcp.sh` now allows startup with empty `ZOEKT_REPOS` and uses `.zukt` managed repos
