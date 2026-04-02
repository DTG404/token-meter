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
