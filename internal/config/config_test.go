package config

import (
	"testing"
	"time"
)

func TestLoadOpenAICompatibleConfig(t *testing.T) {
	t.Setenv("GOLEM_PROVIDER", "openai_compatible")
	t.Setenv("GOLEM_API_KEY", "test-key")
	t.Setenv("GOLEM_BASE_URL", "https://example.test/v1")
	t.Setenv("GOLEM_MODEL", "grok-test")
	t.Setenv("GOLEM_ROUTER_MODEL", "grok-cheap")
	t.Setenv("GOLEM_TIMEOUT", "45s")
	t.Setenv("GOLEM_TEAM_MODE", "on")
	t.Setenv("GOLEM_DISABLE_CODE_MODE", "true")
	t.Setenv("GOLEM_TOP_LEVEL_PERSONALITY", "1")
	t.Setenv("GOLEM_DISABLE_GREEDY_THINKING_PRESSURE", "yes")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Provider != ProviderOpenAICompatible {
		t.Fatalf("provider = %q", cfg.Provider)
	}
	if cfg.APIKey != "test-key" || cfg.BaseURL != "https://example.test/v1" || cfg.Model != "grok-test" {
		t.Fatalf("unexpected api/model fields: %+v", cfg)
	}
	if cfg.RouterModel != "grok-cheap" {
		t.Fatalf("router model = %q", cfg.RouterModel)
	}
	if cfg.Timeout != 45*time.Second {
		t.Fatalf("timeout = %s", cfg.Timeout)
	}
	if cfg.TeamMode != "on" || cfg.DisableCodeMode != true {
		t.Fatalf("unexpected team/code mode fields: %+v", cfg)
	}
	if !cfg.TopLevelPersonality || !cfg.DisableGreedyThinkingPressure {
		t.Fatalf("expected personality and greedy-thinking flags to be enabled")
	}
	if cfg.AutoContextMaxTokens != 900000 || cfg.AutoContextKeepLastN != 20 {
		t.Fatalf("unexpected auto-context config: %d/%d", cfg.AutoContextMaxTokens, cfg.AutoContextKeepLastN)
	}
}

func TestLoadOpenAIConfigReadsBaseURLOverride(t *testing.T) {
	t.Setenv("GOLEM_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("GOLEM_BASE_URL", "https://proxy.example.test/v1")
	t.Setenv("OPENAI_BASE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Provider != ProviderOpenAI {
		t.Fatalf("provider = %q", cfg.Provider)
	}
	if cfg.BaseURL != "https://proxy.example.test/v1" {
		t.Fatalf("baseURL = %q", cfg.BaseURL)
	}
}

func TestLoadRejectsInvalidTimeout(t *testing.T) {
	t.Setenv("GOLEM_PROVIDER", "anthropic")
	t.Setenv("GOLEM_TIMEOUT", "definitely-not-a-duration")

	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid timeout error")
	}
}

func TestDetectProviderPrefersExplicitCompatibleSignals(t *testing.T) {
	keys := []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "XAI_API_KEY", "GOLEM_BASE_URL", "GOLEM_API_KEY", "GOOGLE_APPLICATION_CREDENTIALS", "VERTEX_PROJECT"}
	for _, key := range keys {
		t.Setenv(key, "")
	}

	t.Setenv("GOLEM_API_KEY", "x")
	if got := detectProvider(); got != ProviderOpenAICompatible {
		t.Fatalf("detectProvider() = %q", got)
	}
}
