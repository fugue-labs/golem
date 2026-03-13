package automations

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
)

// Daemon orchestrates all automation triggers and routes events to the runner.
type Daemon struct {
	cfg    *Config
	runner *Runner
}

// NewDaemon creates an automations daemon from the given configuration.
func NewDaemon(cfg *Config) *Daemon {
	return &Daemon{
		cfg:    cfg,
		runner: NewRunner(5), // max 5 concurrent automation sessions
	}
}

// Start begins all configured triggers and blocks until ctx is cancelled.
func (d *Daemon) Start(ctx context.Context) error {
	if d.cfg == nil || len(d.cfg.Automations) == 0 {
		return fmt.Errorf("no automations configured")
	}

	if err := d.cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Build a lookup map for automation workflows.
	automationMap := make(map[string]Automation)
	for _, a := range d.cfg.Automations {
		automationMap[a.Name] = a
	}

	handler := func(event Event) {
		a, ok := automationMap[event.Name]
		if !ok {
			log.Printf("[automations] unknown automation %q in event", event.Name)
			return
		}
		log.Printf("[automations] trigger fired: %s (%s)", event.Name, event.Type)
		d.runner.Run(ctx, event, a.Workflow, automationMap)
	}

	// Start cron triggers.
	cronCount := 0
	for _, a := range d.cfg.Automations {
		if !a.IsEnabled() || a.Trigger.Type != "cron" {
			continue
		}
		schedule, err := ParseCronSchedule(a.Trigger.Schedule)
		if err != nil {
			return fmt.Errorf("automation %q: invalid cron schedule: %w", a.Name, err)
		}
		trigger := CronTrigger{
			Name:     a.Name,
			Schedule: schedule,
			Workflow: a.Workflow,
		}
		go RunCronTrigger(ctx, trigger, handler)
		cronCount++
		log.Printf("[automations] cron trigger started: %s (%s)", a.Name, a.Trigger.Schedule)
	}

	// Start webhook server (handles all github_webhook automations).
	webhookServer := NewWebhookServer(d.cfg, handler)
	hasWebhooks := false
	for _, a := range d.cfg.Automations {
		if a.IsEnabled() && a.Trigger.Type == "github_webhook" {
			hasWebhooks = true
			break
		}
	}

	log.Printf("[automations] daemon started: %d cron triggers, webhooks=%v", cronCount, hasWebhooks)

	// Write PID file for status checks.
	_ = writePIDFile()

	if hasWebhooks {
		return webhookServer.Start(ctx)
	}

	// No webhooks — just wait for cancellation (cron triggers run in goroutines).
	<-ctx.Done()
	_ = removePIDFile()
	return nil
}

// ListAutomations prints a formatted list of configured automations to stdout.
func ListAutomations(cfg *Config) string {
	if cfg == nil || len(cfg.Automations) == 0 {
		return "No automations configured.\n\nCreate ~/.golem/automations.json to get started."
	}

	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tENABLED\tDETAILS")
	for _, a := range cfg.Automations {
		enabled := "yes"
		if !a.IsEnabled() {
			enabled = "no"
		}

		var details string
		switch a.Trigger.Type {
		case "cron":
			details = a.Trigger.Schedule
		case "github_webhook":
			details = strings.Join(a.Trigger.Events, ", ")
			if len(a.Trigger.Repos) > 0 {
				details += " [" + strings.Join(a.Trigger.Repos, ", ") + "]"
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", a.Name, a.Trigger.Type, enabled, details)
	}
	w.Flush()
	return b.String()
}

// StatusSummary returns the current daemon status.
func StatusSummary() string {
	pid, running := readPIDFile()
	if !running {
		return "Automations daemon: not running"
	}
	return fmt.Sprintf("Automations daemon: running (PID %d)", pid)
}

func pidFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".golem", "automations", "daemon.pid")
}

func writePIDFile() error {
	path := pidFilePath()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	return os.WriteFile(path, []byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
}

func removePIDFile() error {
	return os.Remove(pidFilePath())
}

func readPIDFile() (int, bool) {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return 0, false
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return 0, false
	}
	// Check if process is still running.
	proc, err := os.FindProcess(pid)
	if err != nil {
		return 0, false
	}
	// On Unix, FindProcess always succeeds. Signal 0 checks existence.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return 0, false
	}
	return pid, true
}

// ExampleConfig returns a documented example automations.json for new users.
func ExampleConfig() string {
	example := Config{
		Automations: []Automation{
			{
				Name: "pr-review",
				Trigger: Trigger{
					Type:   "github_webhook",
					Events: []string{"pull_request.opened", "pull_request.synchronize"},
					Repos:  []string{"your-org/your-repo"},
				},
				Workflow: Workflow{
					Prompt:     "Review the changes in this pull request: {{event.pull_request.html_url}}",
					WorkingDir: "/path/to/your/repo",
					Timeout:    "30m",
				},
			},
			{
				Name: "daily-check",
				Trigger: Trigger{
					Type:     "cron",
					Schedule: "0 9 * * 1-5",
				},
				Workflow: Workflow{
					Prompt:     "Run the test suite and report any failures",
					WorkingDir: "/path/to/your/repo",
					Timeout:    "15m",
				},
			},
		},
		Server: ServerConfig{
			Port:          7654,
			WebhookSecret: "$GOLEM_WEBHOOK_SECRET",
		},
	}

	data, _ := json.MarshalIndent(example, "", "  ")
	return string(data)
}
