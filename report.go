package main

import (
	"fmt"
	"strings"
	"time"
)

// formatNum formats an integer with comma separators.
func formatNum(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	result := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}

// barChart returns a bar of █ characters proportional to modelTotal/grandTotal.
// Each block represents 5 percentage points.
func barChart(modelTotal, grandTotal int) string {
	if grandTotal == 0 {
		return ""
	}
	pct := modelTotal * 100 / grandTotal
	blocks := pct / 5
	return strings.Repeat("█", blocks)
}

// runReport queries the DB and prints the token usage report.
func runReport(days int) error {
	db, err := openDB()
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	today, err := queryToday(db)
	if err != nil {
		return err
	}
	rolling, err := queryRolling(db, days)
	if err != nil {
		return err
	}
	projects, err := queryByProject(db, days)
	if err != nil {
		return err
	}
	models, err := queryByModel(db, days)
	if err != nil {
		return err
	}
	top, err := queryTopSessions(db, days)
	if err != nil {
		return err
	}

	// TODAY
	fmt.Printf("TODAY — %s\n", todayDate())
	fmt.Printf("  sessions: %d   subagents: %d   cache efficiency: %d%%\n\n",
		today.Sessions, today.Subagents, today.CachePct)
	fmt.Printf("  input: %s   output: %s   cache_read: %s\n\n",
		formatNum(today.Input), formatNum(today.Output), formatNum(today.CacheRead))

	// ROLLING
	fmt.Printf("%d-DAY ROLLING\n", days)
	fmt.Printf("  input: %s   output: %s   cache efficiency: %d%%\n\n",
		formatNum(rolling.Input), formatNum(rolling.Output), rolling.CachePct)

	// BY PROJECT
	if len(projects) > 0 {
		fmt.Printf("BY PROJECT (%dd)          tokens    cache%%\n", days)
		for _, p := range projects {
			fmt.Printf("  %-24s %8s    %3d%%\n", p.Project, formatNum(p.Total), p.CachePct)
		}
		fmt.Println()
	}

	// BY MODEL
	if len(models) > 0 {
		var grandTotal int
		for _, m := range models {
			grandTotal += m.Total
		}
		fmt.Printf("BY MODEL (%dd)\n", days)
		for _, m := range models {
			pct := 0
			if grandTotal > 0 {
				pct = m.Total * 100 / grandTotal
			}
			bar := barChart(m.Total, grandTotal)
			fmt.Printf("  %-8s %3d%%  %s\n", shortModel(m.Model), pct, bar)
		}
		fmt.Println()
	}

	// TOP SESSIONS
	if len(top) > 0 {
		fmt.Printf("TOP SESSIONS (%dd)\n", days)
		for _, s := range top {
			agents := ""
			if s.Subagents > 0 {
				agents = fmt.Sprintf(" (%d subagents)", s.Subagents)
			}
			fmt.Printf("  %s  %-24s %8s%s\n", s.Day, s.Project, formatNum(s.Total), agents)
		}
		fmt.Println()
	}

	return nil
}

// todayDate returns today's date in YYYY-MM-DD format.
func todayDate() string {
	return time.Now().Format("2006-01-02")
}
