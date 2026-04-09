#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export ZOEKT_BACKEND="${ZOEKT_BACKEND:-http}"
export ZOEKT_AUTOPILOT="${ZOEKT_AUTOPILOT:-true}"
export ZOEKT_HTTP_BASE_URL="${ZOEKT_HTTP_BASE_URL:-http://127.0.0.1:6070}"
export ZOEKT_INDEX_DIR="${ZOEKT_INDEX_DIR:-$ROOT_DIR/zoekt-index}"
export ZOEKT_FORCE_REINDEX="${ZOEKT_FORCE_REINDEX:-false}"
export ZOEKT_ALLOWED_ROOTS="${ZOEKT_ALLOWED_ROOTS:-$HOME}"
export MCP_SERVER_NAME="${MCP_SERVER_NAME:-zoekt-mcp-wrapper}"
export MCP_SERVER_VERSION="${MCP_SERVER_VERSION:-0.1.0}"
export GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}"

mkdir -p "$GOCACHE"

if [ "${ZOEKT_AUTOPILOT}" = "true" ] && [ -z "${ZOEKT_REPOS:-}" ]; then
  echo "warning: ZOEKT_REPOS is empty; autopilot will use repos marked by .zukt files in workspace" >&2
fi

exec go run ./cmd/zoekt-mcp
