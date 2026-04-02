# Token Meter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `token-meter`, a Go CLI that ingests Claude Code session JSONL logs into SQLite and reports token usage, cache efficiency, and model routing breakdown.

**Architecture:** A Stop hook calls `token-meter ingest --project-dir $(pwd)` after every Claude Code session. Ingest discovers the latest session JSONL, parses token usage from all assistant turns (deduplicating by message ID), writes one row to SQLite, and fires a Nexus note. `token-meter report` queries the DB and prints a structured summary.

**Tech Stack:** Go 1.21+, `modernc.org/sqlite` (pure-Go, no cgo), `database/sql`, standard library only otherwise.

---

## File Structure

```
~/projects/token-efficency/
├── main.go          -- entry point, routes --help / ingest / report subcommands
├── jsonl.go         -- Entry type, parseJSONL, encodePath
├── db.go            -- openDB, initSchema, upsertSession, query funcs
├── ingest.go        -- aggregateTokens, findLatestJSONL, findSubagentJSONLs, runIngest
├── report.go        -- runReport, formatting helpers
├── jsonl_test.go    -- tests for parseJSONL, encodePath
├── db_test.go       -- tests for schema, upsert, all query funcs
├── ingest_test.go   -- tests for aggregateTokens (dedup, summation, field extraction)
├── report_test.go   -- tests for cacheEfficiency, barChart, formatNum
├── go.mod
└── go.sum
```

---

## Task 1: Initialize Go module

**Files:**
- Create: `go.mod`, `go.sum`

- [ ] **Step 1: Init module**

```bash
cd ~/projects/token-efficency
go mod init token-meter
```

Expected: `go.mod` created with `module token-meter`.

- [ ] **Step 2: Add SQLite dependency**

```bash
go get modernc.org/sqlite@latest
```

