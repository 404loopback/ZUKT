#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

mkdir -p zoekt-index repos

echo "[zoekt] indexing repositories from ./repos into ./zoekt-index ..."
docker compose --profile tools run --rm zoekt-indexer
echo "[zoekt] indexing complete"
