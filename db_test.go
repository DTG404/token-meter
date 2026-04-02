package main

import (
	"database/sql"
	"path/filepath"
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

func TestOpenDBAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := openDBAt(path)
	if err != nil {
		t.Fatalf("openDBAt: %v", err)
	}
	defer db.Close()
	// verify schema was initialized
	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='sessions'").Scan(&name)
	if err != nil {
		t.Fatalf("sessions table not found: %v", err)
	}
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
