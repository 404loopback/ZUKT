# Zuckt - Zoekt MCP Wrapper

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
- `search_code` (`query`, `repo?`, `limit?`, `mode?`)
- `prepare_semantic_index` (`repo`)
- `get_file` (`repo?`, `path`, `start_line?`, `end_line?`)
- `get_context` (`repo?`, `path`, `line`, `before?`, `after?`)
- `get_status` (backend URL, timeout, health)

### Détails importants

- `search_code.query` accepte la syntaxe Zoekt (ex: `r:`, `file:`, `sym:`, `lang:`, `case:`).
- `search_code.mode` accepte `lexical` ou `semantic` (défaut: `semantic`).
- `search_code.repo` accepte soit:
  - un nom logique (ex: `ZUKT`)
  - un chemin absolu de repo (validé contre `ZOEKT_ALLOWED_ROOTS`)
- `mode=semantic` correspond à une recherche fusionnée lexicale+sémantique (préparation automatique par repo).
- `get_file` / `get_context` lisent uniquement des fichiers locaux dans les roots autorisés (`ZOEKT_ALLOWED_ROOTS`).
- Si `path` est relatif, `repo` est requis pour `get_file` / `get_context`.
- `search_code`, `list_repos`, `get_status` renvoient un résumé court dans `result.content[0].text` et le payload complet dans `result.structuredContent`.
- `get_file` / `get_context` renvoient le contenu brut dans `result.content[0].text` et les métadonnées (sans duplication du champ `content`) dans `result.structuredContent`.

### Exemples MCP prêts à copier

#### 1) Full-text simple dans un repo logique

```json
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_code","arguments":{"query":"SearchCode","repo":"ZUKT","limit":5}}}
```

#### 2) Recherche symbole (Zoekt DSL)

```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_code","arguments":{"query":"sym:SearchCode r:ZUKT","limit":10}}}
```

#### 2b) Préparer l’index sémantique d’un repo

```json
{"jsonrpc":"2.0","id":21,"method":"tools/call","params":{"name":"prepare_semantic_index","arguments":{"repo":"ZUKT"}}}
```

#### 2c) Recherche sémantique (fusion lexical+sémantique)

```json
{"jsonrpc":"2.0","id":22,"method":"tools/call","params":{"name":"search_code","arguments":{"query":"app factory fastapi","repo":"PITANCE","mode":"semantic","limit":10}}}
```

#### 3) Recherche par fichier + langage (Zoekt DSL)

```json
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search_code","arguments":{"query":"file:service.go lang:go query is required","limit":10}}}
```

#### 4) Lire un extrait de fichier

```json
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_file","arguments":{"repo":"ZUKT","path":"internal/search/service.go","start_line":52,"end_line":110}}}
```

#### 5) Lire le contexte autour d’une ligne

```json
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"get_context","arguments":{"repo":"ZUKT","path":"internal/search/service.go","line":64,"before":12,"after":12}}}
```

#### 6) Lire avec chemin absolu (sans repo)

```json
{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"get_file","arguments":{"path":"/home/gh0st/ZUKT/internal/mcp/server.go","start_line":150,"end_line":240}}}
```

#### 7) Vérifier runtime/back-end

```json
{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"get_status","arguments":{}}}
```

### Notes de diagnostic rapide

- Erreur `repo "... is ambiguous"`: plusieurs repos du même nom existent sous `ZOEKT_ALLOWED_ROOTS`; utiliser un chemin absolu de repo.
- Erreur `path "... is outside allowed roots"`: ajuster `ZOEKT_ALLOWED_ROOTS` pour inclure le chemin attendu.
- Résultats vides: vérifier l’index Zoekt (indexation à jour) puis tester `search_code` sans `repo` pour isoler le filtre.

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
