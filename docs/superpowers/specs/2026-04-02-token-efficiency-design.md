# Token Efficiency System — Design Spec
**Date:** 2026-04-02
**Project:** `~/projects/token-efficency`

## Goal

Track Claude Code token usage across all sessions and projects. Surface cost and cache efficiency data through a CLI and Nexus integration — without requiring any changes to Go tools (none call the Claude API directly).

## Scope

- **In scope:** Claude Code session token tracking (main sessions + subagents), SQLite persistence, CLI reporting, Nexus note integration, Stop hook automation
- **Out of scope:** Go tool instrumentation (no Claude API calls found), per-turn granularity, context health/compaction detection

---

## Architecture

```
Claude Code sessions
  └─ JSONL files (~/.claude/projects/**/<session-id>.jsonl)
                  (~/.claude/projects/**/<session-id>/subagents/*.jsonl)
        │
        ▼ Stop hook
  token-meter ingest --session-id $CLAUDE_SESSION_ID --project-dir $CLAUDE_PROJECT_DIR
        │
        ├─ writes to → ~/.local/share/token-tracker/usage.db  (SQLite)
        └─ writes to → nexus note (one-line session summary)
        │
        ▼ on demand
  token-meter report
```

---

## Data Collection

### JSONL Source

Each assistant turn in Claude Code's JSONL logs contains a `usage` object:

```json
{
  "type": "assistant",
  "message": {
    "model": "claude-sonnet-4-6",
    "usage": {
      "input_tokens": 2,
      "cache_creation_input_tokens": 11344,
      "cache_read_input_tokens": 6692,
      "output_tokens": 11
    }
  },
  "sessionId": "abc123",
  "cwd": "/home/digitalghost",
  "timestamp": "2026-04-02T18:41:41.175Z"
}
```

### Ingest Logic

`token-meter ingest` is called by the Stop hook with the current working directory. It:

1. Encodes the cwd to locate the project dir under `~/.claude/projects/`
2. Finds the most recently modified JSONL in that directory (= current session)
3. Finds all subagent JSONLs under `<session-id>/subagents/*.jsonl`
4. Parses all files, summing token fields across all assistant turns
5. Derives project name from `cwd` (basename of the working directory)
6. Writes one row to SQLite — `model` field is the model used by the main session JSONL; subagent models are not separately tracked
7. Fires `nexus note` with a one-line summary

> **Implementation note:** Claude Code's Stop hook env vars are unverified. The command should use `$(pwd)` for project dir and discover the session JSONL by recency rather than by session ID, to avoid dependency on undocumented env vars.

### Project dir encoding

Claude Code encodes the project path as the directory name under `~/.claude/projects/` by replacing `/` with `-` and stripping the leading `-`. The ingest command must replicate this encoding to locate the right directory.

---

## Storage

**Path:** `~/.local/share/token-tracker/usage.db`

**Schema:**

```sql
CREATE TABLE IF NOT EXISTS sessions (
  session_id            TEXT PRIMARY KEY,
  project               TEXT NOT NULL,
  started_at            TIMESTAMP NOT NULL,
  ended_at              TIMESTAMP NOT NULL,
  model                 TEXT NOT NULL,
  input_tokens          INTEGER NOT NULL DEFAULT 0,
  output_tokens         INTEGER NOT NULL DEFAULT 0,
  cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
  cache_read_tokens     INTEGER NOT NULL DEFAULT 0,
  subagent_count        INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_sessions_project    ON sessions(project);
CREATE INDEX IF NOT EXISTS idx_sessions_started_at ON sessions(started_at);
CREATE INDEX IF NOT EXISTS idx_sessions_model      ON sessions(model);
```

`cache_creation_tokens` maps to `cache_creation_input_tokens` in the JSONL.
`cache_read_tokens` maps to `cache_read_input_tokens` in the JSONL.

---

## CLI

### Binary

- **Name:** `token-meter`
- **Location:** `~/.local/bin/token-meter` (same as nexus, already in PATH)
- **Module:** `token-meter` in `~/projects/token-efficency/`

### Subcommands

#### `token-meter ingest`

```
token-meter ingest --project-dir <path>
```

Called by Stop hook with `$(pwd)` as `--project-dir`. Discovers the current session JSONL by finding the most recently modified file in the encoded project directory. Silent on success. Exits 0 even on partial failure to avoid disrupting session close.

#### `token-meter report`

```
token-meter report [--days N]   # default: 7
```

Output format:

```
TODAY — 2026-04-02
  sessions: 3   subagents: 7   cache efficiency: 73%

  input: 42,310   output: 3,450   cache_read: 18,200

7-DAY ROLLING
  input: 284,100   output: 21,800   cache efficiency: 68%

BY PROJECT (7d)          tokens    cache%
  digitalghost          112,400      71%
  dtg-obsidian-mcp       89,200      65%
  nexus                  52,100      74%

BY MODEL (7d)
  sonnet   71%  ████████████████████
  haiku    22%  ██████
  opus      7%  ██

TOP SESSIONS (7d)
  2026-04-01  dtg-obsidian-mcp   84,200  (8 subagents)
  2026-03-31  digitalghost       61,400  (3 subagents)
```

**Cache efficiency** = `cache_read_tokens / (input_tokens + cache_read_tokens)` × 100. Headline metric for prompt caching health.

**Model breakdown** validates that CLAUDE.md model routing is working — haiku should appear for search/analysis tasks.

---

## Automation

### Stop Hook

Added to `~/.claude/settings.json` under `hooks.Stop`:

```json
"Stop": [
  {
    "hooks": [
      {
        "type": "command",
        "command": "token-meter ingest --project-dir \"$(pwd)\"",
        "timeout": 10,
        "statusMessage": "Recording token usage..."
      }
    ]
  }
]
```

Runs silently after every session. Failure does not affect session close.

### Nexus Note

Written by `ingest` via:

```
nexus note "token-meter: <project> | <input> in / <output> out | cache <N>% | <subagent_count> subagents | <model>"
```

Example:
```
token-meter: digitalghost | 42,310 in / 3,450 out | cache 73% | 3 subagents | sonnet
```

Surfaces token cost alongside work context in `nexus context` and `nexus report`.

---

## Build & Install

```
cd ~/projects/token-efficency
go build -o ~/.local/bin/token-meter .
```

No external dependencies beyond the Go standard library and `modernc.org/sqlite` (pure-Go SQLite driver, no cgo required).

---

## Non-Goals

- Per-turn token breakdown (session-level is sufficient for v1)
- Context health / compaction detection (no reliable JSONL marker exists)
- Retroactive backfill of historical sessions (can be added later)
- TUI / live dashboard (future iteration once data patterns are understood)
