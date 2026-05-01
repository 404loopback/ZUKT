# ZUKT - Zoekt MCP with Local Semantic Search

Serveur MCP local-first en Go pour exposer une recherche code `lexical + semantic + hybrid`.

## Contrat runtime

- `zukt` reste un binaire MCP/search.
- Zoekt reste le backend lexical.
- Le backend sémantique est configurable (`disabled`, `hash`, `qdrant`).
- En cas d'indisponibilité Zoekt au démarrage: fail-fast explicite.

## Architecture cible

```text
Codex
  ↓ MCP stdio
npx zukt-mcp
  ├─ vérifie/lance Qdrant (si backend=qdrant)
  ├─ vérifie Ollama + modèle embedding
  └─ lance zukt (stdio forward)

zukt
  ├─ Zoekt       -> lexical search
  ├─ hash/qdrant -> semantic search backend
  └─ hybrid      -> fusion lexical + semantic (RRF)
```

## Modes de recherche

- `lexical`: Zoekt uniquement.
- `semantic`: backend vectoriel uniquement.
- `hybrid`: Zoekt + backend vectoriel (fusion RRF).

## Quickstart

### 1) Démarrer Zoekt localement

Exemple: `zoekt-webserver` sur `127.0.0.1:6070` avec index à jour.

### 2) Démarrer MCP

Option A (historique):

```bash
./scripts/run-mcp.sh
```

Option B (recommandée, wrapper):

```bash
npx -y zukt-mcp
```

## Configuration

Variables principales:

- `ZOEKT_BACKEND=http`
- `ZOEKT_HTTP_BASE_URL=http://127.0.0.1:6070`
- `ZOEKT_HTTP_TIMEOUT=5s`
- `ZOEKT_ALLOWED_ROOTS=/home/your-user`
- `ZOEKT_EXCLUDE_DIRS=.git,node_modules,.venv,dist,build`

Sémantique:

```env
ZUKT_SEMANTIC_BACKEND=hash

ZUKT_QDRANT_URL=http://127.0.0.1:6333
ZUKT_QDRANT_COLLECTION_PREFIX=zukt

ZUKT_EMBEDDING_PROVIDER=ollama
ZUKT_OLLAMA_URL=http://127.0.0.1:11434
ZUKT_EMBEDDING_MODEL=nomic-embed-text
ZUKT_EMBEDDING_DIM=768

ZUKT_CHUNK_LINES=80
ZUKT_CHUNK_OVERLAP=20
ZUKT_MAX_FILE_BYTES=1048576
```

Valeurs backend:

- `ZUKT_SEMANTIC_BACKEND=disabled`
- `ZUKT_SEMANTIC_BACKEND=hash`
- `ZUKT_SEMANTIC_BACKEND=qdrant`

## Tools MCP exposés

- `search_code` (`query`, `repo?`, `limit?`, `mode?`, `verbosity?`)
- `prepare_semantic_index` (`repo`)
- `list_repos`
- `get_status`
- `get_file`
- `get_context`

Notes:

- `search_code.mode`: `lexical|semantic|hybrid` (défaut: `hybrid`).
- `search_code.query` accepte la syntaxe Zoekt (`r:`, `file:`, `sym:`, `lang:`, `case:`).
- `semantic` exige un `repo` résolu localement.

## Wrapper `zukt-mcp`

Dans `npm/zukt-mcp`:

- `zukt-mcp` (run MCP)
- `zukt-mcp doctor`
- `zukt-mcp setup`
- `zukt-mcp prepare --repo <name-or-path>`

Qdrant Docker Compose fourni: `npm/zukt-mcp/docker/docker-compose.yml`.

## Codex config example

```json
{
  "mcpServers": {
    "zukt": {
      "command": "npx",
      "args": ["-y", "zukt-mcp"],
      "env": {
        "ZOEKT_ALLOWED_ROOTS": "/home/gh0st",
        "ZUKT_SEMANTIC_BACKEND": "qdrant"
      }
    }
  }
}
```

## Docs

- [semantic-search.md](/home/gh0st/ZUKT/docs/semantic-search.md)
- [codex-routing.md](/home/gh0st/ZUKT/docs/codex-routing.md)
- [architecture.md](/home/gh0st/ZUKT/docs/architecture.md)
- [install-local-prod.md](/home/gh0st/ZUKT/docs/install-local-prod.md)
