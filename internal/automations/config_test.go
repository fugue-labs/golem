package automations

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFrom(t *testing.T) {
	t.Run("missing file returns nil", func(t *testing.T) {
		cfg, err := LoadConfigFrom("/nonexistent/path.json")
		if err != nil {
			t.Fatalf("expected nil error for missing file, got %v", err)
		}
		if cfg != nil {
			t.Fatalf("expected nil config for missing file, got %+v", cfg)
		}
	})

	t.Run("valid config", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "automations.json")
		data := `{
			"automations": [
				{
					"name": "test-cron",
					"trigger": {"type": "cron", "schedule": "0 9 * * *"},
					"workflow": {"prompt": "run tests"}
				},
				{
					"name": "test-webhook",
					"trigger": {"type": "github_webhook", "events": ["push"]},
					"workflow": {"prompt": "check push"}
				}
			],
			"server": {"port": 8080}
		}`
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}

		cfg, err := LoadConfigFrom(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg == nil {
			t.Fatal("expected non-nil config")
		}
		if len(cfg.Automations) != 2 {
			t.Fatalf("expected 2 automations, got %d", len(cfg.Automations))
		}
		if cfg.Server.Port != 8080 {
			t.Fatalf("expected port 8080, got %d", cfg.Server.Port)
		}
	})

	t.Run("default port", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "automations.json")
		data := `{"automations": []}`
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}

		cfg, err := LoadConfigFrom(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Server.Port != 7654 {
			t.Fatalf("expected default port 7654, got %d", cfg.Server.Port)
		}
	})

	t.Run("env expansion in webhook secret", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "automations.json")
		data := `{"automations": [], "server": {"webhook_secret": "$TEST_GOLEM_SECRET"}}`
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}

		t.Setenv("TEST_GOLEM_SECRET", "mysecret123")
		cfg, err := LoadConfigFrom(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Server.WebhookSecret != "mysecret123" {
			t.Fatalf("expected expanded secret, got %q", cfg.Server.WebhookSecret)
		}
	})
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name:    "empty config with valid port",
			cfg:     Config{Server: ServerConfig{Port: 7654}},
			wantErr: "", // empty automations is valid
		},
		{
			name: "missing name",
			cfg: Config{
				Automations: []Automation{{Trigger: Trigger{Type: "cron", Schedule: "* * * * *"}, Workflow: Workflow{Prompt: "test"}}},
				Server:      ServerConfig{Port: 7654},
			},
			wantErr: "name is required",
		},
		{
			name: "duplicate names",
			cfg: Config{
				Automations: []Automation{
					{Name: "dup", Trigger: Trigger{Type: "cron", Schedule: "* * * * *"}, Workflow: Workflow{Prompt: "a"}},
					{Name: "dup", Trigger: Trigger{Type: "cron", Schedule: "* * * * *"}, Workflow: Workflow{Prompt: "b"}},
				},
				Server: ServerConfig{Port: 7654},
			},
			wantErr: "duplicate name",
		},
		{
			name: "unknown trigger type",
			cfg: Config{
				Automations: []Automation{{Name: "x", Trigger: Trigger{Type: "ftp"}, Workflow: Workflow{Prompt: "test"}}},
				Server:      ServerConfig{Port: 7654},
			},
			wantErr: "unsupported trigger type",
		},
		{
			name: "cron missing schedule",
			cfg: Config{
				Automations: []Automation{{Name: "x", Trigger: Trigger{Type: "cron"}, Workflow: Workflow{Prompt: "test"}}},
				Server:      ServerConfig{Port: 7654},
			},
			wantErr: "requires a schedule",
		},
		{
			name: "webhook missing events",
			cfg: Config{
				Automations: []Automation{{Name: "x", Trigger: Trigger{Type: "github_webhook"}, Workflow: Workflow{Prompt: "test"}}},
				Server:      ServerConfig{Port: 7654},
			},
			wantErr: "requires at least one event",
		},
		{
			name: "missing prompt",
			cfg: Config{
				Automations: []Automation{{Name: "x", Trigger: Trigger{Type: "cron", Schedule: "* * * * *"}, Workflow: Workflow{}}},
				Server:      ServerConfig{Port: 7654},
			},
			wantErr: "prompt is required",
		},
		{
			name: "valid full config",
			cfg: Config{
				Automations: []Automation{
					{Name: "cron-job", Trigger: Trigger{Type: "cron", Schedule: "0 9 * * *"}, Workflow: Workflow{Prompt: "test"}},
					{Name: "webhook-job", Trigger: Trigger{Type: "github_webhook", Events: []string{"push"}}, Workflow: Workflow{Prompt: "check"}},
				},
				Server: ServerConfig{Port: 7654},
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if got := err.Error(); !contains(got, tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, got)
				}
			}
		})
	}
}

func TestAutomationIsEnabled(t *testing.T) {
	a := Automation{Name: "test"}
	if !a.IsEnabled() {
		t.Fatal("nil Enabled should default to true")
	}

	f := false
	a.Enabled = &f
	if a.IsEnabled() {
		t.Fatal("Enabled=false should return false")
	}

	tr := true
	a.Enabled = &tr
	if !a.IsEnabled() {
		t.Fatal("Enabled=true should return true")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
