# token-meter

A Go CLI that tracks Claude Code session token usage. After each session, a Stop hook automatically ingests the session's JSONL log into SQLite and fires a Nexus note. `token-meter report` prints a breakdown by project, model, and cache efficiency.

## Why

Claude Code doesn't surface token costs or context pressure in any persistent form. This tool makes them visible:

- **Cost visibility** — total input/output tokens per session, per project, per day
- **Cache efficiency** — what percentage of input was served from cache (lower cost, faster responses)
- **Model routing validation** — confirms that `haiku` is actually firing for simple tasks, not just `sonnet` for everything
- **Subagent accounting** — how many parallel agents ran per session

## Architecture

```
Claude Code session ends
  └─ Stop hook calls: token-meter ingest --project-dir "$(pwd)"
        │
        ├─ Finds latest JSONL in ~/.claude/projects/<encoded-path>/
        ├─ Finds subagent JSONLs in <session-id>/subagents/
        ├─ Parses all assistant turns, deduplicates streaming duplicates by message ID
        ├─ Aggregates into one SessionRow
        ├─ Writes to ~/.local/share/token-tracker/usage.db (SQLite)
        └─ Fires: nexus note "token-meter: <project> | <in> / <out> | cache N%"

token-meter report
  └─ Queries SQLite → prints TODAY / ROLLING / BY PROJECT / BY MODEL / TOP SESSIONS
```

## Installation

```bash
cd ~/projects/token-efficency
go build -o ~/.local/bin/token-meter .
```

The Stop hook in `~/.claude/settings.json` is already wired:

```json
"Stop": [{
  "hooks": [{
    "type": "command",
    "command": "token-meter ingest --project-dir \"$(pwd)\"",
    "timeout": 10,
    "statusMessage": "Recording token usage..."
  }]
}]
```

Data is stored at `~/.local/share/token-tracker/usage.db`.

## Usage

### Report

```bash
token-meter report              # last 7 days (default)
token-meter report --days 30    # custom window
```

Example output:

```
TODAY — 2026-04-02
  sessions: 3   subagents: 22   cache efficiency: 94%

  input: 42,310   output: 67,726   cache_read: 14,183,263

7-DAY ROLLING
  input: 284,100   output: 121,800   cache efficiency: 89%

BY PROJECT (7d)          tokens    cache%
  digitalghost             14,186,365    100%
  dtg-obsidian-mcp          2,341,100     87%
  nexus                       841,200     91%

BY MODEL (7d)
  sonnet    71%  ██████████████
  haiku     22%  ████
  opus       7%  █

TOP SESSIONS (7d)
  2026-04-02  digitalghost             14,186,365 (22 subagents)
  2026-04-01  dtg-obsidian-mcp          2,341,100 (4 subagents)
```

**Model breakdown** is the key signal for routing health — if `haiku` never appears, the model routing rules in `~/.claude/CLAUDE.md` aren't firing.

**Cache efficiency** = `cache_read_tokens / (input_tokens + cache_read_tokens)`. Near 100% on long sessions is normal (prompt caching). Consistently low cache efficiency on short sessions suggests the system prompt isn't being cached.

### Manual ingest (backfill)

```bash
token-meter ingest --project-dir /home/digitalghost/projects/nexus
```

Always exits 0 — designed to be safe in Stop hook context.

## Development

```bash
go test ./...       # run all tests (18 tests)
go vet ./...        # vet
go build ./...      # build check
```

### File structure

| File | Responsibility |
|---|---|
| `jsonl.go` | `Entry` type, `parseJSONL`, `encodePath` |
| `db.go` | SQLite schema, `upsertSession`, query functions |
| `ingest.go` | `aggregateTokens`, file discovery, `runIngest`, `nexusNote` |
| `report.go` | `runReport`, `formatNum`, `barChart`, `shortModel` |
| `main.go` | CLI entry point, flag routing |

### Database schema

```sql
sessions (
  session_id            TEXT PRIMARY KEY,
  project               TEXT,
  started_at            TIMESTAMP,
  ended_at              TIMESTAMP,
  model                 TEXT,
  input_tokens          INTEGER,
  output_tokens         INTEGER,
  cache_creation_tokens INTEGER,
  cache_read_tokens     INTEGER,
  subagent_count        INTEGER
)
```

Query directly:

```bash
sqlite3 ~/.local/share/token-tracker/usage.db \
  "SELECT project, SUM(input_tokens+cache_read_tokens) FROM sessions GROUP BY project ORDER BY 2 DESC;"
```

## Design docs

- [`docs/superpowers/specs/2026-04-02-token-efficiency-design.md`](docs/superpowers/specs/2026-04-02-token-efficiency-design.md) — design spec
- [`docs/superpowers/plans/2026-04-02-token-meter-implementation.md`](docs/superpowers/plans/2026-04-02-token-meter-implementation.md) — implementation plan

## Known limitations

- **Encoding collision** — Claude Code encodes paths by replacing `/` with `-`, so `/a/b-c` and `/a-b/c` produce the same project dir name. Rare in practice.
- **No retroactive backfill** — only sessions after Stop hook installation are tracked. Run `token-meter ingest --project-dir <path>` manually to backfill a specific project.
- **Session discovery by recency** — ingest finds the most recently modified JSONL in the project dir. If two sessions end within the same second, one may be missed.
