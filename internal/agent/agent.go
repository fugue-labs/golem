package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/skills"
	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/codetool"
	"github.com/fugue-labs/gollem/provider/anthropic"
	"github.com/fugue-labs/gollem/provider/openai"
	"github.com/fugue-labs/gollem/provider/vertexai"
	vertexanthropic "github.com/fugue-labs/gollem/provider/vertexai_anthropic"
)

// New creates a configured coding agent using the full competitive gollem
// codetool agent setup — battle-tested system prompt, all tools (bash,
// bash_status, bash_kill, view, write, edit, multi_edit, grep, glob, ls,
// lsp), middleware (loop detection, progress tracking, context injection,
// reasoning sandwich, verification, context overflow), guardrails,
// auto-context, and tracing.
//
// Additional AgentOptions can be passed to customize the agent (e.g., hooks).
// activeSkills are injected as a dynamic system prompt.
func New(cfg *config.Config, activeSkills []skills.Skill, extra ...core.AgentOption[string]) (*core.Agent[string], error) {
	model, err := createModel(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating model: %w", err)
	}

	// Use the full competitive agent options from codetool.
	// WithModel enables delegate/subagent, invariants, and open_image.
	toolOpts := []codetool.Option{
		codetool.WithModel(model),
	}

	// Provider-aware auto-context limits — match the competitive CLI.
	switch cfg.Provider {
	case config.ProviderAnthropic, config.ProviderVertexAnthropic:
		toolOpts = append(toolOpts, codetool.WithAutoContextConfig(core.AutoContextConfig{
			MaxTokens: 150000,
			KeepLastN: 12,
		}))
	case config.ProviderOpenAI:
		toolOpts = append(toolOpts, codetool.WithAutoContextConfig(core.AutoContextConfig{
			MaxTokens: 900000,
			KeepLastN: 20,
		}))
	case config.ProviderVertexAI:
		toolOpts = append(toolOpts, codetool.WithAutoContextConfig(core.AutoContextConfig{
			MaxTokens: 900000,
			KeepLastN: 20,
		}))
	case config.ProviderOpenAICompatible:
		toolOpts = append(toolOpts, codetool.WithAutoContextConfig(core.AutoContextConfig{
			MaxTokens: 900000,
			KeepLastN: 20,
		}))
	}

	opts := codetool.AgentOptions(cfg.WorkingDir, toolOpts...)

	// Inject skills as dynamic system prompt.
	if len(activeSkills) > 0 {
		prompt := buildSkillsPrompt(activeSkills)
		opts = append(opts, core.WithDynamicSystemPrompt[string](
			func(_ context.Context, _ *core.RunContext) (string, error) {
				return prompt, nil
			},
		))
	}

	// Reasoning effort override.
	if cfg.ReasoningEffort != "" {
		opts = append(opts, core.WithReasoningEffort[string](cfg.ReasoningEffort))
	}

	// Append caller-provided options (e.g., TUI hooks).
	opts = append(opts, extra...)

	agent := core.NewAgent[string](model, opts...)
	return agent, nil
}

func buildSkillsPrompt(activeSkills []skills.Skill) string {
	var b strings.Builder
	b.WriteString("\n# Active Skills\n\n")
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
			openai.WithMaxTokens(128000),
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
			openai.WithMaxTokens(128000),
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
