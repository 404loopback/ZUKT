# Migration Codex: avant / après

## Objectif

Passer d’un runtime mixte (orchestration interne) à un runtime strict MCP/search.

## Avant (legacy)

- `zukt` pouvait embarquer des comportements d’orchestration locale.
- Variables comme `ZOEKT_AUTOPILOT`, `ZOEKT_REPOS`, `ZOEKT_INDEX_DIR`, `ZOEKT_FORCE_REINDEX` pouvaient être utilisées.

## Après (release de transition)

- `zukt` ne fait que:
  - exposer MCP
  - appeler Zoekt HTTP
- Variables legacy:
  - encore acceptées
  - ignorées
  - warning de dépréciation au démarrage

## Configuration cible Codex

Conserver uniquement:
- `ZOEKT_BACKEND=http`
- `ZOEKT_HTTP_BASE_URL=http://127.0.0.1:6070`
- `ZOEKT_HTTP_TIMEOUT=5s`
- `ZOEKT_ALLOWED_ROOTS=...` (optionnel selon politique locale)
- `ZOEKT_EXCLUDE_DIRS=...` (optionnel)

## Plan recommandé

1. Arrêter d’utiliser les variables legacy côté `mcp.json`/scripts locaux.
2. Déployer un indexer host externe (`scripts/indexer-local.sh` + timer/cron).
3. Vérifier le tool MCP `get_status`.
4. Release suivante: suppression définitive des variables legacy.
