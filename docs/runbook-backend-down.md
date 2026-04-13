# Runbook: Incident `backend down`

## Symptômes

- `zukt` ne démarre pas.
- Erreur startup contenant `zoekt backend unreachable`.
- MCP `get_status` retourne `health=down`.

## Vérifications immédiates

1. Vérifier Zoekt webserver:
```bash
curl -fsS http://127.0.0.1:6070/api/list
```
2. Vérifier les conteneurs (si Docker):
```bash
docker compose ps
docker compose logs --tail=100 zoekt-web
```
3. Vérifier le dossier d’index partagé:
```bash
ls -la "${ZOEKT_SHARED_INDEX_DIR:-/home/gh0st/ZUKT/zoekt-index}"
```

## Remédiation rapide

1. Redémarrer le webserver:
```bash
cd /home/gh0st/ZUKT
./scripts/down.sh
./scripts/up.sh
```
2. Réindexer si nécessaire:
```bash
cd /home/gh0st/ZUKT
./scripts/indexer-local.sh /absolute/path/to/repo
```
3. Revalider:
```bash
curl -fsS http://127.0.0.1:6070/api/list
```

## Prévention

- Superviser le service webserver (systemd/launchd/Docker restart policy).
- Planifier l’indexation (timer systemd/launchd/cron).
- Vérifier périodiquement `get_status` côté client MCP.
