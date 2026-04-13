#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export ZOEKT_SHARED_INDEX_DIR="${ZOEKT_SHARED_INDEX_DIR:-$ROOT_DIR/zoekt-index}"
mkdir -p "$ZOEKT_SHARED_INDEX_DIR" repos

echo "[zoekt] indexing repositories from ./repos into $ZOEKT_SHARED_INDEX_DIR ..."
docker compose --profile tools run --rm zoekt-indexer
echo "[zoekt] indexing complete"
