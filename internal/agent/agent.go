package agent

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"sync"

	montygo "github.com/fugue-labs/monty-go"

	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/skills"
	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/codetool"
	"github.com/fugue-labs/gollem/modelutil"
	"github.com/fugue-labs/gollem/provider/anthropic"
	"github.com/fugue-labs/gollem/provider/openai"
	"github.com/fugue-labs/gollem/provider/vertexai"
	vertexanthropic "github.com/fugue-labs/gollem/provider/vertexai_anthropic"
)

//go:embed system_prompt.md
var vesselSystemPrompt string

var (
	codeRunnerOnce sync.Once
	codeRunner     *montygo.Runner
	codeRunnerErr  error
)

// New creates a configured coding agent using Gollem's full codetool stack,
// plus Vessel-specific runtime guidance that highlights the framework's
// strongest capabilities in the TUI.
func New(cfg *config.Config, runPrompt string, activeSkills []skills.Skill, extra ...core.AgentOption[string]) (*core.Agent[string], error) {
	model, err := createModel(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating model: %w", err)
	}

	applyRuntimeProfile(cfg, runPrompt)

	toolOpts := []codetool.Option{
		codetool.WithModel(model),
		codetool.WithTimeout(cfg.Timeout),
	}
	if cfg.AutoContextMaxTokens > 0 {
		toolOpts = append(toolOpts, codetool.WithAutoContextConfig(core.AutoContextConfig{
			MaxTokens: cfg.AutoContextMaxTokens,
			KeepLastN: cfg.AutoContextKeepLastN,
		}))
	}
	if cfg.DisableDelegate {
		toolOpts = append(toolOpts, codetool.WithDisableDelegate())
	}
	if cfg.EffectiveTeamMode {
		toolOpts = append(toolOpts, codetool.WithTeamMode())
	}
	if cfg.DisableGreedyThinkingPressure {
		toolOpts = append(toolOpts, codetool.WithDisableGreedyThinkingPressure())
	}
	if sandwichCfg, ok := reasoningSandwichConfig(cfg); ok {
		toolOpts = append(toolOpts, codetool.WithReasoningSandwichConfig(sandwichCfg))
	}
	if runner, err := maybeCodeRunner(cfg); err != nil {
		cfg.CodeModeStatus = "unavailable"
		cfg.CodeModeError = err.Error()
	} else if runner != nil {
		toolOpts = append(toolOpts, codetool.WithCodeMode(runner))
		cfg.CodeModeStatus = "on"
	}

	opts := codetool.AgentOptions(cfg.WorkingDir, toolOpts...)
	opts = append(opts,
		core.WithSystemPrompt[string](strings.TrimSpace(vesselSystemPrompt)),
		core.WithDynamicSystemPrompt[string](func(_ context.Context, _ *core.RunContext) (string, error) {
			return buildRuntimePrompt(cfg, activeSkills), nil
		}),
	)

	if cfg.TopLevelPersonality {
		personalityGen := modelutil.CachedPersonalityGenerator(modelutil.GeneratePersonality(model))
		opts = append(opts, core.WithDynamicSystemPrompt[string](func(ctx context.Context, rc *core.RunContext) (string, error) {
			if rc == nil || strings.TrimSpace(rc.Prompt) == "" {
				return "", nil
			}
			generated, err := personalityGen(ctx, modelutil.PersonalityRequest{
				Task:       rc.Prompt,
				Role:       "terminal coding agent and Gollem/Vessel showcase",
				BasePrompt: "Prioritize verifier-defined success criteria, decisive execution, and visible progress through planning, invariants, and verification.",
				Context: map[string]string{
					"provider": string(cfg.Provider),
					"model":    cfg.Model,
					"workdir":  cfg.ShortDir(),
				},
			})
			if err != nil {
				return "", nil
			}
			return generated, nil
		}))
	}

	if len(activeSkills) > 0 {
		prompt := buildSkillsPrompt(activeSkills)
		opts = append(opts, core.WithDynamicSystemPrompt[string](
			func(_ context.Context, _ *core.RunContext) (string, error) {
				return prompt, nil
			},
		))
	}

	switch cfg.Provider {
	case config.ProviderAnthropic, config.ProviderVertexAnthropic, config.ProviderVertexAI:
		if cfg.ThinkingBudget > 0 {
			maxTokens := cfg.ThinkingBudget + 16000
			opts = append(opts,
				core.WithThinkingBudget[string](cfg.ThinkingBudget),
				core.WithMaxTokens[string](maxTokens),
			)
		}
	case config.ProviderOpenAI, config.ProviderOpenAICompatible:
		if cfg.ReasoningEffort != "" {
			opts = append(opts, core.WithReasoningEffort[string](cfg.ReasoningEffort))
			if strings.Contains(strings.ToLower(cfg.Model), "codex") {
				opts = append(opts,
					core.WithMaxTokens[string](50000),
					core.WithTemperature[string](0),
				)
			}
		}
	}

	opts = append(opts, extra...)
	return core.NewAgent[string](model, opts...), nil
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

func buildRuntimePrompt(cfg *config.Config, activeSkills []skills.Skill) string {
	var b strings.Builder
	b.WriteString("# Vessel Runtime Profile\n")
	b.WriteString("You are running inside Vessel, the flagship TUI for the fugue-labs/gollem framework. Demonstrate a world-class coding workflow that can compete with Claude Code, Gemini, and Codex by fully using Gollem's planning, invariants, delegation, verification, auto-context, and code-execution architecture.\n\n")
	b.WriteString("## Effective runtime\n")
	fmt.Fprintf(&b, "- provider/model: %s/%s\n", cfg.Provider, cfg.Model)
	fmt.Fprintf(&b, "- timeout: %s\n", cfg.Timeout)
	fmt.Fprintf(&b, "- team mode: %s", cfg.TeamMode)
	if cfg.TeamModeReason != "" {
		fmt.Fprintf(&b, " (%s)", cfg.TeamModeReason)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "- delegate: %s\n", onOff(!cfg.DisableDelegate))
	fmt.Fprintf(&b, "- code mode: %s\n", cfg.CodeModeStatus)
	if cfg.CodeModeError != "" {
		fmt.Fprintf(&b, "- code mode note: %s\n", cfg.CodeModeError)
	}
	if cfg.ReasoningEffort != "" {
		fmt.Fprintf(&b, "- reasoning effort: %s\n", cfg.ReasoningEffort)
	}
	if cfg.ThinkingBudget > 0 {
		fmt.Fprintf(&b, "- thinking budget: %d\n", cfg.ThinkingBudget)
	}
	if cfg.AutoContextMaxTokens > 0 {
		fmt.Fprintf(&b, "- auto-context: %d tokens, keep last %d turns\n", cfg.AutoContextMaxTokens, cfg.AutoContextKeepLastN)
	}
	fmt.Fprintf(&b, "- top-level personality: %s\n", onOff(cfg.TopLevelPersonality))
	if len(activeSkills) > 0 {
		b.WriteString("- active skills: ")
		for i, s := range activeSkills {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(s.Name)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n## Showcase expectations\n")
	b.WriteString("- For non-trivial work, create and maintain a plan early, then keep it accurate.\n")
	b.WriteString("- Use invariants as hard completion gates; summarize them before finishing.\n")
	b.WriteString("- Use delegate/team capabilities for parallel-safe research or isolated focused subtasks when that will reduce wall-clock time.\n")
	b.WriteString("- Use execute_code/code mode for batched analysis or transformations when it saves API turns.\n")
	b.WriteString("- If context recovery appears, immediately re-anchor on the task, call planning get when needed, and continue decisively.\n")
	return b.String()
}

func applyRuntimeProfile(cfg *config.Config, prompt string) {
	cfg.CodeModeError = ""
	if cfg.DisableCodeMode {
		cfg.CodeModeStatus = "off"
	} else {
		cfg.CodeModeStatus = "pending"
	}
	cfg.EffectiveTeamMode, cfg.TeamModeReason = decideTeamMode(cfg, prompt)
}

func decideTeamMode(cfg *config.Config, prompt string) (bool, string) {
	if cfg.DisableDelegate {
		return false, "delegate disabled"
	}
	switch cfg.TeamMode {
	case "on":
		return true, "forced on"
	case "off":
		return false, "forced off"
	}
	lower := strings.ToLower(prompt)
	score := 0
	if len(prompt) >= 320 {
		score++
	}
	if strings.Count(prompt, "\n") >= 6 {
		score++
	}
	for _, hint := range []string{
		"parallel", "multiple files", "large codebase", "end-to-end", "architecture",
		"refactor", "migrate", "investigate", "research", "benchmark", "compare",
		"full stack", "across the repo", "multi-step", "planner", "team",
	} {
		if strings.Contains(lower, hint) {
			score++
		}
	}
	if score >= 3 {
		return true, fmt.Sprintf("auto heuristic score=%d", score)
	}
	return false, fmt.Sprintf("auto heuristic score=%d", score)
}

func reasoningSandwichConfig(cfg *config.Config) (codetool.ReasoningSandwichConfig, bool) {
	if cfg.ReasoningEffort != "" {
		return codetool.ReasoningSandwichConfigForMaxEffort(cfg.ReasoningEffort), true
	}
	if cfg.ThinkingBudget > 0 {
		cfgOut := codetool.DefaultReasoningSandwichConfig()
		cfgOut.Planning.ThinkingBudget = cfg.ThinkingBudget
		cfgOut.Verification.ThinkingBudget = cfg.ThinkingBudget
		cfgOut.Implementation.ThinkingBudget = max(8000, cfg.ThinkingBudget/2)
		return cfgOut, true
	}
	return codetool.ReasoningSandwichConfig{}, false
}

func maybeCodeRunner(cfg *config.Config) (*montygo.Runner, error) {
	if cfg.DisableCodeMode {
		return nil, nil
	}
	codeRunnerOnce.Do(func() {
		codeRunner, codeRunnerErr = montygo.New()
	})
	return codeRunner, codeRunnerErr
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
