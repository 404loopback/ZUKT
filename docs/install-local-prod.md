# Install Locale Prod: Docker webserver + indexer host

Objectif: séparer clairement recherche MCP (`zukt`) et indexation (host).

## 1. Dossier d’index partagé

Choisir un dossier unique (exemple):

```bash
export ZOEKT_SHARED_INDEX_DIR=/var/lib/zoekt/index
mkdir -p "$ZOEKT_SHARED_INDEX_DIR"
```

Ce dossier doit être utilisé par:
- `zoekt-webserver` (lecture index)
- `scripts/indexer-local.sh` (écriture index)

## 2. Démarrer le webserver Zoekt (Docker)

```bash
cd /home/gh0st/ZUKT
ZOEKT_SHARED_INDEX_DIR="$ZOEKT_SHARED_INDEX_DIR" ./scripts/up.sh
```

## 3. Indexer côté host

Installer `zoekt-git-index` (ou `zoekt-index`) sur l’hôte, puis:

```bash
cd /home/gh0st/ZUKT
ZOEKT_SHARED_INDEX_DIR="$ZOEKT_SHARED_INDEX_DIR" ./scripts/indexer-local.sh /absolute/path/repo-a /absolute/path/repo-b
```

## 4. Démarrer MCP

```bash
cd /home/gh0st/ZUKT
ZOEKT_HTTP_BASE_URL=http://127.0.0.1:6070 ./scripts/run-mcp.sh
```

## 5. Service d’indexation périodique (exemples)

### systemd service + timer

`/etc/systemd/system/zoekt-indexer.service`:

```ini
[Unit]
Description=Zoekt host index refresh
After=network-online.target

[Service]
Type=oneshot
Environment=ZOEKT_SHARED_INDEX_DIR=/var/lib/zoekt/index
ExecStart=/home/gh0st/ZUKT/scripts/indexer-local.sh /home/gh0st/repos/repo-a /home/gh0st/repos/repo-b
```

`/etc/systemd/system/zoekt-indexer.timer`:

```ini
[Unit]
Description=Run Zoekt index refresh every 15 minutes

[Timer]
OnBootSec=2m
OnUnitActiveSec=15m
Unit=zoekt-indexer.service

[Install]
WantedBy=timers.target
```

Activation:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now zoekt-indexer.timer
```

### launchd (macOS) exemple

`~/Library/LaunchAgents/com.zukt.zoekt-indexer.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key><string>com.zukt.zoekt-indexer</string>
    <key>ProgramArguments</key>
    <array>
      <string>/home/gh0st/ZUKT/scripts/indexer-local.sh</string>
      <string>/home/gh0st/repos/repo-a</string>
      <string>/home/gh0st/repos/repo-b</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
      <key>ZOEKT_SHARED_INDEX_DIR</key><string>/var/lib/zoekt/index</string>
    </dict>
    <key>StartInterval</key><integer>900</integer>
    <key>RunAtLoad</key><true/>
  </dict>
</plist>
```

### Cron fallback

```cron
*/15 * * * * ZOEKT_SHARED_INDEX_DIR=/var/lib/zoekt/index /home/gh0st/ZUKT/scripts/indexer-local.sh /home/gh0st/repos/repo-a /home/gh0st/repos/repo-b >> /var/log/zoekt-indexer.log 2>&1
```
