# Zoekt MCP Wrapper

Serveur MCP (Model Context Protocol) en Go qui encapsule Zoekt et expose des outils de recherche de code pour des clients LLM.

## Objectifs

- Exposer Zoekt via une interface MCP claire et stable.
- Garder un découplage strict entre protocole MCP et backend Zoekt.
- Préparer une base maintenable, testable et extensible (auth, observabilité, multi-tenant).

## Architecture

```text
cmd/zoekt-mcp/
  main.go                  # Point d'entrée

internal/app/
  app.go                   # Bootstrap + wiring

internal/config/
  config.go                # Chargement/validation de config

internal/mcp/
  server.go                # Boucle JSON-RPC MCP (stdin/stdout)

internal/search/
  service.go               # Cas d'usage: search_code, list_repos

internal/zoekt/
  interface.go             # Contrat Searcher
  mock.go                  # Implémentation locale pour dev/tests

docs/
  architecture.md          # Détails d'architecture et roadmap
```

Principe clé:
- `internal/mcp` dépend du service métier (`internal/search`), jamais directement de Zoekt.
- `internal/search` dépend de l'interface `zoekt.Searcher`, pas d'une implémentation concrète.
- Les adapters Zoekt (HTTP/CLI) sont remplaçables sans casser l'API MCP.

## Outils MCP exposés (v0)

- `search_code`
  - Input: `query`, `repo` (optionnel), `limit` (optionnel)
  - Output: liste de résultats (`repo`, `file`, `line`, `snippet`)
- `list_repos`
  - Input: aucun
  - Output: liste des repositories indexés

## Démarrage

### Prérequis

- Go `1.22+`

### Variables d'environnement

- `MCP_SERVER_NAME` (défaut: `zoekt-mcp-wrapper`)
- `MCP_SERVER_VERSION` (défaut: `0.1.0`)
- `ZOEKT_BACKEND` (`mock` par défaut, futur: `http`)
- `ZOEKT_HTTP_BASE_URL` (réservé pour backend HTTP)
- `ZOEKT_HTTP_TIMEOUT` (défaut: `5s`)

### Run

```bash
go run ./cmd/zoekt-mcp
```

Le serveur lit des messages JSON-RPC 2.0 sur `stdin` et écrit sur `stdout`.

## Initialiser le repo GitHub

1. Créer un repository vide (ex: `zoekt-mcp-wrapper`).
2. Initialiser Git localement:

```bash
git init
git branch -M main
git add .
git commit -m "chore: bootstrap zoekt MCP wrapper"
```

3. Connecter le remote et pousser:

```bash
git remote add origin git@github.com:<org>/zoekt-mcp-wrapper.git
git push -u origin main
```

4. Activer les protections de branche `main`:
- Pull request obligatoire
- CI obligatoire
- Push direct interdit

## Feuille de route

1. Implémenter adapter Zoekt HTTP réel (`internal/zoekt/http.go`).
2. Ajouter authentification backend + timeouts + retry policy.
3. Ajouter tests d'intégration MCP.
4. Ajouter observabilité (logs structurés + métriques).
