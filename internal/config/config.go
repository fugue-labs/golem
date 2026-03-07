package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Provider identifies an LLM provider.
type Provider string

const (
	ProviderAnthropic        Provider = "anthropic"
	ProviderOpenAI           Provider = "openai"
	ProviderVertexAI         Provider = "vertexai"
	ProviderVertexAnthropic  Provider = "vertexai_anthropic"
	ProviderOpenAICompatible Provider = "openai_compatible"
)

// Config holds the application configuration.
type Config struct {
	Provider        Provider
	Model           string
	APIKey          string
	BaseURL         string // for OpenAI-compatible endpoints (xAI, Groq, etc.)
	WorkingDir      string
	ProjectID       string // for Vertex AI
	Region          string // for Vertex AI
	ReasoningEffort string // for reasoning models (e.g., "low", "medium", "high", "xhigh")
}

// Load reads configuration from environment variables and flags.
func Load() (*Config, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	cfg := &Config{
		WorkingDir: wd,
	}

	// Detect provider from environment.
	switch {
	case os.Getenv("ANTHROPIC_API_KEY") != "":
		cfg.Provider = ProviderAnthropic
		cfg.APIKey = os.Getenv("ANTHROPIC_API_KEY")
		cfg.Model = envOr("GOLEM_MODEL", "claude-sonnet-4-20250514")

	case os.Getenv("OPENAI_API_KEY") != "":
		cfg.Provider = ProviderOpenAI
		cfg.APIKey = os.Getenv("OPENAI_API_KEY")
		cfg.Model = envOr("GOLEM_MODEL", "gpt-5.4")
		cfg.ReasoningEffort = envOr("GOLEM_REASONING_EFFORT", "xhigh")

	case os.Getenv("XAI_API_KEY") != "":
		cfg.Provider = ProviderOpenAICompatible
		cfg.APIKey = os.Getenv("XAI_API_KEY")
		cfg.BaseURL = envOr("XAI_BASE_URL", "https://api.x.ai/v1")
		cfg.Model = envOr("GOLEM_MODEL", "grok-3")

	case os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" || os.Getenv("VERTEX_PROJECT") != "":
		cfg.Provider = ProviderVertexAI
		cfg.ProjectID = os.Getenv("VERTEX_PROJECT")
		cfg.Region = envOr("VERTEX_REGION", "us-central1")
		cfg.Model = envOr("GOLEM_MODEL", "gemini-2.5-pro")

	default:
		// Default to anthropic if no env detected.
		cfg.Provider = ProviderAnthropic
		cfg.Model = envOr("GOLEM_MODEL", "claude-sonnet-4-20250514")
	}

	// Override provider if explicitly set.
	if p := os.Getenv("GOLEM_PROVIDER"); p != "" {
		cfg.Provider = Provider(strings.ToLower(p))
	}
	if m := os.Getenv("GOLEM_MODEL"); m != "" {
		cfg.Model = m
	}
	if u := os.Getenv("GOLEM_BASE_URL"); u != "" {
		cfg.BaseURL = u
	}

	return cfg, nil
}

// ShortDir returns a display-friendly working directory path.
func (c *Config) ShortDir() string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(c.WorkingDir, home) {
		return "~" + c.WorkingDir[len(home):]
	}
	return filepath.Base(c.WorkingDir)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
