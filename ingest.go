package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
