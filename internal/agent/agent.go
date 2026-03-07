package agent

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/fugue-labs/golem/internal/agent/tools"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/skills"
	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/provider/anthropic"
	"github.com/fugue-labs/gollem/provider/openai"
	"github.com/fugue-labs/gollem/provider/vertexai"
	vertexanthropic "github.com/fugue-labs/gollem/provider/vertexai_anthropic"
)

//go:embed system_prompt.md
var systemPrompt string

// New creates a configured coding agent from the given config.
// Additional AgentOptions can be passed to customize the agent (e.g., hooks).
// activeSkills are injected into the system prompt.
func New(cfg *config.Config, activeSkills []skills.Skill, extra ...core.AgentOption[string]) (*core.Agent[string], error) {
	model, err := createModel(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating model: %w", err)
	}

	codingTools := tools.CodingTools(cfg.WorkingDir)

	prompt := buildSystemPrompt(activeSkills)

	opts := []core.AgentOption[string]{
		core.WithSystemPrompt[string](prompt),
		core.WithTools[string](codingTools...),
		core.WithMaxConcurrency[string](1),
		core.WithOutputOptions[string](core.WithOutputMode(core.OutputModeText)),
	}
	if cfg.ReasoningEffort != "" {
		opts = append(opts, core.WithReasoningEffort[string](cfg.ReasoningEffort))
	}
	opts = append(opts, extra...)

	agent := core.NewAgent[string](model, opts...)
	return agent, nil
}

func buildSystemPrompt(activeSkills []skills.Skill) string {
	if len(activeSkills) == 0 {
		return systemPrompt
	}
	var b strings.Builder
	b.WriteString(systemPrompt)
	b.WriteString("\n\n# Active Skills\n\n")
	for _, s := range activeSkills {
		b.WriteString("## Skill: ")
		b.WriteString(s.Name)
		b.WriteString("\n\n")
		b.WriteString(s.Content)
		b.WriteString("\n\n")
	}
	return b.String()
}

func createModel(cfg *config.Config) (core.Model, error) {
	switch cfg.Provider {
	case config.ProviderAnthropic:
		opts := []anthropic.Option{
			anthropic.WithModel(cfg.Model),
		}
		if cfg.APIKey != "" {
			opts = append(opts, anthropic.WithAPIKey(cfg.APIKey))
		}
		return anthropic.New(opts...), nil

	case config.ProviderOpenAI:
		opts := []openai.Option{
			openai.WithModel(cfg.Model),
			openai.WithPromptCacheRetention("24h"),
			openai.WithPromptCacheKey("golem"),
		}
		if cfg.APIKey != "" {
			opts = append(opts, openai.WithAPIKey(cfg.APIKey))
		}
		return openai.New(opts...), nil

	case config.ProviderOpenAICompatible:
		opts := []openai.Option{
			openai.WithModel(cfg.Model),
			openai.WithPromptCacheRetention("24h"),
			openai.WithPromptCacheKey("golem"),
		}
		if cfg.APIKey != "" {
			opts = append(opts, openai.WithAPIKey(cfg.APIKey))
		}
		if cfg.BaseURL != "" {
			opts = append(opts, openai.WithBaseURL(cfg.BaseURL))
		}
		return openai.New(opts...), nil

	case config.ProviderVertexAI:
		opts := []vertexai.Option{
			vertexai.WithModel(cfg.Model),
		}
		if cfg.ProjectID != "" {
			opts = append(opts, vertexai.WithProject(cfg.ProjectID))
		}
		if cfg.Region != "" {
			opts = append(opts, vertexai.WithLocation(cfg.Region))
		}
		return vertexai.New(opts...), nil

	case config.ProviderVertexAnthropic:
		opts := []vertexanthropic.Option{
			vertexanthropic.WithModel(cfg.Model),
		}
		if cfg.ProjectID != "" {
			opts = append(opts, vertexanthropic.WithProject(cfg.ProjectID))
		}
		if cfg.Region != "" {
			opts = append(opts, vertexanthropic.WithLocation(cfg.Region))
		}
		return vertexanthropic.New(opts...), nil

	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}
