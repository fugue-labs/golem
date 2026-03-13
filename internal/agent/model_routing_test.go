package agent

import (
	"testing"

	"github.com/fugue-labs/golem/internal/config"
)

func TestClassifyPromptHeuristic(t *testing.T) {
	tests := []struct {
		prompt string
		want   string
	}{
		// Simple prompts.
		{"show me the config file", "simple"},
		{"find the main function", "simple"},
		{"search for TODO comments", "simple"},
		{"list all go files", "simple"},
		{"what is this function", "simple"},
		{"grep for error handling", "simple"},
		{"view the readme", "simple"},

		// Complex prompts.
		{"refactor the authentication module", "complex"},
		{"implement a new caching layer", "complex"},
		{"debug the race condition in the worker pool", "complex"},
		{"rewrite the database layer to use connection pooling", "complex"},
		{"migrate from REST to gRPC across all files", "complex"},

		// Uncertain prompts (should return empty).
		{"make it better", ""},
		{"can you help me with this code", ""},
		{"I need to update the tests", ""},
	}

	for _, tt := range tests {
		t.Run(tt.prompt, func(t *testing.T) {
			got := ClassifyPromptHeuristic(tt.prompt)
			if got != tt.want {
				t.Errorf("ClassifyPromptHeuristic(%q) = %q, want %q", tt.prompt, got, tt.want)
			}
		})
	}
}

func TestRouteModel(t *testing.T) {
	cfg := &config.Config{
		Provider: config.ProviderAnthropic,
		Model:    "claude-sonnet-4-20250514",
	}
	rc := RoutingConfig{
		Enabled:   "on",
		FastModel: "claude-haiku-4-5",
	}

	// Simple task should route to fast model.
	model, tier, reason := RouteModel(cfg, rc, "simple")
	if model != "claude-haiku-4-5" {
		t.Errorf("simple: model = %q, want claude-haiku-4-5", model)
	}
	if tier != TierFast {
		t.Errorf("simple: tier = %q, want fast", tier)
	}
	if reason == "" {
		t.Error("simple: empty reason")
	}

	// Complex task should route to strong model.
	model, tier, reason = RouteModel(cfg, rc, "complex")
	if model != "claude-sonnet-4-20250514" {
		t.Errorf("complex: model = %q, want claude-sonnet-4-20250514", model)
	}
	if tier != TierStrong {
		t.Errorf("complex: tier = %q, want strong", tier)
	}
	if reason == "" {
		t.Error("complex: empty reason")
	}
}

func TestRouteModelNoFastModel(t *testing.T) {
	cfg := &config.Config{
		Provider: config.ProviderOpenAICompatible,
		Model:    "custom-model",
	}
	rc := RoutingConfig{
		Enabled:   "on",
		FastModel: "",
	}

	// When no fast model, even simple tasks use strong model.
	model, tier, _ := RouteModel(cfg, rc, "simple")
	if model != "custom-model" {
		t.Errorf("model = %q, want custom-model", model)
	}
	if tier != TierStrong {
		t.Errorf("tier = %q, want strong", tier)
	}
}

func TestIsRoutingEnabled(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		rc      RoutingConfig
		want    bool
	}{
		{
			name: "forced on",
			cfg:  &config.Config{Provider: config.ProviderAnthropic, Model: "claude-sonnet-4"},
			rc:   RoutingConfig{Enabled: "on", FastModel: "claude-haiku-4-5"},
			want: true,
		},
		{
			name: "forced off",
			cfg:  &config.Config{Provider: config.ProviderAnthropic, Model: "claude-sonnet-4"},
			rc:   RoutingConfig{Enabled: "off", FastModel: "claude-haiku-4-5"},
			want: false,
		},
		{
			name: "auto with fast model available",
			cfg:  &config.Config{Provider: config.ProviderAnthropic, Model: "claude-sonnet-4"},
			rc:   RoutingConfig{Enabled: "auto"},
			want: true, // default fast model for Anthropic is haiku
		},
		{
			name: "auto with same model",
			cfg:  &config.Config{Provider: config.ProviderAnthropic, Model: "claude-haiku-4-5"},
			rc:   RoutingConfig{Enabled: "auto"},
			want: false, // fast model = main model, no point routing
		},
		{
			name: "auto with no fast model",
			cfg:  &config.Config{Provider: config.ProviderOpenAICompatible, Model: "custom-model"},
			rc:   RoutingConfig{Enabled: "auto"},
			want: false, // no default fast model for openai_compatible
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRoutingEnabled(tt.cfg, tt.rc)
			if got != tt.want {
				t.Errorf("IsRoutingEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultFastModel(t *testing.T) {
	tests := []struct {
		provider config.Provider
		want     string
	}{
		{config.ProviderAnthropic, "claude-haiku-4-5"},
		{config.ProviderOpenAI, "gpt-4o-mini"},
		{config.ProviderVertexAI, "gemini-2.5-flash"},
		{config.ProviderVertexAnthropic, "claude-haiku-4-5"},
		{config.ProviderOpenAICompatible, ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.provider), func(t *testing.T) {
			got := DefaultFastModel(tt.provider)
			if got != tt.want {
				t.Errorf("DefaultFastModel(%s) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}

func TestNormalizeRoutingEnabled(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"on", "on"},
		{"ON", "on"},
		{"true", "on"},
		{"1", "on"},
		{"off", "off"},
		{"false", "off"},
		{"0", "off"},
		{"auto", "auto"},
		{"", "auto"},
		{"invalid", "auto"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeRoutingEnabled(tt.input)
			if got != tt.want {
				t.Errorf("normalizeRoutingEnabled(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
