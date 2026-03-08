package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	tea "charm.land/bubbletea/v2"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/login"
	"github.com/fugue-labs/golem/internal/ui"
)

func main() {
	errOut := os.Stderr

	// Handle subcommands before starting TUI.
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "login":
			var provider string
			if len(os.Args) >= 3 {
				provider = os.Args[2]
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			if err := login.Run(ctx, provider); err != nil {
				fmt.Fprintf(errOut, "login error: %v\n", err)
				os.Exit(1)
			}
			return
		case "logout":
			if err := login.Logout(); err != nil {
				fmt.Fprintf(errOut, "logout error: %v\n", err)
				os.Exit(1)
			}
			return
		case "status":
			fmt.Println(config.Status())
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
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
		fmt.Fprintf(errOut, "error: %v\n", err)
		os.Exit(1)
	}
}
