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
	return openDBAt(path)
}

func openDBAt(path string) (*sql.DB, error) {
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
