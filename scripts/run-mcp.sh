#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export ZOEKT_BACKEND="${ZOEKT_BACKEND:-http}"
export ZOEKT_HTTP_BASE_URL="${ZOEKT_HTTP_BASE_URL:-http://127.0.0.1:6070}"
export MCP_SERVER_NAME="${MCP_SERVER_NAME:-zoekt-mcp-wrapper}"
export MCP_SERVER_VERSION="${MCP_SERVER_VERSION:-0.1.0}"
export GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}"

mkdir -p "$GOCACHE"

exec go run ./cmd/zoekt-mcp

