# Changelog

## [0.3.0] - 2026-04-09

### Breaking
- Runtime contract is now strict: `zukt` only provides MCP/search against an already running Zoekt HTTP backend.
- Removed `internal/autopilot` and `internal/admin` (code + tests).
- Removed obsolete runtime fields in config (`ZOEKT_AUTOPILOT`, `ZOEKT_REPOS`, `ZOEKT_INDEX_DIR`, `ZOEKT_FORCE_REINDEX`) from active behavior.

### Added
- MCP tool `get_status` (backend URL, timeout, health).
- Startup fail-fast error with operable remediation context.
- Local host indexer script `scripts/indexer-local.sh`.
- Ops docs: backend incident runbook, local production install, Codex migration guide.

### Compatibility (transition release)
- Legacy env vars (`ZOEKT_AUTOPILOT`, `ZOEKT_REPOS`, `ZOEKT_INDEX_DIR`, `ZOEKT_FORCE_REINDEX`) are still accepted but ignored with deprecation warnings.
- Legacy vars will be removed in the next release.

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