Expected: `go.mod` updated, `go.sum` created.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: initialize Go module with sqlite dependency"
```

---

## Task 2: JSONL types and parser (TDD)

**Files:**
- Create: `jsonl_test.go`
- Create: `jsonl.go`

- [ ] **Step 1: Write failing test**

Create `jsonl_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseJSONL(t *testing.T) {
	content := `{"type":"user","message":{"role":"user","content":"hello"},"sessionId":"sess1","cwd":"/home/digitalghost","timestamp":"2026-04-02T10:00:00.000Z"}
{"type":"assistant","message":{"id":"msg_001","model":"claude-sonnet-4-6","usage":{"input_tokens":100,"cache_creation_input_tokens":500,"cache_read_input_tokens":200,"output_tokens":50}},"sessionId":"sess1","cwd":"/home/digitalghost","timestamp":"2026-04-02T10:00:05.000Z"}
{"type":"assistant","message":{"id":"msg_001","model":"claude-sonnet-4-6","usage":{"input_tokens":100,"cache_creation_input_tokens":500,"cache_read_input_tokens":200,"output_tokens":50}},"sessionId":"sess1","cwd":"/home/digitalghost","timestamp":"2026-04-02T10:00:05.100Z"}
{"type":"assistant","message":{"id":"msg_002","model":"claude-haiku-4-5","usage":{"input_tokens":20,"cache_creation_input_tokens":0,"cache_read_input_tokens":300,"output_tokens":10}},"sessionId":"sess1","cwd":"/home/digitalghost","timestamp":"2026-04-02T10:01:00.000Z"}`

	f, err := os.CreateTemp(t.TempDir(), "session*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(content)
	f.Close()

	entries, err := parseJSONL(f.Name())
	if err != nil {
		t.Fatalf("parseJSONL error: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	// first assistant entry
	e := entries[1]
	if e.Type != "assistant" {
		t.Errorf("expected type assistant, got %s", e.Type)
	}
	if e.Message.ID != "msg_001" {
		t.Errorf("expected msg_001, got %s", e.Message.ID)
	}
	if e.Message.Usage.InputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", e.Message.Usage.InputTokens)
	}
	if e.Message.Usage.CacheCreationInputTokens != 500 {
		t.Errorf("expected 500 cache creation tokens, got %d", e.Message.Usage.CacheCreationInputTokens)
	}
	if e.Message.Usage.CacheReadInputTokens != 200 {
		t.Errorf("expected 200 cache read tokens, got %d", e.Message.Usage.CacheReadInputTokens)
	}
	if e.SessionID != "sess1" {
		t.Errorf("expected sessionId sess1, got %s", e.SessionID)
	}
	if e.CWD != "/home/digitalghost" {
		t.Errorf("expected cwd /home/digitalghost, got %s", e.CWD)
	}
}

func TestEncodePathRoundTrip(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"/home/digitalghost", "home-digitalghost"},
		{"/home/digitalghost/projects/nexus", "home-digitalghost-projects-nexus"},
		{"/home/digitalghost/projects/dtg-obsidian-mcp", "home-digitalghost-projects-dtg-obsidian-mcp"},
	}
	for _, c := range cases {
		got := encodePath(c.input)
		if got != c.want {
			t.Errorf("encodePath(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestParseJSONLSkipsMalformed(t *testing.T) {
	content := "not json\n{\"type\":\"user\",\"sessionId\":\"s1\",\"cwd\":\"/tmp\",\"timestamp\":\"2026-04-02T10:00:00.000Z\"}\n"
	f, _ := os.CreateTemp(t.TempDir(), "*.jsonl")
	f.WriteString(content)
	f.Close()

	entries, err := parseJSONL(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 valid entry, got %d", len(entries))
	}
}

// silence unused import warning during test compilation
var _ = filepath.Join
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd ~/projects/token-efficency && go test ./... 2>&1 | head -20
```

Expected: compile error — `parseJSONL`, `encodePath` undefined.

- [ ] **Step 3: Create `jsonl.go`**

```go
package main

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

// Entry represents one line in a Claude Code session JSONL file.
type Entry struct {
	Type    string `json:"type"`
	Message struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			OutputTokens             int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
	Timestamp string `json:"timestamp"`
}

// parseJSONL reads a JSONL file and returns all parseable entries.
// Malformed lines are silently skipped.
func parseJSONL(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 2*1024*1024), 2*1024*1024) // 2MB per line
	for scanner.Scan() {
		var e Entry
		if json.Unmarshal(scanner.Bytes(), &e) == nil {
			entries = append(entries, e)
		}
	}
	return entries, scanner.Err()
}

// encodePath converts an absolute path to Claude Code's project directory encoding.
// e.g. /home/digitalghost -> home-digitalghost
func encodePath(absPath string) string {
	encoded := strings.ReplaceAll(absPath, "/", "-")
	return strings.TrimPrefix(encoded, "-")
}
```

- [ ] **Step 4: Run tests**

```bash
cd ~/projects/token-efficency && go test ./... -v -run TestParse
```

Expected: `PASS` for TestParseJSONL, TestEncodePathRoundTrip, TestParseJSONLSkipsMalformed.

- [ ] **Step 5: Commit**

```bash
git add jsonl.go jsonl_test.go
git commit -m "feat: add JSONL parser and path encoder"
```

---

## Task 3: Database layer (TDD)

**Files:**
- Create: `db_test.go`
- Create: `db.go`

- [ ] **Step 1: Write failing tests**

Create `db_test.go`:

```go
package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := initSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInitSchema(t *testing.T) {
	db := openTestDB(t)
	var name string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='sessions'").Scan(&name)
	if err != nil {
		t.Fatalf("sessions table not created: %v", err)
	}
	if name != "sessions" {
		t.Errorf("expected 'sessions', got %q", name)
	}
}

func TestUpsertAndQuery(t *testing.T) {
	db := openTestDB(t)

	row := SessionRow{
		SessionID:           "abc123",
		Project:             "nexus",
		StartedAt:           "2026-04-02T10:00:00Z",
		EndedAt:             "2026-04-02T10:30:00Z",
		Model:               "claude-sonnet-4-6",
		InputTokens:         1000,
		OutputTokens:        200,
		CacheCreationTokens: 500,
		CacheReadTokens:     300,
		SubagentCount:       2,
	}

	if err := upsertSession(db, row); err != nil {
		t.Fatalf("upsertSession: %v", err)
	}

	var got SessionRow
	err := db.QueryRow(`SELECT session_id, project, model, input_tokens, output_tokens,
		cache_creation_tokens, cache_read_tokens, subagent_count FROM sessions WHERE session_id = ?`,
		"abc123").Scan(&got.SessionID, &got.Project, &got.Model, &got.InputTokens,
		&got.OutputTokens, &got.CacheCreationTokens, &got.CacheReadTokens, &got.SubagentCount)
	if err != nil {
		t.Fatalf("query after upsert: %v", err)
	}

	if got.InputTokens != 1000 {
		t.Errorf("input_tokens: want 1000, got %d", got.InputTokens)
	}
	if got.SubagentCount != 2 {
		t.Errorf("subagent_count: want 2, got %d", got.SubagentCount)
	}
}

func TestUpsertIdempotent(t *testing.T) {
	db := openTestDB(t)

	row := SessionRow{
		SessionID: "dup1", Project: "p", StartedAt: "2026-04-02T10:00:00Z",
		EndedAt: "2026-04-02T10:05:00Z", Model: "claude-sonnet-4-6",
		InputTokens: 100, OutputTokens: 10,
	}
	if err := upsertSession(db, row); err != nil {
		t.Fatal(err)
	}

	// upsert again with updated token counts
	row.InputTokens = 200
	if err := upsertSession(db, row); err != nil {
		t.Fatal(err)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM sessions WHERE session_id = 'dup1'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 row after double upsert, got %d", count)
	}

	var input int
	db.QueryRow("SELECT input_tokens FROM sessions WHERE session_id = 'dup1'").Scan(&input)
	if input != 200 {
		t.Errorf("expected updated input_tokens=200, got %d", input)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd ~/projects/token-efficency && go test ./... 2>&1 | head -20
```

Expected: compile error — `SessionRow`, `initSchema`, `upsertSession` undefined.

- [ ] **Step 3: Create `db.go`**

```go
package main

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const relDBPath = ".local/share/token-tracker/usage.db"

// SessionRow is one row in the sessions table.
type SessionRow struct {
	SessionID           string
	Project             string
	StartedAt           string
	EndedAt             string
	Model               string
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
	SubagentCount       int
}

func openDB() (*sql.DB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, relDBPath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	return db, initSchema(db)
}

func initSchema(db *sql.DB) error {
	_, err := db.Exec(`
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
	`)
	return err
}

func upsertSession(db *sql.DB, s SessionRow) error {
	_, err := db.Exec(`
		INSERT INTO sessions
			(session_id, project, started_at, ended_at, model,
			 input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, subagent_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			ended_at             = excluded.ended_at,
			input_tokens         = excluded.input_tokens,
			output_tokens        = excluded.output_tokens,
			cache_creation_tokens = excluded.cache_creation_tokens,
			cache_read_tokens    = excluded.cache_read_tokens,
			subagent_count       = excluded.subagent_count`,
		s.SessionID, s.Project, s.StartedAt, s.EndedAt, s.Model,
		s.InputTokens, s.OutputTokens, s.CacheCreationTokens, s.CacheReadTokens, s.SubagentCount,
	)
	return err
}
```

- [ ] **Step 4: Run tests**

```bash
cd ~/projects/token-efficency && go test ./... -v -run TestInit
cd ~/projects/token-efficency && go test ./... -v -run TestUpsert
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add db.go db_test.go
git commit -m "feat: add SQLite schema and upsert"
```

---

## Task 4: Token aggregation (TDD)

**Files:**
- Create: `ingest_test.go`
- Create: `ingest.go` (aggregation only, no file I/O yet)

- [ ] **Step 1: Write failing tests**

Create `ingest_test.go`:

```go
package main

import (
	"testing"
)

func makeAssistantEntry(id, model, cwd, sessionID, ts string, input, cacheCreate, cacheRead, output int) Entry {
	var e Entry
	e.Type = "assistant"
	e.Message.ID = id
	e.Message.Model = model
	e.Message.Usage.InputTokens = input
	e.Message.Usage.CacheCreationInputTokens = cacheCreate
	e.Message.Usage.CacheReadInputTokens = cacheRead
	e.Message.Usage.OutputTokens = output
	e.CWD = cwd
	e.SessionID = sessionID
	e.Timestamp = ts
	return e
}

func TestAggregateTokens(t *testing.T) {
	entries := []Entry{
		// user message — should be ignored
		{Type: "user", SessionID: "s1", CWD: "/home/digitalghost", Timestamp: "2026-04-02T10:00:00.000Z"},
		// first assistant turn
		makeAssistantEntry("msg_001", "claude-sonnet-4-6", "/home/digitalghost", "s1", "2026-04-02T10:00:05.000Z", 100, 500, 200, 50),
		// duplicate of msg_001 (streaming artifact) — must not be double-counted
		makeAssistantEntry("msg_001", "claude-sonnet-4-6", "/home/digitalghost", "s1", "2026-04-02T10:00:05.100Z", 100, 500, 200, 50),
		// second assistant turn
		makeAssistantEntry("msg_002", "claude-sonnet-4-6", "/home/digitalghost", "s1", "2026-04-02T10:01:00.000Z", 20, 0, 300, 10),
	}

	got := aggregateTokens(entries)

	if got.SessionID != "s1" {
		t.Errorf("SessionID: want s1, got %s", got.SessionID)
	}
	if got.Project != "digitalghost" {
		t.Errorf("Project: want digitalghost, got %s", got.Project)
	}
	if got.Model != "claude-sonnet-4-6" {
		t.Errorf("Model: want claude-sonnet-4-6, got %s", got.Model)
	}
	// msg_001 (once) + msg_002: 100+20 = 120
	if got.InputTokens != 120 {
		t.Errorf("InputTokens: want 120, got %d", got.InputTokens)
	}
	// 500+0 = 500
	if got.CacheCreationTokens != 500 {
		t.Errorf("CacheCreationTokens: want 500, got %d", got.CacheCreationTokens)
	}
	// 200+300 = 500
	if got.CacheReadTokens != 500 {
		t.Errorf("CacheReadTokens: want 500, got %d", got.CacheReadTokens)
	}
	// 50+10 = 60
	if got.OutputTokens != 60 {
		t.Errorf("OutputTokens: want 60, got %d", got.OutputTokens)
	}
	if got.StartedAt != "2026-04-02T10:00:05.000Z" {
		t.Errorf("StartedAt: want first assistant ts, got %s", got.StartedAt)
	}
	if got.EndedAt != "2026-04-02T10:01:00.000Z" {
		t.Errorf("EndedAt: want last assistant ts, got %s", got.EndedAt)
	}
}

func TestAggregateTokensEmpty(t *testing.T) {
	got := aggregateTokens([]Entry{})
	if got.SessionID != "" {
		t.Errorf("expected empty SessionRow for empty entries")
	}
}

func TestAggregateTokensNoAssistant(t *testing.T) {
	entries := []Entry{
		{Type: "user", SessionID: "s2", CWD: "/home/digitalghost", Timestamp: "2026-04-02T10:00:00Z"},
	}
	got := aggregateTokens(entries)
	if got.InputTokens != 0 {
		t.Errorf("expected 0 tokens when no assistant entries")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd ~/projects/token-efficency && go test ./... 2>&1 | head -10
```

Expected: compile error — `aggregateTokens` undefined.

- [ ] **Step 3: Create `ingest.go` with aggregation only**

```go
package main

import (
	"path/filepath"
	"strings"
)

// aggregateTokens sums token usage across all assistant entries, deduplicating
// by message ID to handle Claude Code's streaming duplicate entries.
// Returns an empty SessionRow if there are no assistant entries.
func aggregateTokens(entries []Entry) SessionRow {
	seen := make(map[string]bool)
	var row SessionRow
	var firstAssistant, lastAssistant *Entry

	for i := range entries {
		e := &entries[i]
		if e.Type != "assistant" || e.Message.ID == "" {
			continue
		}
		if seen[e.Message.ID] {
			continue
		}
		seen[e.Message.ID] = true

		if firstAssistant == nil {
			firstAssistant = e
			row.SessionID = e.SessionID
			row.Model = e.Message.Model
			row.Project = filepath.Base(e.CWD)
		}
		lastAssistant = e

		row.InputTokens += e.Message.Usage.InputTokens
		row.CacheCreationTokens += e.Message.Usage.CacheCreationInputTokens
		row.CacheReadTokens += e.Message.Usage.CacheReadInputTokens
		row.OutputTokens += e.Message.Usage.OutputTokens
	}

	if firstAssistant != nil {
		row.StartedAt = firstAssistant.Timestamp
		row.EndedAt = lastAssistant.Timestamp
	}

	return row
}

// shortModel returns a short model name for display/Nexus notes.
// e.g. "claude-sonnet-4-6" -> "sonnet", "claude-haiku-4-5-20251001" -> "haiku"
func shortModel(model string) string {
	for _, name := range []string{"haiku", "sonnet", "opus"} {
		if strings.Contains(model, name) {
			return name
		}
	}
	return model
}
```

- [ ] **Step 4: Run tests**

```bash
cd ~/projects/token-efficency && go test ./... -v -run TestAggregate
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add ingest.go ingest_test.go
git commit -m "feat: add token aggregation with message ID deduplication"
```

---

## Task 5: File discovery and ingest command

**Files:**
- Modify: `ingest.go` — add `findLatestJSONL`, `findSubagentJSONLs`, `runIngest`

- [ ] **Step 1: Add file discovery and runIngest to `ingest.go`**

Append to `ingest.go`:

```go
import (
	// add to existing imports:
	"fmt"
	"os"
	"os/exec"
	"sort"
	"path/filepath"
	"strings"
	"time"
)
```

Replace the import block at the top of `ingest.go` with:

```go
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)
```

Then append these functions to `ingest.go`:

```go
// findLatestJSONL returns the most recently modified top-level JSONL file
// in the Claude project directory for the given absolute project path.
// It excludes subagent files (which are in subdirectories).
func findLatestJSONL(projectDir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	claudeProjectDir := filepath.Join(home, ".claude", "projects", encodePath(projectDir))

	entries, err := os.ReadDir(claudeProjectDir)
	if err != nil {
		return "", fmt.Errorf("claude project dir not found for %s: %w", projectDir, err)
	}

	type fileInfo struct {
		path    string
		modTime int64
	}
	var jsonls []fileInfo

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		jsonls = append(jsonls, fileInfo{
			path:    filepath.Join(claudeProjectDir, e.Name()),
			modTime: info.ModTime().UnixNano(),
		})
	}

	if len(jsonls) == 0 {
		return "", fmt.Errorf("no JSONL files found in %s", claudeProjectDir)
	}

	sort.Slice(jsonls, func(i, j int) bool {
		return jsonls[i].modTime > jsonls[j].modTime
	})
	return jsonls[0].path, nil
}

// findSubagentJSONLs returns all subagent JSONL files for the given session JSONL.
// Session JSONL is at <dir>/<session-id>.jsonl; subagents are at <dir>/<session-id>/subagents/*.jsonl
func findSubagentJSONLs(sessionJSONL string) ([]string, error) {
	sessionID := strings.TrimSuffix(filepath.Base(sessionJSONL), ".jsonl")
	subagentDir := filepath.Join(filepath.Dir(sessionJSONL), sessionID, "subagents")

	entries, err := os.ReadDir(subagentDir)
	if os.IsNotExist(err) {
		return nil, nil // no subagents is normal
	}
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			paths = append(paths, filepath.Join(subagentDir, e.Name()))
		}
	}
	return paths, nil
}

// runIngest discovers the latest session JSONL for projectDir, aggregates tokens,
// writes to SQLite, and fires a Nexus note.
func runIngest(projectDir string) error {
	sessionJSONL, err := findLatestJSONL(projectDir)
	if err != nil {
		return err
	}

	allEntries, err := parseJSONL(sessionJSONL)
	if err != nil {
		return fmt.Errorf("parse main JSONL: %w", err)
	}

	subagentFiles, err := findSubagentJSONLs(sessionJSONL)
	if err != nil {
		return err
	}

	subagentCount := len(subagentFiles)
	for _, sf := range subagentFiles {
		sub, err := parseJSONL(sf)
		if err != nil {
			continue // partial failure: skip bad subagent file
		}
		allEntries = append(allEntries, sub...)
	}

	row := aggregateTokens(allEntries)
	if row.SessionID == "" {
		return nil // no assistant turns; nothing to record
	}
	row.SubagentCount = subagentCount

	db, err := openDB()
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	if err := upsertSession(db, row); err != nil {
		return fmt.Errorf("upsert session: %w", err)
	}

	nexusNote(row)
	return nil
}

// nexusNote fires a nexus note summarizing the session. Failure is silent.
func nexusNote(row SessionRow) {
	var cachePct int
	total := row.InputTokens + row.CacheReadTokens
	if total > 0 {
		cachePct = row.CacheReadTokens * 100 / total
	}
	note := fmt.Sprintf("token-meter: %s | %s in / %s out | cache %d%% | %d subagents | %s",
		row.Project,
		formatNum(row.InputTokens),
		formatNum(row.OutputTokens),
		cachePct,
		row.SubagentCount,
		shortModel(row.Model),
	)
	exec.Command("nexus", "note", note).Run() //nolint:errcheck
}
```

- [ ] **Step 2: Add `formatNum` to `report.go`** (needed by nexusNote above — create a minimal `report.go` now)

Create `report.go` with just the helper (full report comes in Task 6):

```go
package main

import "fmt"

// formatNum formats an integer with comma separators.
func formatNum(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	result := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}
```

- [ ] **Step 3: Build to verify it compiles**

```bash
cd ~/projects/token-efficency && go build ./...
```

Expected: no errors.

- [ ] **Step 4: Run all tests**

```bash
cd ~/projects/token-efficency && go test ./...
```

Expected: PASS (existing tests unaffected).

- [ ] **Step 5: Commit**

```bash
git add ingest.go report.go
git commit -m "feat: add file discovery and ingest command"
```

---

## Task 6: Report queries (TDD)

**Files:**
- Modify: `db_test.go` — add query tests
- Modify: `db.go` — add query functions and result types

- [ ] **Step 1: Add query result types and tests to `db_test.go`**

Append to `db_test.go`:

```go
func seedSessions(t *testing.T, db *sql.DB) {
	t.Helper()
	rows := []SessionRow{
		{
			SessionID: "s1", Project: "nexus", Model: "claude-sonnet-4-6",
			StartedAt: "2026-04-02T08:00:00Z", EndedAt: "2026-04-02T08:30:00Z",
			InputTokens: 10000, OutputTokens: 1000, CacheCreationTokens: 2000, CacheReadTokens: 5000,
			SubagentCount: 2,
		},
		{
			SessionID: "s2", Project: "digitalghost", Model: "claude-haiku-4-5",
			StartedAt: "2026-04-02T09:00:00Z", EndedAt: "2026-04-02T09:15:00Z",
			InputTokens: 500, OutputTokens: 100, CacheCreationTokens: 0, CacheReadTokens: 8000,
			SubagentCount: 0,
		},
		{
			// older session — 8 days ago, outside 7-day window
			SessionID: "s3", Project: "nexus", Model: "claude-sonnet-4-6",
			StartedAt: "2026-03-25T10:00:00Z", EndedAt: "2026-03-25T10:10:00Z",
			InputTokens: 99999, OutputTokens: 9999, CacheCreationTokens: 0, CacheReadTokens: 0,
			SubagentCount: 0,
		},
	}
	for _, r := range rows {
		if err := upsertSession(db, r); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func TestQueryByProject(t *testing.T) {
	db := openTestDB(t)
	seedSessions(t, db)

	results, err := queryByProject(db, 7)
	if err != nil {
		t.Fatalf("queryByProject: %v", err)
	}
	// only s1 and s2 are within 7 days; s3 is excluded
	if len(results) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(results))
	}
	// nexus: 10000+5000 = 15000 total
	// digitalghost: 500+8000 = 8500 total
	// nexus should be first (higher total)
	if results[0].Project != "nexus" {
		t.Errorf("expected nexus first, got %s", results[0].Project)
	}
	if results[0].Total != 15000 {
		t.Errorf("nexus total: want 15000, got %d", results[0].Total)
	}
}

func TestQueryByModel(t *testing.T) {
	db := openTestDB(t)
	seedSessions(t, db)

	results, err := queryByModel(db, 7)
	if err != nil {
		t.Fatalf("queryByModel: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 models, got %d", len(results))
	}
}

func TestQueryTopSessions(t *testing.T) {
	db := openTestDB(t)
	seedSessions(t, db)

	results, err := queryTopSessions(db, 7)
	if err != nil {
		t.Fatalf("queryTopSessions: %v", err)
	}
	// s3 is outside 7-day window, so only s1 and s2
	if len(results) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(results))
	}
	// s1 has 15000 total, s2 has 8500 — s1 first
	if results[0].Project != "nexus" {
		t.Errorf("expected nexus first, got %s", results[0].Project)
	}
}
```

- [ ] **Step 2: Run to verify tests fail**

```bash
cd ~/projects/token-efficency && go test ./... -run TestQuery 2>&1 | head -10
```

Expected: compile error — `queryByProject`, `queryByModel`, `queryTopSessions` undefined.

- [ ] **Step 3: Add result types and query functions to `db.go`**

Append to `db.go`:

```go
// ProjectSummary aggregates token usage per project.
type ProjectSummary struct {
	Project  string
	Total    int
	CachePct int
}

