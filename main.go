package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/ui"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	// Redirect stderr to a log file so codetool's middleware logging
	// doesn't corrupt the TUI. Logs go to /tmp/golem.log for debugging.
	logFile, err := os.OpenFile("/tmp/golem.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err == nil {
		os.Stderr = logFile
		defer logFile.Close()
	}

	m := ui.New(cfg)
	p := tea.NewProgram(m)
	m.SetProgram(p)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
