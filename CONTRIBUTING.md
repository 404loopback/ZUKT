# Contributing

## Development setup

```bash
go version
go test ./...
```

## Code standards

- Keep `internal/mcp` transport/protocol only.
- Keep Zoekt backend logic in `internal/zoekt` adapters.
- Add/adjust tests for behavior changes.
- Run `gofmt` before commit.

## Commit convention

Use clear conventional commits when possible:
- `feat:` new feature
- `fix:` bug fix
- `refactor:` non-functional structural improvement
- `chore:` maintenance/bootstrap
