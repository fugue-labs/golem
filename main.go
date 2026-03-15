package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/automations"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/login"
	"github.com/fugue-labs/golem/internal/ui"
	"github.com/fugue-labs/golem/internal/ui/dashboard"
)

var (
	loadConfigFunc     = config.Load
	prepareRuntimeFunc = agent.PrepareRuntime
	loginRunFunc       = login.Run
	logoutFunc         = login.Logout
	runDashboardFunc   = runDashboard
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, out, errOut io.Writer) int {
	if len(args) >= 1 {
		switch args[0] {
		case "login":
			var provider string
			if len(args) >= 2 {
				provider = args[1]
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			if err := loginRunFunc(ctx, provider); err != nil {
				fmt.Fprintf(errOut, "login error: %v\n", err)
				return 1
			}
			return 0
		case "logout":
			if err := logoutFunc(); err != nil {
				fmt.Fprintf(errOut, "logout error: %v\n", err)
				return 1
			}
			return 0
		case "dashboard":
			var missionID string
			if len(args) >= 2 {
				missionID = args[1]
			}
			return runDashboardFunc(missionID, errOut)
		case "status", "runtime":
			return runRuntimeCommand(args[0], args[1:], out, errOut)
		case "automations":
			return runAutomationsCommand(args[1:], out, errOut)
		}
	}

	cfg, err := loadConfigFunc()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}

	validation := cfg.Validate()
	if validation.HasErrors() {
		fmt.Fprintln(errOut, agent.RenderStatusSummary(agent.BuildRuntimeReport(cfg, agent.InitialRuntimeState(cfg), validation, nil)))
		return 1
	}
	if len(validation.Warnings) > 0 {
		fmt.Fprintln(errOut, agent.RenderStatusSummary(agent.BuildRuntimeReport(cfg, agent.InitialRuntimeState(cfg), validation, nil)))
	}

	// Redirect stderr to a log file so codetool's middleware logging
	// doesn't corrupt the TUI. Logs go to /tmp/golem.log for debugging.
	logFile, err := os.OpenFile("/tmp/golem.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err == nil {
		os.Stderr = logFile
		defer logFile.Close()
	}

	m := ui.New(cfg)

	// If remaining args after subcommand check, join as initial prompt.
	// e.g. `golem fix the failing tests` → prompt="fix the failing tests"
	if len(args) > 0 {
		prompt := strings.Join(args, " ")
		m.SetInitialPrompt(prompt)
	}

	p := tea.NewProgram(m)
	m.SetProgram(p)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}
	return 0
}

func runRuntimeCommand(name string, args []string, out, errOut io.Writer) int {
	jsonOutput, ok := parseJSONFlag(args)
	if !ok {
		switch name {
		case "runtime":
			fmt.Fprintln(errOut, "usage: golem runtime [--json]")
		default:
			fmt.Fprintln(errOut, "usage: golem status [--json]")
		}
		return 1
	}

	cfg, err := loadConfigFunc()
	if err != nil {
		if jsonOutput {
			_ = json.NewEncoder(out).Encode(map[string]string{"error": err.Error()})
		} else {
			fmt.Fprintf(errOut, "config error: %v\n", err)
		}
		return 1
	}

	validation := cfg.Validate()
	runtime, runtimeErr := prepareRuntimeFunc(context.Background(), cfg, "")
	if runtimeErr != nil {
		runtime = agent.InitialRuntimeState(cfg)
	}
	report := agent.BuildRuntimeReport(cfg, runtime, validation, runtimeErr)

	if jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			switch name {
			case "runtime":
				fmt.Fprintf(errOut, "encoding runtime output: %v\n", err)
			default:
				fmt.Fprintf(errOut, "encoding status output: %v\n", err)
			}
			return 1
		}
	} else {
		switch name {
		case "status":
			fmt.Fprintln(out, agent.RenderStatusSummary(report))
		case "runtime":
			fmt.Fprintln(out, agent.RenderRuntimeSummary(report))
		}
	}

	if validation.HasErrors() || runtimeErr != nil {
		return 1
	}
	return 0
}

func runDashboard(missionID string, errOut io.Writer) int {
	m := dashboard.New(missionID)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(errOut, "dashboard error: %v\n", err)
		return 1
	}
	return 0
}

func parseJSONFlag(args []string) (bool, bool) {
	if len(args) == 0 {
		return false, true
	}
	if len(args) == 1 && args[0] == "--json" {
		return true, true
	}
	return false, false
}

func runAutomationsCommand(args []string, out, errOut io.Writer) int {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}

	switch sub {
	case "list":
		cfg, err := automations.LoadConfig()
		if err != nil {
			fmt.Fprintf(errOut, "error loading automations config: %v\n", err)
			return 1
		}
		fmt.Fprintln(out, automations.ListAutomations(cfg))
		return 0

	case "start":
		cfg, err := automations.LoadConfig()
		if err != nil {
			fmt.Fprintf(errOut, "error loading automations config: %v\n", err)
			return 1
		}
		if cfg == nil {
			fmt.Fprintln(errOut, "no automations configured")
			fmt.Fprintln(errOut, "\nCreate ~/.golem/automations.json to get started.")
			fmt.Fprintln(errOut, "Run `golem automations init` for an example configuration.")
			return 1
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()

		daemon := automations.NewDaemon(cfg)
		if err := daemon.Start(ctx); err != nil {
			fmt.Fprintf(errOut, "automations daemon error: %v\n", err)
			return 1
		}
		return 0

	case "status":
		fmt.Fprintln(out, automations.StatusSummary())
		return 0

	case "init":
		fmt.Fprintln(out, "Example ~/.golem/automations.json:")
		fmt.Fprintln(out, automations.ExampleConfig())
		return 0

	default:
		fmt.Fprintf(errOut, "unknown automations subcommand: %s\n", sub)
		fmt.Fprintln(errOut, "usage: golem automations [list|start|status|init]")
		return 1
	}
}
