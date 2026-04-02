# token-meter

CLI that tracks Claude Code session token usage. Parses JSONL session logs → SQLite → `token-meter report`. Binary installed at `~/.local/bin/token-meter`. DB at `~/.local/share/token-tracker/usage.db`.

## Tech Stack

- **Go 1.25.0**
- **Frameworks:** SQLite

## Commands

```bash
go build ./...
go test ./...
go vet ./...
```

