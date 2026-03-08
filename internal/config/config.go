package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	BaseURL         string // for custom OpenAI/OpenAI-compatible endpoints (xAI, Groq, proxies, etc.)
	WorkingDir      string
	ProjectID       string // for Vertex AI
	Region          string // for Vertex AI
	ReasoningEffort string // for reasoning models (e.g., "low", "medium", "high", "xhigh")
	ThinkingBudget  int    // for Anthropic and Gemini thinking-capable models
	Timeout         time.Duration

	TeamMode                      string // auto, on, off
	DisableDelegate               bool
	DisableCodeMode               bool
	TopLevelPersonality           bool
	DisableGreedyThinkingPressure bool

	AutoContextMaxTokens int
	AutoContextKeepLastN int
}

// Load reads configuration from environment variables and flags.
func Load() (*Config, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	cfg := &Config{WorkingDir: wd}
	cfg.Provider = detectProvider()
	if p := strings.TrimSpace(os.Getenv("GOLEM_PROVIDER")); p != "" {
		cfg.Provider = Provider(strings.ToLower(p))
	}

	switch cfg.Provider {
	case ProviderAnthropic:
		cfg.APIKey = os.Getenv("ANTHROPIC_API_KEY")
		cfg.Model = envOr("GOLEM_MODEL", "claude-sonnet-4-20250514")
		cfg.ThinkingBudget, err = intEnvOr("GOLEM_THINKING_BUDGET", 16000)
		if err != nil {
			return nil, err
		}
		cfg.AutoContextMaxTokens = 150000
		cfg.AutoContextKeepLastN = 12

	case ProviderOpenAI:
		cfg.APIKey = os.Getenv("OPENAI_API_KEY")
		cfg.BaseURL = firstNonEmpty(os.Getenv("GOLEM_BASE_URL"), os.Getenv("OPENAI_BASE_URL"))
		cfg.Model = envOr("GOLEM_MODEL", "gpt-5.4")
		cfg.ReasoningEffort = envOr("GOLEM_REASONING_EFFORT", "xhigh")
		cfg.AutoContextMaxTokens = 900000
		cfg.AutoContextKeepLastN = 20

	case ProviderOpenAICompatible:
		cfg.APIKey = firstNonEmpty(os.Getenv("GOLEM_API_KEY"), os.Getenv("XAI_API_KEY"))
		cfg.BaseURL = firstNonEmpty(os.Getenv("GOLEM_BASE_URL"), os.Getenv("XAI_BASE_URL"), "https://api.x.ai/v1")
		cfg.Model = envOr("GOLEM_MODEL", "grok-3")
		cfg.AutoContextMaxTokens = 900000
		cfg.AutoContextKeepLastN = 20

	case ProviderVertexAI:
		cfg.ProjectID = os.Getenv("VERTEX_PROJECT")
		cfg.Region = envOr("VERTEX_REGION", "us-central1")
		cfg.Model = envOr("GOLEM_MODEL", "gemini-2.5-pro")
		cfg.ThinkingBudget, err = intEnvOr("GOLEM_THINKING_BUDGET", 16000)
		if err != nil {
			return nil, err
		}
		cfg.AutoContextMaxTokens = 900000
		cfg.AutoContextKeepLastN = 20

	case ProviderVertexAnthropic:
		cfg.ProjectID = os.Getenv("VERTEX_PROJECT")
		cfg.Region = envOr("VERTEX_REGION", "us-central1")
		cfg.Model = envOr("GOLEM_MODEL", "claude-sonnet-4-5")
		cfg.ThinkingBudget, err = intEnvOr("GOLEM_THINKING_BUDGET", 16000)
		if err != nil {
			return nil, err
		}
		cfg.AutoContextMaxTokens = 150000
		cfg.AutoContextKeepLastN = 12

	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}

	timeout, err := durationEnvOr("GOLEM_TIMEOUT", 30*time.Minute)
	if err != nil {
		return nil, err
	}
	cfg.Timeout = timeout
	cfg.TeamMode = teamModeEnvOr("GOLEM_TEAM_MODE", "auto")
	cfg.DisableDelegate = isTruthyEnv("GOLEM_DISABLE_DELEGATE")
	cfg.DisableCodeMode = isTruthyEnv("GOLEM_DISABLE_CODE_MODE") || isTruthyEnv("GOLEM_NO_CODE_MODE")
	cfg.TopLevelPersonality = isTruthyEnv("GOLEM_TOP_LEVEL_PERSONALITY")
	cfg.DisableGreedyThinkingPressure = isTruthyEnv("GOLEM_DISABLE_GREEDY_THINKING_PRESSURE")

	return cfg, nil
}

func detectProvider() Provider {
	switch {
	case hasNonEmptyEnv("ANTHROPIC_API_KEY"):
		return ProviderAnthropic
	case hasNonEmptyEnv("OPENAI_API_KEY"):
		return ProviderOpenAI
	case hasNonEmptyEnv("XAI_API_KEY") || hasNonEmptyEnv("GOLEM_BASE_URL") || hasNonEmptyEnv("GOLEM_API_KEY"):
		return ProviderOpenAICompatible
	case hasNonEmptyEnv("GOOGLE_APPLICATION_CREDENTIALS") || hasNonEmptyEnv("VERTEX_PROJECT"):
		return ProviderVertexAI
	default:
		return ProviderAnthropic
	}
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
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func durationEnvOr(key string, fallback time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s duration %q: %w", key, v, err)
	}
	return d, nil
}

func intEnvOr(key string, fallback int) (int, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s integer %q: %w", key, v, err)
	}
	return n, nil
}

func teamModeEnvOr(key, fallback string) string {
	v := strings.ToLower(strings.TrimSpace(envOr(key, fallback)))
	switch v {
	case "on", "off", "auto":
		return v
	default:
		return fallback
	}
}

func hasNonEmptyEnv(key string) bool {
	return strings.TrimSpace(os.Getenv(key)) != ""
}

func isTruthyEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
