package automations

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is the top-level automations configuration loaded from
// ~/.golem/automations.json.
type Config struct {
	Automations []Automation `json:"automations"`
	Server      ServerConfig `json:"server"`
}

// ServerConfig holds settings for the webhook HTTP server.
type ServerConfig struct {
	Port          int    `json:"port"`
	WebhookSecret string `json:"webhook_secret"` // supports $ENV_VAR expansion
}

// Automation defines a single trigger-to-workflow binding.
type Automation struct {
	Name     string   `json:"name"`
	Trigger  Trigger  `json:"trigger"`
	Workflow Workflow `json:"workflow"`
	Enabled  *bool    `json:"enabled,omitempty"` // defaults to true if nil
}

// IsEnabled returns whether this automation is active.
func (a Automation) IsEnabled() bool {
	return a.Enabled == nil || *a.Enabled
}

// Trigger defines what event starts an automation.
type Trigger struct {
	Type string `json:"type"` // "github_webhook", "cron", "file_watch"

	// GitHub webhook fields.
	Events []string `json:"events,omitempty"` // e.g. "pull_request.opened"
	Repos  []string `json:"repos,omitempty"`  // e.g. "fugue-labs/golem"

	// Cron fields.
	Schedule string `json:"schedule,omitempty"` // cron expression: "0 9 * * *"
}

// Workflow defines what happens when a trigger fires.
type Workflow struct {
	Prompt     string `json:"prompt"`                // prompt template with {{event.*}} placeholders
	WorkingDir string `json:"working_dir,omitempty"` // working directory (supports templates)
	Timeout    string `json:"timeout,omitempty"`      // e.g. "30m", defaults to config.Timeout
}

// LoadConfig reads the automations configuration from ~/.golem/automations.json.
// Returns nil config (no error) if the file doesn't exist.
func LoadConfig() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}
	return LoadConfigFrom(filepath.Join(home, ".golem", "automations.json"))
}

// LoadConfigFrom reads the automations configuration from the given path.
func LoadConfigFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	// Expand environment variable references in sensitive fields.
	cfg.Server.WebhookSecret = expandEnv(cfg.Server.WebhookSecret)

	// Apply defaults.
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 7654
	}

	return &cfg, nil
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}

	names := make(map[string]bool)
	for i, a := range c.Automations {
		if a.Name == "" {
			return fmt.Errorf("automation[%d]: name is required", i)
		}
		if names[a.Name] {
			return fmt.Errorf("automation[%d]: duplicate name %q", i, a.Name)
		}
		names[a.Name] = true

		switch a.Trigger.Type {
		case "github_webhook":
			if len(a.Trigger.Events) == 0 {
				return fmt.Errorf("automation %q: github_webhook trigger requires at least one event", a.Name)
			}
		case "cron":
			if a.Trigger.Schedule == "" {
				return fmt.Errorf("automation %q: cron trigger requires a schedule", a.Name)
			}
		default:
			return fmt.Errorf("automation %q: unsupported trigger type %q", a.Name, a.Trigger.Type)
		}

		if strings.TrimSpace(a.Workflow.Prompt) == "" {
			return fmt.Errorf("automation %q: workflow prompt is required", a.Name)
		}
	}

	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535, got %d", c.Server.Port)
	}

	return nil
}

// expandEnv expands $VAR and ${VAR} references in a string using environment variables.
func expandEnv(s string) string {
	if s == "" {
		return s
	}
	return os.ExpandEnv(s)
}
