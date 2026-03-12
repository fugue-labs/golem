package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fugue-labs/golem/internal/login"
	openaiauth "github.com/fugue-labs/gollem/auth/openai"
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

// ProviderSource describes where the provider selection came from.
type ProviderSource string

const (
	SourceDefault  ProviderSource = "default"
	SourceEnvVar   ProviderSource = "env"
	SourceLogin    ProviderSource = "golem login"
	SourceGolemEnv ProviderSource = "GOLEM_PROVIDER"
)

// Config holds the application configuration.
type Config struct {
	Provider       Provider
	ProviderSource ProviderSource
	Model          string
	RouterModel    string
	//nolint:gosec // Provider auth is stored in memory for runtime use and masked in user-facing status output.
	APIKey          string
	BaseURL         string // for custom OpenAI/OpenAI-compatible endpoints (xAI, Groq, proxies, etc.)
	WorkingDir      string
	ProjectID       string // for Vertex AI
	Region          string // for Vertex AI
	ReasoningEffort string // for reasoning models (e.g., "low", "medium", "high", "xhigh")
	ThinkingBudget  int    // for Anthropic and Gemini thinking-capable models
	Timeout         time.Duration

	RawTeamMode                   string
	TeamMode                      string // auto, on, off
	DisableDelegate               bool
	DisableCodeMode               bool
	EnableFetchURL                bool
	TopLevelPersonality           bool
	DisableGreedyThinkingPressure bool
	PermissionMode                string // "auto" or "suggest" (default: "suggest")

	AutoContextMaxTokens int
	AutoContextKeepLastN int

	// ChatGPT subscription auth (populated from ~/.golem/auth.json).
	ChatGPTCreds *openaiauth.Credentials // nil when not using ChatGPT auth

	// LoginProvider is the raw provider name from `golem login` (e.g. "chatgpt").
	// Empty when login config wasn't used.
	LoginProvider string
}

