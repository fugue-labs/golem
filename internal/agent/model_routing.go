package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/fugue-labs/golem/internal/config"
)

// RoutingConfig controls per-turn model selection based on task complexity.
// Loaded from ~/.golem/routing.json with provider-aware defaults.
type RoutingConfig struct {
	// Enabled controls whether model routing is active.
	// "on" = always route, "off" = always use default model, "auto" = route when fast_model is set.
	Enabled string `json:"enabled"`

	// FastModel is the cheap/fast model used for simple tasks (grep, view, ls, simple edits).
	FastModel string `json:"fast_model"`

	// StrongModel overrides the default model for complex tasks. Empty = use cfg.Model.
	StrongModel string `json:"strong_model,omitempty"`
}

// ModelTier classifies the model tier for a given turn.
type ModelTier string

const (
	TierFast   ModelTier = "fast"
	TierStrong ModelTier = "strong"
)

// routingConfigPath returns the path to the routing config file.
func routingConfigPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".golem", "routing.json")
}

// LoadRoutingConfig loads routing configuration from ~/.golem/routing.json.
// Returns sensible defaults if the file doesn't exist. Routing is off by
// default; set enabled to "on" or "auto" in the config file to enable it.
func LoadRoutingConfig() RoutingConfig {
	path := routingConfigPath()
	if path == "" {
		return RoutingConfig{Enabled: "off"}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return RoutingConfig{Enabled: "off"}
	}
	var rc RoutingConfig
	if err := json.Unmarshal(data, &rc); err != nil {
		return RoutingConfig{Enabled: "off"}
	}
	rc.Enabled = normalizeRoutingEnabled(rc.Enabled)
	return rc
}

// DefaultFastModel returns the default fast model for a given provider.
func DefaultFastModel(provider config.Provider) string {
	switch provider {
	case config.ProviderAnthropic, config.ProviderVertexAnthropic:
		return "claude-haiku-4-5"
	case config.ProviderOpenAI:
		return "gpt-4o-mini"
	case config.ProviderOpenAICompatible:
		return "" // no sensible default for arbitrary endpoints
	case config.ProviderVertexAI:
		return "gemini-2.5-flash"
	default:
		return ""
	}
}

// ResolveFastModel returns the fast model to use, considering config, env, and defaults.
func ResolveFastModel(cfg *config.Config, rc RoutingConfig) string {
	// Explicit env var takes precedence.
	if v := strings.TrimSpace(os.Getenv("GOLEM_FAST_MODEL")); v != "" {
		return v
	}
	// Config file.
	if rc.FastModel != "" {
		return rc.FastModel
	}
	// Provider-aware default.
	if cfg != nil {
		return DefaultFastModel(cfg.Provider)
	}
	return ""
}

// ResolveStrongModel returns the strong model to use.
func ResolveStrongModel(cfg *config.Config, rc RoutingConfig) string {
	if rc.StrongModel != "" {
		return rc.StrongModel
	}
	if cfg != nil {
		return cfg.Model
	}
	return ""
}

// IsRoutingEnabled returns true if model routing should be attempted.
func IsRoutingEnabled(cfg *config.Config, rc RoutingConfig) bool {
	switch rc.Enabled {
	case "on":
		return true
	case "off":
		return false
	default: // "auto"
		return ResolveFastModel(cfg, rc) != "" && ResolveFastModel(cfg, rc) != cfg.Model
	}
}

// RouteModel selects the model for a turn based on the router's complexity classification.
// Returns the model name and a human-readable reason.
func RouteModel(cfg *config.Config, rc RoutingConfig, complexity string) (string, ModelTier, string) {
	fastModel := ResolveFastModel(cfg, rc)
	strongModel := ResolveStrongModel(cfg, rc)

	switch strings.ToLower(strings.TrimSpace(complexity)) {
	case "simple":
		if fastModel != "" && fastModel != strongModel {
			return fastModel, TierFast, "routed → fast (simple task)"
		}
		return strongModel, TierStrong, "strong (fast model unavailable)"
	default: // "complex" or unknown
		return strongModel, TierStrong, "routed → strong (complex task)"
	}
}

// ClassifyPromptHeuristic provides a fast zero-latency complexity classification
// based on prompt patterns. Returns "" if uncertain (should use router).
func ClassifyPromptHeuristic(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ""
	}

	lower := strings.ToLower(prompt)
	words := strings.Fields(lower)
	wordCount := len(words)

	// Very short prompts that are questions about code are typically simple.
	if wordCount <= 5 {
		for _, prefix := range simplePrefixes {
			if strings.HasPrefix(lower, prefix) {
				return "simple"
			}
		}
	}

	// Prompts explicitly about complex operations.
	for _, kw := range complexKeywords {
		if strings.Contains(lower, kw) {
			return "complex"
		}
	}

	// Short prompts with read-intent words.
	if wordCount <= 15 {
		for _, kw := range simpleKeywords {
			if strings.Contains(lower, kw) {
				return "simple"
			}
		}
	}

	return "" // uncertain, defer to router
}

var simplePrefixes = []string{
	"show ", "find ", "search ", "list ", "what is ", "what's ",
	"where is ", "where's ", "how many ", "cat ", "read ",
	"grep ", "glob ", "ls ", "view ",
}

var simpleKeywords = []string{
	"show me", "find the", "search for", "list all", "look up",
	"what does", "where does", "check if", "print ",
}

var complexKeywords = []string{
	"refactor", "implement", "redesign", "migrate", "rewrite",
	"architect", "debug", "fix the bug", "add feature",
	"create a new", "build a", "set up", "configure",
	"across all files", "entire codebase",
}

func normalizeRoutingEnabled(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "on", "true", "1", "yes":
		return "on"
	case "off", "false", "0", "no":
		return "off"
	default:
		return "auto"
	}
}
