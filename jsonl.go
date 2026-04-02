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
// Note: this encoding is lossy — hyphens in directory names are
// indistinguishable from path separators (e.g. /a/b-c and /a-b/c both
// encode to "a-b-c"). This matches Claude Code's own encoding scheme.
func encodePath(absPath string) string {
	encoded := strings.ReplaceAll(absPath, "/", "-")
	return strings.TrimPrefix(encoded, "-")
}
