#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 1 ]; then
  echo "usage: $0 /absolute/path/to/repo [/absolute/path/to/another/repo ...]"
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INDEX_DIR="${ZOEKT_SHARED_INDEX_DIR:-$ROOT_DIR/zoekt-index}"

mkdir -p "$INDEX_DIR"

index_with_git_tool() {
  local repo
  for repo in "$@"; do
    if [ ! -d "$repo/.git" ]; then
      echo "[indexer] not a git repository: $repo"
      exit 1
    fi
    echo "[indexer] zoekt-git-index $repo -> $INDEX_DIR"
    zoekt-git-index -index "$INDEX_DIR" "$repo"
  done
}

index_with_generic_tool() {
  local repo
  for repo in "$@"; do
    if [ ! -d "$repo" ]; then
      echo "[indexer] path not found: $repo"
      exit 1
    fi
    echo "[indexer] zoekt-index $repo -> $INDEX_DIR"
    zoekt-index -index "$INDEX_DIR" "$repo"
  done
}

if command -v zoekt-git-index >/dev/null 2>&1; then
  index_with_git_tool "$@"
elif command -v zoekt-index >/dev/null 2>&1; then
  index_with_generic_tool "$@"
else
  echo "[indexer] neither zoekt-git-index nor zoekt-index is available in PATH"
  exit 1
fi

echo "[indexer] indexing complete"
