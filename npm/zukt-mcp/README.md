# zukt-mcp

Local MCP runner for ZUKT.

`zukt-mcp` bootstraps local dependencies (Qdrant/Ollama checks) and starts `zukt` over stdio for MCP clients.

## Install / Run

```bash
npx -y zukt-mcp
```

## Commands

### `zukt-mcp`

Start the MCP server (stdio forward).

### `zukt-mcp doctor`

Check:

- Docker availability
- Qdrant reachability
- Ollama reachability
- embedding model presence

### `zukt-mcp setup`

Start Qdrant via Docker Compose and verify connectivity.

### `zukt-mcp prepare --repo <name-or-path> [--timeout-ms <ms>]`

Run `prepare_semantic_index` by spawning `zukt` and calling the MCP tool via an integrated JSON-RPC client.

Examples:

```bash
npx -y zukt-mcp prepare --repo ZUKT
npx -y zukt-mcp prepare --repo /home/me/dev/ZUKT --timeout-ms 180000
```

## Environment variables

### Runtime

- `ZUKT_SEMANTIC_BACKEND` (`disabled|hash|qdrant`)
- `ZUKT_QDRANT_URL` (default `http://127.0.0.1:6333`)
- `ZUKT_OLLAMA_URL` (default `http://127.0.0.1:11434`)
- `ZUKT_EMBEDDING_MODEL` (default `nomic-embed-text`)

### Local command override

- `ZUKT_BIN`: override command used to start zukt.
- `ZUKT_BIN_ARGS`: additional args for `ZUKT_BIN`.

By default, the runner executes:

```bash
go run ./cmd/zukt
```

(from the discovered ZUKT repository root).

## Publish checklist

```bash
cd npm/zukt-mcp
npm run check
npm run pack:dry-run
npm publish
```

For scoped packages, use `npm publish --access public`.
