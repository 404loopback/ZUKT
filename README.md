# Zoekt MCP Wrapper

Serveur MCP (Model Context Protocol) local-first en Go pour exposer Zoekt à Codex.

## Contrat Runtime

- `zukt` ne fait que MCP/search.
- Backend requis: Zoekt HTTP local (`localhost`/loopback).
- `zukt` n’indexe pas et ne démarre pas Zoekt.
- En cas d’indisponibilité backend au démarrage: fail-fast avec message opérable.

## Quickstart (1 commande côté MCP)

1. Démarrer Zoekt localement (ex: `zoekt-webserver` sur `127.0.0.1:6070`) et maintenir son index à jour en dehors de `zukt`.
2. Lancer le serveur MCP:

```bash
cd /home/gh0st/ZUKT
./scripts/run-mcp.sh
```

Au démarrage, `zukt` effectue un health check HTTP explicite sur Zoekt.
Si Zoekt est indisponible, `zukt` échoue en erreur claire.

## Configuration

Variables publiques:

- `ZOEKT_BACKEND` (`http` uniquement)
- `ZOEKT_HTTP_BASE_URL` (`http://127.0.0.1:6070` par défaut)
- `ZOEKT_HTTP_TIMEOUT` (`5s` par défaut)
- `ZOEKT_ALLOWED_ROOTS` (CSV, défaut: `$HOME`, utilisé pour validation locale)
- `ZOEKT_EXCLUDE_DIRS` (défaut: `.git,node_modules,.venv,dist,build`)
- `MCP_SERVER_NAME` (défaut: `zukt`)
- `MCP_SERVER_VERSION` (défaut: `0.1.0`)

Contraintes de sécurité:
- `ZOEKT_HTTP_BASE_URL` doit être local (`localhost` ou IP loopback).

Variables legacy de transition (acceptées mais ignorées avec warning):
- `ZOEKT_AUTOPILOT`
- `ZOEKT_REPOS`
- `ZOEKT_INDEX_DIR`
- `ZOEKT_FORCE_REINDEX`

## Tools MCP exposés

- `list_repos`
- `search_code` (`query`, `repo?`, `limit?`)
- `get_status` (backend URL, timeout, health)

## Codex config (prêt à coller)

Utiliser [configs/mcp.codex.json](/home/gh0st/ZUKT/configs/mcp.codex.json), ou:

```json
{
  "mcpServers": {
    "zoekt-local": {
      "command": "/bin/bash",
      "args": [
        "-lc",
        "cd /home/gh0st/ZUKT && ./scripts/run-mcp.sh"
      ],
      "env": {
        "ZOEKT_ALLOWED_ROOTS": "/home/gh0st"
      }
    }
  }
}
```

## Scripts utiles

- `scripts/run-mcp.sh`: démarrage MCP en mode search-only
- `scripts/indexer-local.sh`: indexation host officielle (`zoekt-git-index` ou `zoekt-index`) vers dossier partagé
- `scripts/up.sh`: démarre Zoekt webserver (optionnel)
- `scripts/down.sh`: arrête Zoekt
- `scripts/index-path.sh /abs/path/repo`: indexation manuelle d’un repo (hors `zukt`)
- `scripts/index-local.sh`: indexe le contenu de `./repos` (hors `zukt`)

## Ops & migration

- Runbook incident: [docs/runbook-backend-down.md](/home/gh0st/ZUKT/docs/runbook-backend-down.md)
- Install locale prod (webserver Docker + indexer host): [docs/install-local-prod.md](/home/gh0st/ZUKT/docs/install-local-prod.md)
- Migration Codex avant/après: [docs/migration-codex.md](/home/gh0st/ZUKT/docs/migration-codex.md)

## CI/Release

- CI: format check + tests (`go test ./...`)
- Release: build multi-OS sur tags `v*` via `.github/workflows/release.yml`
