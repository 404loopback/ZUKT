# Changelog

## [0.2.0] - 2026-04-09

### Added
- Autopilot mode for local-first orchestration (`probe -> start zoekt-web -> index repos -> serve MCP`)
- Config hardening for localhost-only HTTP backend and allowed repository roots
- Default noise directory filtering (`.git`, `node_modules`, `.venv`, `dist`, `build`)
- MCP integration smoke test and expanded HTTP adapter tests
- Release workflow for multi-platform binary artifacts

### Changed
- Default backend is now `http` with local URL `http://127.0.0.1:6070`
- `run-mcp.sh` now enforces `ZOEKT_REPOS` when autopilot is enabled