// ModelSummary holds percentage share per model.
type ModelSummary struct {
	Model string
	Total int
}

// TopSession is one row in the top sessions list.
type TopSession struct {
	Day       string
	Project   string
	Total     int
	Subagents int
}

// DaySummary is the aggregated summary for today or a rolling window.
type DaySummary struct {
	Sessions  int
	Subagents int
	Input     int
	Output    int
	CacheRead int
	CachePct  int
}

func queryToday(db *sql.DB) (DaySummary, error) {
	var s DaySummary
	err := db.QueryRow(`
		SELECT
			COUNT(*),
			COALESCE(SUM(subagent_count), 0),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cache_read_tokens), 0),
			COALESCE(ROUND(100.0 * SUM(cache_read_tokens) /
				NULLIF(SUM(input_tokens) + SUM(cache_read_tokens), 0)), 0)
		FROM sessions
		WHERE date(started_at) = date('now', 'localtime')
	`).Scan(&s.Sessions, &s.Subagents, &s.Input, &s.Output, &s.CacheRead, &s.CachePct)
	return s, err
}

func queryRolling(db *sql.DB, days int) (DaySummary, error) {
	var s DaySummary
	err := db.QueryRow(`
		SELECT
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cache_read_tokens), 0),
			COALESCE(ROUND(100.0 * SUM(cache_read_tokens) /
				NULLIF(SUM(input_tokens) + SUM(cache_read_tokens), 0)), 0)
		FROM sessions
		WHERE started_at >= datetime('now', printf('-%d days', ?), 'localtime')
	`, days).Scan(&s.Input, &s.Output, &s.CacheRead, &s.CachePct)
	return s, err
}

