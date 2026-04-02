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
