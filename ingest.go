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
