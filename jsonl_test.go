package main

import (
	"os"
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
	f, err := os.CreateTemp(t.TempDir(), "*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
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
