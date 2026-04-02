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
	if got != strings.Repeat("█", 10) {
		t.Errorf("barChart(50, 100): want 10 blocks, got: %q", got)
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