// ValidationResult captures fatal config errors and non-fatal warnings.
type ValidationResult struct {
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// HasErrors reports whether validation found fatal issues.
func (r ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// Load reads configuration with the following precedence:
//  1. GOLEM_PROVIDER env var (explicit override, always wins)
//  2. Saved config from `golem login` (~/.golem/config.json)
//  3. Env var auto-detection (ANTHROPIC_API_KEY, OPENAI_API_KEY, etc.)
//  4. Default (anthropic)
func Load() (*Config, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	cfg := &Config{WorkingDir: wd}
	savedKeys, _ := login.LoadAPIKeys()

	// --- Determine provider (precedence: GOLEM_PROVIDER > golem login > env detection > default) ---

	if p := strings.TrimSpace(os.Getenv("GOLEM_PROVIDER")); p != "" {
		// 1. Explicit GOLEM_PROVIDER env var.
		cfg.Provider = Provider(strings.ToLower(p))
		cfg.ProviderSource = SourceGolemEnv
	} else if sc := login.LoadConfig(); sc != nil {
		// 2. Saved config from `golem login`.
		cfg.LoginProvider = sc.Provider
		cfg.ProviderSource = SourceLogin
		switch sc.Provider {
		case "chatgpt":
			cfg.Provider = ProviderOpenAI
			if creds, err := openaiauth.LoadCredentials(); err == nil && creds.AuthMode == "chatgpt" {
				cfg.ChatGPTCreds = creds
			}
		case "openai":
			cfg.Provider = ProviderOpenAI
		case "anthropic":
			cfg.Provider = ProviderAnthropic
		case "xai":
			cfg.Provider = ProviderOpenAICompatible
		default:
			cfg.Provider = Provider(sc.Provider)
		}
	} else {
		// 3. Env var auto-detection / 4. Default.
		cfg.Provider, cfg.ProviderSource = detectProvider(savedKeys)
	}

	// --- Configure provider-specific settings ---

	switch cfg.Provider {
	case ProviderAnthropic:
		cfg.APIKey = firstNonEmpty(os.Getenv("ANTHROPIC_API_KEY"), savedKeys["anthropic"])
		cfg.Model = envOr("GOLEM_MODEL", "claude-sonnet-4-20250514")
		cfg.ThinkingBudget, err = intEnvOr("GOLEM_THINKING_BUDGET", 16000)
		if err != nil {
			return nil, err
		}
		cfg.AutoContextMaxTokens = 150000
		cfg.AutoContextKeepLastN = 12

	case ProviderOpenAI:
		if cfg.ChatGPTCreds == nil {
			// Prefer API keys when configured; otherwise fall back to ChatGPT
			// subscription credentials from ~/.golem/auth.json.
			cfg.APIKey = firstNonEmpty(os.Getenv("OPENAI_API_KEY"), savedKeys["openai"])
			if cfg.APIKey == "" {
				if creds, err := openaiauth.LoadCredentials(); err == nil && creds.AuthMode == "chatgpt" {
					cfg.ChatGPTCreds = creds
				}
			}
		}
		cfg.BaseURL = firstNonEmpty(os.Getenv("GOLEM_BASE_URL"), os.Getenv("OPENAI_BASE_URL"))
		cfg.Model = envOr("GOLEM_MODEL", "gpt-5.4")
		cfg.ReasoningEffort = envOr("GOLEM_REASONING_EFFORT", "xhigh")
		cfg.AutoContextMaxTokens = 900000
		cfg.AutoContextKeepLastN = 20

	case ProviderOpenAICompatible:
		cfg.APIKey = firstNonEmpty(os.Getenv("GOLEM_API_KEY"), os.Getenv("XAI_API_KEY"), savedKeys["xai"])
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
	cfg.RouterModel = firstNonEmpty(os.Getenv("GOLEM_ROUTER_MODEL"), os.Getenv("GOLEM_CHEAP_MODEL"))
	cfg.RawTeamMode = strings.TrimSpace(os.Getenv("GOLEM_TEAM_MODE"))
	cfg.TeamMode = teamModeEnvOr("GOLEM_TEAM_MODE", "auto")
	cfg.DisableDelegate = isTruthyEnv("GOLEM_DISABLE_DELEGATE")
	cfg.DisableCodeMode = isTruthyEnv("GOLEM_DISABLE_CODE_MODE") || isTruthyEnv("GOLEM_NO_CODE_MODE")
	cfg.EnableFetchURL = isTruthyEnv("GOLEM_ENABLE_FETCH_URL")
	cfg.TopLevelPersonality = isTruthyEnv("GOLEM_TOP_LEVEL_PERSONALITY")
	cfg.DisableGreedyThinkingPressure = isTruthyEnv("GOLEM_DISABLE_GREEDY_THINKING_PRESSURE")
	cfg.PermissionMode = permissionModeEnvOr("GOLEM_PERMISSION_MODE", "suggest")

	return cfg, nil
}

// Validate reports fatal config errors and non-fatal warnings.
func (c *Config) Validate() ValidationResult {
	if c == nil {
		return ValidationResult{Errors: []string{"config is nil"}}
	}

	var result ValidationResult

	if raw := strings.ToLower(strings.TrimSpace(c.RawTeamMode)); raw != "" && !isValidTeamMode(raw) {
		result.Errors = append(result.Errors, fmt.Sprintf("invalid GOLEM_TEAM_MODE %q: must be one of auto, on, off", c.RawTeamMode))
	}

	switch c.Provider {
	case ProviderAnthropic:
		if strings.TrimSpace(c.APIKey) == "" {
			result.Errors = append(result.Errors, "ANTHROPIC_API_KEY is required for anthropic provider")
		}
	case ProviderOpenAI:
		if strings.TrimSpace(c.APIKey) == "" && c.ChatGPTCreds == nil {
			result.Errors = append(result.Errors, "OPENAI_API_KEY or ChatGPT subscription credentials are required for openai provider")
		}
	case ProviderOpenAICompatible:
		if strings.TrimSpace(c.APIKey) == "" {
			result.Errors = append(result.Errors, "GOLEM_API_KEY or XAI_API_KEY is required for openai_compatible provider")
		}
		if strings.TrimSpace(c.BaseURL) == "" {
			result.Errors = append(result.Errors, "GOLEM_BASE_URL or XAI_BASE_URL is required for openai_compatible provider")
		}
	case ProviderVertexAI, ProviderVertexAnthropic:
		if strings.TrimSpace(c.ProjectID) == "" {
			result.Errors = append(result.Errors, "VERTEX_PROJECT is required for vertex providers")
		}
		if strings.TrimSpace(c.Region) == "" {
			result.Errors = append(result.Errors, "VERTEX_REGION is required for vertex providers")
		}
	default:
		result.Errors = append(result.Errors, fmt.Sprintf("unsupported provider: %s", c.Provider))
	}

	if c.Timeout <= 0 {
		result.Errors = append(result.Errors, "GOLEM_TIMEOUT must be greater than zero")
	}
	if c.AutoContextMaxTokens < 0 {
		result.Errors = append(result.Errors, "auto-context max tokens must be non-negative")
	}
	if c.AutoContextKeepLastN < 0 {
		result.Errors = append(result.Errors, "auto-context keep-last turns must be non-negative")
	}
	if c.AutoContextMaxTokens > 0 && c.AutoContextKeepLastN == 0 {
		result.Errors = append(result.Errors, "auto-context keep-last turns must be greater than zero when auto-context is enabled")
	}
	if c.DisableDelegate && c.TeamMode == "on" {
		result.Warnings = append(result.Warnings, "team mode is forced on but delegate is disabled, so team mode will remain off at runtime")
	}

	return result
}

// Status returns a human-readable summary of the current configuration.
func Status() string {
	cfg, err := Load()
	if err != nil {
		return fmt.Sprintf("Error loading config: %v", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Provider:  %s", cfg.Provider)
	if cfg.LoginProvider != "" {
		fmt.Fprintf(&b, " (%s)", cfg.LoginProvider)
	}
	fmt.Fprintf(&b, "\nSource:    %s", cfg.ProviderSource)
	fmt.Fprintf(&b, "\nModel:     %s", cfg.Model)

	// Auth method.
	_, authSummary := cfg.AuthStatus()
	fmt.Fprintf(&b, "\nAuth:      %s", authSummary)

	if cfg.BaseURL != "" {
		fmt.Fprintf(&b, "\nBase URL:  %s", cfg.BaseURL)
	}

	return b.String()
}

// AuthStatus returns the auth mode and a human-readable summary.
func (c *Config) AuthStatus() (string, string) {
	switch {
	case c == nil:
		return "missing", "none"
	case c.ChatGPTCreds != nil:
		return "chatgpt_oauth", "ChatGPT subscription (OAuth)"
	case strings.TrimSpace(c.APIKey) != "":
		return "api_key", fmt.Sprintf("API key (%s)", maskedSecret(c.APIKey))
	default:
		return "missing", "none (will fail at runtime)"
	}
}

func detectProvider(savedKeys map[string]string) (Provider, ProviderSource) {
	// Check env vars.
	switch {
	case hasNonEmptyEnv("ANTHROPIC_API_KEY"):
		return ProviderAnthropic, SourceEnvVar
	case hasNonEmptyEnv("OPENAI_API_KEY"):
		return ProviderOpenAI, SourceEnvVar
	case hasNonEmptyEnv("XAI_API_KEY") || hasNonEmptyEnv("GOLEM_BASE_URL") || hasNonEmptyEnv("GOLEM_API_KEY"):
		return ProviderOpenAICompatible, SourceEnvVar
	case hasNonEmptyEnv("GOOGLE_APPLICATION_CREDENTIALS") || hasNonEmptyEnv("VERTEX_PROJECT"):
		return ProviderVertexAI, SourceEnvVar
	}

	// Check saved API keys.
	switch {
	case savedKeys["anthropic"] != "":
		return ProviderAnthropic, SourceLogin
	case savedKeys["openai"] != "":
		return ProviderOpenAI, SourceLogin
	case savedKeys["xai"] != "":
		return ProviderOpenAICompatible, SourceLogin
	}

	// Check for ChatGPT OAuth creds on disk (legacy — before config.json was saved).
	if creds, err := openaiauth.LoadCredentials(); err == nil && creds.AuthMode == "chatgpt" {
		return ProviderOpenAI, SourceLogin
	}

	return ProviderAnthropic, SourceDefault
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
	return normalizeTeamMode(v, fallback)
}

func normalizeTeamMode(value, fallback string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "on", "off", "auto":
		return v
	default:
		return fallback
	}
}

func isValidTeamMode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "off", "auto":
		return true
	default:
		return false
	}
}

func maskedSecret(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= 8 {
		return trimmed
	}
	return trimmed[:4] + "..." + trimmed[len(trimmed)-4:]
}

func hasNonEmptyEnv(key string) bool {
	return strings.TrimSpace(os.Getenv(key)) != ""
}

func permissionModeEnvOr(key, fallback string) string {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "auto", "suggest":
		return v
	default:
		return fallback
	}
}

func isTruthyEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
