#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

mkdir -p zoekt-index repos

echo "[zoekt] starting webserver on http://localhost:6070 ..."
docker compose up -d zoekt-web
echo "[zoekt] webserver started"