func queryByProject(db *sql.DB, days int) ([]ProjectSummary, error) {
	rows, err := db.Query(`
		SELECT
			project,
			COALESCE(SUM(input_tokens + cache_read_tokens), 0) AS total,
			COALESCE(ROUND(100.0 * SUM(cache_read_tokens) /
				NULLIF(SUM(input_tokens) + SUM(cache_read_tokens), 0)), 0) AS cache_pct
		FROM sessions
		WHERE started_at >= datetime('now', printf('-%d days', ?), 'localtime')
		GROUP BY project
		ORDER BY total DESC
	`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ProjectSummary
	for rows.Next() {
		var p ProjectSummary
		if err := rows.Scan(&p.Project, &p.Total, &p.CachePct); err != nil {
			return nil, err
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

func queryByModel(db *sql.DB, days int) ([]ModelSummary, error) {
	rows, err := db.Query(`
		SELECT
			model,
			COALESCE(SUM(input_tokens + cache_read_tokens), 0) AS total
		FROM sessions
		WHERE started_at >= datetime('now', printf('-%d days', ?), 'localtime')
		GROUP BY model
		ORDER BY total DESC
	`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ModelSummary
	for rows.Next() {
		var m ModelSummary
		if err := rows.Scan(&m.Model, &m.Total); err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

func queryTopSessions(db *sql.DB, days int) ([]TopSession, error) {
	rows, err := db.Query(`
		SELECT
			date(started_at, 'localtime') AS day,
			project,
			input_tokens + cache_read_tokens AS total,
			subagent_count
		FROM sessions
		WHERE started_at >= datetime('now', printf('-%d days', ?), 'localtime')
		ORDER BY total DESC
		LIMIT 10
	`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TopSession
	for rows.Next() {
		var s TopSession
		if err := rows.Scan(&s.Day, &s.Project, &s.Total, &s.Subagents); err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, rows.Err()
}
```

- [ ] **Step 4: Run tests**

```bash
cd ~/projects/token-efficency && go test ./... -v -run TestQuery
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add db.go db_test.go
git commit -m "feat: add report query functions"
```

---

## Task 7: Report formatting and `runReport` (TDD)

**Files:**
- Create: `report_test.go`
- Modify: `report.go` — add `runReport`, `barChart`, `cacheEfficiency`

- [ ] **Step 1: Write failing tests**

Create `report_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestFormatNum(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{42310, "42,310"},
		{284100, "284,100"},
		{1234567, "1,234,567"},
	}
	for _, c := range cases {
		got := formatNum(c.n)
		if got != c.want {
			t.Errorf("formatNum(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestBarChart(t *testing.T) {
	// 50% of 100 total should yield 10 blocks (each block = 5%)
	got := barChart(50, 100)
	if !strings.Contains(got, "██████████") {
		t.Errorf("50%% bar should have 10 blocks, got: %q", got)
	}
	// 0% should yield empty bar
	got = barChart(0, 100)
	if strings.Contains(got, "█") {
		t.Errorf("0%% bar should be empty, got: %q", got)
	}
}

func TestShortModel(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"claude-sonnet-4-6", "sonnet"},
		{"claude-haiku-4-5-20251001", "haiku"},
		{"claude-opus-4-6", "opus"},
		{"unknown-model", "unknown-model"},
	}
	for _, c := range cases {
		got := shortModel(c.input)
		if got != c.want {
			t.Errorf("shortModel(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run to verify tests fail**

```bash
cd ~/projects/token-efficency && go test ./... -run TestBarChart 2>&1 | head -10
```

Expected: compile error — `barChart` undefined.

- [ ] **Step 3: Add `barChart` and `runReport` to `report.go`**

Replace the contents of `report.go` with:

```go
package main

import (
	"fmt"
	"strings"
	"time"
)

// formatNum formats an integer with comma separators.
func formatNum(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	result := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}

// barChart returns a bar of █ characters proportional to modelTotal/grandTotal.
// Each block represents 5 percentage points.
func barChart(modelTotal, grandTotal int) string {
	if grandTotal == 0 {
		return ""
	}
	pct := modelTotal * 100 / grandTotal
	blocks := pct / 5
	return strings.Repeat("█", blocks)
}

// runReport queries the DB and prints the token usage report.
func runReport(days int) error {
	db, err := openDB()
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	today, err := queryToday(db)
	if err != nil {
		return err
	}
	rolling, err := queryRolling(db, days)
	if err != nil {
		return err
	}
	projects, err := queryByProject(db, days)
	if err != nil {
		return err
	}
	models, err := queryByModel(db, days)
	if err != nil {
		return err
	}
	top, err := queryTopSessions(db, days)
	if err != nil {
		return err
	}

	// TODAY
	fmt.Printf("TODAY — %s\n", todayDate())
	fmt.Printf("  sessions: %d   subagents: %d   cache efficiency: %d%%\n\n",
		today.Sessions, today.Subagents, today.CachePct)
	fmt.Printf("  input: %s   output: %s   cache_read: %s\n\n",
		formatNum(today.Input), formatNum(today.Output), formatNum(today.CacheRead))

	// ROLLING
	fmt.Printf("%d-DAY ROLLING\n", days)
	fmt.Printf("  input: %s   output: %s   cache efficiency: %d%%\n\n",
		formatNum(rolling.Input), formatNum(rolling.Output), rolling.CachePct)

	// BY PROJECT
	if len(projects) > 0 {
		fmt.Printf("BY PROJECT (%dd)          tokens    cache%%\n", days)
		for _, p := range projects {
			fmt.Printf("  %-24s %8s    %3d%%\n", p.Project, formatNum(p.Total), p.CachePct)
		}
		fmt.Println()
	}

	// BY MODEL
	if len(models) > 0 {
		var grandTotal int
		for _, m := range models {
			grandTotal += m.Total
		}
		fmt.Printf("BY MODEL (%dd)\n", days)
		for _, m := range models {
			pct := 0
			if grandTotal > 0 {
				pct = m.Total * 100 / grandTotal
			}
			bar := barChart(m.Total, grandTotal)
			fmt.Printf("  %-8s %3d%%  %s\n", shortModel(m.Model), pct, bar)
		}
		fmt.Println()
	}

	// TOP SESSIONS
	if len(top) > 0 {
		fmt.Printf("TOP SESSIONS (%dd)\n", days)
		for _, s := range top {
			agents := ""
			if s.Subagents > 0 {
				agents = fmt.Sprintf(" (%d subagents)", s.Subagents)
			}
			fmt.Printf("  %s  %-24s %8s%s\n", s.Day, s.Project, formatNum(s.Total), agents)
		}
		fmt.Println()
	}

	return nil
}

// todayDate returns today's date in YYYY-MM-DD format.
func todayDate() string {
	return time.Now().Format("2006-01-02")
}
```

- [ ] **Step 4: Run all tests**

```bash
cd ~/projects/token-efficency && go test ./...
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add report.go report_test.go
git commit -m "feat: add report formatting and runReport"
```

---

## Task 8: Main entry point, build, and install

**Files:**
- Create: `main.go`

- [ ] **Step 1: Create `main.go`**

```go
package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "ingest":
		fs := flag.NewFlagSet("ingest", flag.ExitOnError)
		projectDir := fs.String("project-dir", "", "absolute path to the project directory (required)")
		fs.Parse(os.Args[2:])

		if *projectDir == "" {
			fmt.Fprintln(os.Stderr, "token-meter ingest: --project-dir is required")
			os.Exit(1)
		}
		if err := runIngest(*projectDir); err != nil {
			fmt.Fprintf(os.Stderr, "token-meter ingest: %v\n", err)
			os.Exit(0) // exit 0 to not disrupt Stop hook
		}

	case "report":
		fs := flag.NewFlagSet("report", flag.ExitOnError)
		days := fs.Int("days", 7, "number of days for rolling window")
		fs.Parse(os.Args[2:])

		if err := runReport(*days); err != nil {
			fmt.Fprintf(os.Stderr, "token-meter report: %v\n", err)
			os.Exit(1)
		}

	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `token-meter — Claude Code token usage tracker

Usage:
  token-meter ingest --project-dir <path>   record current session tokens
  token-meter report [--days N]             print usage report (default: 7 days)

`)
}

// override todayDate with real implementation
func init() {
	// patch todayDate to return the real date
	_ = time.Now // ensure time is imported
}
```

- [ ] **Step 2: Build**

```bash
cd ~/projects/token-efficency && go build -o /tmp/token-meter . && echo "build ok"
```

Expected: `build ok`.

- [ ] **Step 3: Smoke test ingest (dry run)**

```bash
/tmp/token-meter ingest --project-dir /home/digitalghost 2>&1
echo "exit: $?"
```

Expected: exits 0. May print an error about no JSONL files — that's fine, it means the discovery logic ran. If SQLite file is created, check it:

```bash
ls ~/.local/share/token-tracker/usage.db 2>/dev/null && echo "db exists" || echo "db not yet created"
```

- [ ] **Step 4: Run all tests one final time**

```bash
cd ~/projects/token-efficency && go test ./...
```

Expected: all PASS.

- [ ] **Step 5: Install**

```bash
go build -o ~/.local/bin/token-meter ~/projects/token-efficency/
echo "installed"
token-meter report
```

Expected: installs cleanly. `report` will show empty output or "no data yet" — that's correct before the Stop hook runs.

- [ ] **Step 6: Commit**

```bash
cd ~/projects/token-efficency
git add main.go report.go
git commit -m "feat: wire up main entry point and install token-meter"
```

---

## Task 9: Stop hook wiring

**Files:**
- Modify: `~/.claude/settings.json`

- [x] **Step 1: Read current settings**

```bash
cat ~/.claude/settings.json
```

Verify the current `hooks` section structure before editing.

- [x] **Step 2: Add Stop hook**

Edit `~/.claude/settings.json`. Add a `"Stop"` key to the `"hooks"` object alongside the existing `"PreToolUse"` and `"SessionStart"` keys:

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

The final `hooks` section should look like:

```json
"hooks": {
  "PreToolUse": [ ... existing ... ],
  "SessionStart": [ ... existing ... ],
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
}
```

- [x] **Step 3: Validate JSON**

```bash
python3 -m json.tool ~/.claude/settings.json > /dev/null && echo "valid JSON"
```

Expected: `valid JSON`.

- [x] **Step 4: Commit**

```bash
cd ~/projects/token-efficency
git add -A
git commit -m "feat: wire Stop hook to record token usage after each session"
```

- [ ] **Step 5: Verify hook fires**

End this session and start a new one. After the new session ends, run:

```bash
token-meter report
```

Expected: data appears for the current project. If nothing appears, check:

```bash
sqlite3 ~/.local/share/token-tracker/usage.db "SELECT * FROM sessions ORDER BY ended_at DESC LIMIT 5;"
```

---

## Post-Build Checklist

- [ ] `go test ./...` passes clean
- [ ] `token-meter report` prints output without errors
- [ ] Stop hook fires silently after session end (check `usage.db` has new rows)
- [ ] Nexus notes contain token summaries (run `nexus report` to verify)
- [ ] Model breakdown shows haiku when routing fired (verify over a few sessions)
