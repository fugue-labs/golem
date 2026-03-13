package automations

import (
	"testing"
	"time"
)

func TestEventExpandTemplate(t *testing.T) {
	event := Event{
		Type:      "github_webhook",
		Name:      "pr-review",
		Timestamp: time.Date(2026, 3, 12, 14, 0, 0, 0, time.UTC),
		Properties: map[string]any{
			"action": "opened",
			"repository": map[string]any{
				"full_name": "fugue-labs/golem",
				"clone_url": "https://github.com/fugue-labs/golem.git",
			},
			"pull_request": map[string]any{
				"html_url": "https://github.com/fugue-labs/golem/pull/42",
				"number":   float64(42),
				"title":    "Add automations",
			},
		},
	}

	tests := []struct {
		template string
		want     string
	}{
		{
			"Review {{event.pull_request.html_url}}",
			"Review https://github.com/fugue-labs/golem/pull/42",
		},
		{
			"PR #{{event.pull_request.number}} in {{event.repository.full_name}}",
			"PR #42 in fugue-labs/golem",
		},
		{
			"no placeholders here",
			"no placeholders here",
		},
		{
			"{{event.action}} on {{event.repository.full_name}}",
			"opened on fugue-labs/golem",
		},
	}

	for _, tt := range tests {
		t.Run(tt.template, func(t *testing.T) {
			got := event.ExpandTemplate(tt.template)
			if got != tt.want {
				t.Fatalf("ExpandTemplate(%q) = %q, want %q", tt.template, got, tt.want)
			}
		})
	}
}
