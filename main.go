package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "ingest":
		fs := flag.NewFlagSet("ingest", flag.ExitOnError)
		projectDir := fs.String("project-dir", "", "absolute path to the project directory (required)")
		fs.Parse(os.Args[2:])

		if *projectDir == "" {
			fmt.Fprintln(os.Stderr, "token-meter ingest: --project-dir is required")
			os.Exit(1)
		}
		if err := runIngest(*projectDir); err != nil {
			fmt.Fprintf(os.Stderr, "token-meter ingest: %v\n", err)
			os.Exit(0) // exit 0 to not disrupt Stop hook
		}

	case "report":
		fs := flag.NewFlagSet("report", flag.ExitOnError)
		days := fs.Int("days", 7, "number of days for rolling window")
		fs.Parse(os.Args[2:])

		if err := runReport(*days); err != nil {
			fmt.Fprintf(os.Stderr, "token-meter report: %v\n", err)
			os.Exit(1)
		}

	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `token-meter — Claude Code token usage tracker

Usage:
  token-meter ingest --project-dir <path>   record current session tokens
  token-meter report [--days N]             print usage report (default: 7 days)

`)
}

