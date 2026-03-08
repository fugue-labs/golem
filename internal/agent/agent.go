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
var golemSystemPrompt string

var (
	codeRunnerOnce sync.Once
	codeRunner     *montygo.Runner
	codeRunnerErr  error
)

// thinkingBudgetResponsePaddingTokens reserves room for the model's non-thinking
// response and tool traffic beyond the explicit thinking budget.
const thinkingBudgetResponsePaddingTokens = 16000

// New creates a configured coding agent using Gollem's full codetool stack,
// plus Golem-specific runtime guidance for the consumer-grade TUI.
func New(cfg *config.Config, runPrompt string, activeSkills []skills.Skill, extra ...core.AgentOption[string]) (*core.Agent[string], RuntimeState, error) {
	runtime, err := PrepareRuntime(context.Background(), cfg, runPrompt)
	if err != nil {
		return nil, InitialRuntimeState(cfg), err
	}
	a, err := NewWithRuntime(cfg, &runtime, activeSkills, extra...)
	if err != nil {
		return nil, runtime, err
	}
	return a, runtime, nil
}

// NewWithRuntime constructs a coding agent from a precomputed runtime decision.
func NewWithRuntime(cfg *config.Config, runtime *RuntimeState, activeSkills []skills.Skill, extra ...core.AgentOption[string]) (*core.Agent[string], error) {
	if runtime == nil {
		state := InitialRuntimeState(cfg)
		runtime = &state
	}

	model, err := createModel(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating model: %w", err)
	}

	session := runtime.Session
	if session == nil {
		session = &codetool.Session{}
		runtime.Session = session
	}

	toolOpts := []codetool.Option{
		codetool.WithModel(model),
		codetool.WithTimeout(cfg.Timeout),
		codetool.WithPersistentSession(session),
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
	if runtime.EffectiveTeamMode {
		toolOpts = append(toolOpts, codetool.WithTeamMode())
	}
	if cfg.DisableGreedyThinkingPressure {
		toolOpts = append(toolOpts, codetool.WithDisableGreedyThinkingPressure())
	}
	if sandwichCfg, ok := reasoningSandwichConfig(cfg); ok {
		toolOpts = append(toolOpts, codetool.WithReasoningSandwichConfig(sandwichCfg))
	}
	if runner, err := maybeCodeRunner(cfg); err != nil {
		runtime.CodeModeStatus = "unavailable"
		runtime.CodeModeError = err.Error()
	} else if runner != nil {
		toolOpts = append(toolOpts, codetool.WithCodeMode(runner))
		runtime.CodeModeStatus = "on"
	}

	opts := codetool.AgentOptions(cfg.WorkingDir, toolOpts...)
	opts = append(opts,
		core.WithSystemPrompt[string](strings.TrimSpace(golemSystemPrompt)),
		core.WithDynamicSystemPrompt[string](func(_ context.Context, _ *core.RunContext) (string, error) {
			return buildRuntimePrompt(cfg, *runtime, activeSkills), nil
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
				Role:       "expert terminal coding agent",
				BasePrompt: "Decisive execution, explicit planning, disciplined verification, and effective delegation.",
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

	switch cfg.Provider {
	case config.ProviderAnthropic, config.ProviderVertexAnthropic, config.ProviderVertexAI:
		if cfg.ThinkingBudget > 0 {
			maxTokens := cfg.ThinkingBudget + thinkingBudgetResponsePaddingTokens
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

func buildRuntimePrompt(cfg *config.Config, runtime RuntimeState, activeSkills []skills.Skill) string {
	var b strings.Builder
	b.WriteString("# Golem Runtime Profile\n\n")
	b.WriteString("## Effective runtime\n")
	fmt.Fprintf(&b, "- provider/model: %s/%s\n", cfg.Provider, cfg.Model)
	if cfg.RouterModel != "" {
		fmt.Fprintf(&b, "- router model: %s\n", cfg.RouterModel)
	}
	fmt.Fprintf(&b, "- timeout: %s\n", cfg.Timeout)
	fmt.Fprintf(&b, "- team mode: %s (effective: %s)\n", cfg.TeamMode, onOff(runtime.EffectiveTeamMode))
	if runtime.TeamModeReason != "" {
		fmt.Fprintf(&b, "- team mode note: %s\n", runtime.TeamModeReason)
	}
	fmt.Fprintf(&b, "- delegate: %s\n", onOff(!cfg.DisableDelegate))
	fmt.Fprintf(&b, "- code mode: %s\n", runtime.CodeModeStatus)
	if runtime.CodeModeError != "" {
		fmt.Fprintf(&b, "- code mode note: %s\n", runtime.CodeModeError)
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
		b.WriteString("\n## Active skills\n")
		for _, s := range activeSkills {
			fmt.Fprintf(&b, "\n### %s\n", s.Name)
			if content := strings.TrimSpace(s.Content); content != "" {
				b.WriteString(content)
				b.WriteString("\n")
			}
		}
	}
	return b.String()
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

func createModel(cfg *config.Config) (core.Model, error) {
	return createModelWithName(cfg, cfg.Model)
}

func createModelWithName(cfg *config.Config, modelName string) (core.Model, error) {
	switch cfg.Provider {
	case config.ProviderAnthropic:
		opts := []anthropic.Option{
			anthropic.WithModel(modelName),
		}
		if cfg.APIKey != "" {
			opts = append(opts, anthropic.WithAPIKey(cfg.APIKey))
		}
		return anthropic.New(opts...), nil

	case config.ProviderOpenAI:
		opts := []openai.Option{
			openai.WithModel(modelName),
			openai.WithMaxTokens(128000),
			openai.WithPromptCacheRetention("24h"),
			openai.WithPromptCacheKey("golem"),
		}
		if cfg.APIKey != "" {
			opts = append(opts, openai.WithAPIKey(cfg.APIKey))
		}
		if cfg.BaseURL != "" {
			opts = append(opts,
				openai.WithBaseURL(cfg.BaseURL),
				openai.WithTransport("http"),
			)
		} else {
			opts = append(opts,
				openai.WithTransport("websocket"),
				openai.WithWebSocketHTTPFallback(true),
			)
		}
		return openai.New(opts...), nil

	case config.ProviderOpenAICompatible:
		opts := []openai.Option{
			openai.WithModel(modelName),
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
			vertexai.WithModel(modelName),
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
			vertexanthropic.WithModel(modelName),
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
