# Zoekt MCP Wrapper

Serveur MCP (Model Context Protocol) local-first en Go pour exposer Zoekt à Codex.

## Production Standard (Local)

- Distribution principale: binaire Go + fichier `mcp.json`.
- Runtime par défaut: **autopilot** (startup Zoekt + indexation + service MCP).
- Sécurité v1: **localhost only** + whitelist explicite des chemins autorisés.

## Quickstart (1 commande côté MCP)

1. Vérifier Docker disponible.
2. Optionnel: configurer `ZOEKT_REPOS` (CSV de chemins absolus).
3. Sinon, utiliser des marqueurs `.zukt` à la racine de chaque repo à gérer.
3. Lancer le serveur MCP:

```bash
cd /home/gh0st/ZUKT
ZOEKT_REPOS=/home/gh0st/PITANCE ./scripts/run-mcp.sh
```

En autopilot, le wrapper:
1. vérifie Zoekt HTTP sur `127.0.0.1:6070`
2. démarre `zoekt-web` si nécessaire
3. indexe les repos configurés (`ZOEKT_REPOS`) et/ou marqués par un fichier `.zukt`
4. expose MCP sur stdin/stdout

## Configuration

Variables publiques:

- `ZOEKT_BACKEND` (`http` par défaut)
- `ZOEKT_AUTOPILOT` (`true` par défaut)
- `ZOEKT_HTTP_BASE_URL` (`http://127.0.0.1:6070` par défaut)
- `ZOEKT_HTTP_TIMEOUT` (`5s` par défaut)
- `ZOEKT_REPOS` (CSV, optionnel; fusionné avec repos marqués `.zukt`)
- `ZOEKT_ALLOWED_ROOTS` (CSV, défaut: `$HOME`)
- `ZOEKT_INDEX_DIR` (défaut: `./zoekt-index`)
- `ZOEKT_FORCE_REINDEX` (`false` par défaut)
- `ZOEKT_EXCLUDE_DIRS` (défaut: `.git,node_modules,.venv,dist,build`)
- `MCP_SERVER_NAME` (défaut: `zukt`)
- `MCP_SERVER_VERSION` (défaut: `0.1.0`)

Contraintes de sécurité:
- `ZOEKT_HTTP_BASE_URL` doit être local (`localhost` ou IP loopback).
- chaque repo (env ou `.zukt`) doit exister et être sous `ZOEKT_ALLOWED_ROOTS`.

## Tools MCP exposés

- `list_repos`
- `search_code` (`query`, `repo?`, `limit?`)
- `repos_list` (liste les repos gérés via fichiers `.zukt`)
- `repos_add` (`path`) crée un fichier `.zukt` à la racine du repo
- `repos_remove` (`path`) supprime le fichier `.zukt`
- `repos_index` (`force?`) indexe tous les repos gérés (`.zukt`)
- `index_workspace` (`workspace?`, `force?`) découvre les repos git, crée `.zukt`, puis indexe

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
        "ZOEKT_AUTOPILOT": "true",
        "ZOEKT_REPOS": "/home/gh0st/PITANCE",
        "ZOEKT_ALLOWED_ROOTS": "/home/gh0st"
      }
    }
  }
}
```

## Scripts utiles

- `scripts/run-mcp.sh`: démarrage MCP (autopilot activé par défaut)
- `scripts/up.sh`: démarre Zoekt webserver
- `scripts/down.sh`: arrête Zoekt
- `scripts/index-path.sh /abs/path/repo`: indexation manuelle d’un repo
- `scripts/index-local.sh`: indexe le contenu de `./repos`

## CI/Release

- CI: format check + tests (`go test ./...`)
- Release: build multi-OS sur tags `v*` via `.github/workflows/release.yml`
