#!/usr/bin/env bash
set -euo pipefail

if [ "${1:-}" = "" ]; then
  echo "usage: $0 /absolute/path/to/repo"
  exit 1
fi

SRC_REPO="$1"
SRC_PARENT="$(dirname "$SRC_REPO")"
SRC_BASE="$(basename "$SRC_REPO")"

if [ ! -d "$SRC_REPO" ]; then
  echo "[zoekt] source repo path not found: $SRC_REPO"
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

mkdir -p zoekt-index

echo "[zoekt] indexing $SRC_REPO ..."
docker run --rm \
  -v "$ROOT_DIR/zoekt-index:/data/index" \
  -v "$SRC_PARENT:/data/srcroot:ro" \
  zukt-zoekt-web \
  zoekt-index -index /data/index "/data/srcroot/$SRC_BASE"
echo "[zoekt] indexing complete"
