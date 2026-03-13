package automations

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Runner executes golem sessions in response to automation events.
type Runner struct {
	mu      sync.Mutex
	runs    []RunRecord
	maxRuns int // max concurrent runs (0 = unlimited)
	active  int
}

// RunRecord tracks a single automation execution.
type RunRecord struct {
	AutomationName string    `json:"automation_name"`
	EventType      string    `json:"event_type"`
	StartedAt      time.Time `json:"started_at"`
	FinishedAt     time.Time `json:"finished_at,omitempty"`
	ExitCode       int       `json:"exit_code"`
	Error          string    `json:"error,omitempty"`
	LogFile        string    `json:"log_file,omitempty"`
}

// NewRunner creates a runner with the given concurrency limit (0 = unlimited).
func NewRunner(maxConcurrent int) *Runner {
	return &Runner{maxRuns: maxConcurrent}
}

// Run executes a golem session for the given event and workflow configuration.
// The session runs as a subprocess with the event context injected as the prompt.
func (r *Runner) Run(ctx context.Context, event Event, workflow Workflow, automationsCfg map[string]Automation) {
	r.mu.Lock()
	if r.maxRuns > 0 && r.active >= r.maxRuns {
		r.mu.Unlock()
		log.Printf("[automations] skipping %q: max concurrent runs (%d) reached", event.Name, r.maxRuns)
		return
	}
	r.active++
	r.mu.Unlock()

	go func() {
		defer func() {
			r.mu.Lock()
			r.active--
			r.mu.Unlock()
		}()

		record := RunRecord{
			AutomationName: event.Name,
			EventType:      event.Type,
			StartedAt:      time.Now(),
		}

		if err := r.execute(ctx, event, workflow); err != nil {
			record.Error = err.Error()
			record.ExitCode = 1
			log.Printf("[automations] %q failed: %v", event.Name, err)
		} else {
			log.Printf("[automations] %q completed successfully", event.Name)
		}

		record.FinishedAt = time.Now()
		r.mu.Lock()
		r.runs = append(r.runs, record)
		// Keep only last 100 records.
		if len(r.runs) > 100 {
			r.runs = r.runs[len(r.runs)-100:]
		}
		r.mu.Unlock()
	}()
}

// History returns recent run records.
func (r *Runner) History() []RunRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]RunRecord, len(r.runs))
	copy(result, r.runs)
	return result
}

// ActiveCount returns the number of currently running automation sessions.
func (r *Runner) ActiveCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active
}

func (r *Runner) execute(ctx context.Context, event Event, workflow Workflow) error {
	// Expand template placeholders in prompt and working dir.
	prompt := event.ExpandTemplate(workflow.Prompt)
	workDir := event.ExpandTemplate(workflow.WorkingDir)
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	// Build the event context summary for the prompt.
	var contextBuilder strings.Builder
	contextBuilder.WriteString(prompt)
	contextBuilder.WriteString("\n\n---\n")
	contextBuilder.WriteString(fmt.Sprintf("Automation: %s\n", event.Name))
	contextBuilder.WriteString(fmt.Sprintf("Trigger: %s\n", event.Type))
	contextBuilder.WriteString(fmt.Sprintf("Time: %s\n", event.Timestamp.Format(time.RFC3339)))

	if event.Type == "github_webhook" {
		contextBuilder.WriteString("\nEvent payload (summary):\n")
		if summary := summarizeGitHubEvent(event.Properties); summary != "" {
			contextBuilder.WriteString(summary)
		}
	}

	fullPrompt := contextBuilder.String()

	// Create a log file for this run.
	logDir := automationsLogDir()
	logFile := filepath.Join(logDir, fmt.Sprintf("%s-%s.log", event.Name, event.Timestamp.Format("20060102-150405")))
	_ = os.MkdirAll(logDir, 0o755)

	// Parse timeout.
	timeout := 30 * time.Minute
	if workflow.Timeout != "" {
		if d, err := time.ParseDuration(workflow.Timeout); err == nil {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Spawn golem as a subprocess.
	golemBin, err := os.Executable()
	if err != nil {
		golemBin = "golem"
	}

	cmd := exec.CommandContext(ctx, golemBin, fullPrompt)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"GOLEM_PERMISSION_MODE=auto",
		fmt.Sprintf("GOLEM_AUTOMATION_EVENT=%s", event.Type),
		fmt.Sprintf("GOLEM_AUTOMATION_NAME=%s", event.Name),
	)

	// Write output to log file.
	lf, err := os.Create(logFile)
	if err == nil {
		cmd.Stdout = lf
		cmd.Stderr = lf
		defer lf.Close()
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("golem session exited: %w (log: %s)", err, logFile)
	}
	return nil
}

func automationsLogDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".golem", "automations", "logs")
}

// summarizeGitHubEvent extracts key information from a GitHub webhook payload
// for injection into the golem prompt context.
func summarizeGitHubEvent(props map[string]any) string {
	var b strings.Builder

	if action, ok := props["action"].(string); ok {
		fmt.Fprintf(&b, "Action: %s\n", action)
	}

	if repo, ok := props["repository"].(map[string]any); ok {
		if name, ok := repo["full_name"].(string); ok {
			fmt.Fprintf(&b, "Repository: %s\n", name)
		}
	}

	if pr, ok := props["pull_request"].(map[string]any); ok {
		if title, ok := pr["title"].(string); ok {
			fmt.Fprintf(&b, "PR Title: %s\n", title)
		}
		if num, ok := pr["number"].(float64); ok {
			fmt.Fprintf(&b, "PR Number: #%d\n", int(num))
		}
		if url, ok := pr["html_url"].(string); ok {
			fmt.Fprintf(&b, "PR URL: %s\n", url)
		}
		if head, ok := pr["head"].(map[string]any); ok {
			if ref, ok := head["ref"].(string); ok {
				fmt.Fprintf(&b, "Branch: %s\n", ref)
			}
		}
	}

	if sender, ok := props["sender"].(map[string]any); ok {
		if login, ok := sender["login"].(string); ok {
			fmt.Fprintf(&b, "Sender: %s\n", login)
		}
	}

	// For push events.
	if ref, ok := props["ref"].(string); ok {
		fmt.Fprintf(&b, "Ref: %s\n", ref)
	}
	if commits, ok := props["commits"].([]any); ok {
		fmt.Fprintf(&b, "Commits: %d\n", len(commits))
	}

	return b.String()
}

// ExportEventJSON writes the event payload to a temp file and returns the path.
// Useful for passing event data to golem sessions via environment.
func ExportEventJSON(event Event) (string, error) {
	data, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return "", err
	}
	tmpFile, err := os.CreateTemp("", "golem-event-*.json")
	if err != nil {
		return "", err
	}
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", err
	}
	tmpFile.Close()
	return tmpFile.Name(), nil
}
