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
