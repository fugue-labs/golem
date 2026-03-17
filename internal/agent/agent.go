package agent

import (
	"context"
	_ "embed"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	montygo "github.com/fugue-labs/monty-go"

	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/skills"
	openaiauth "github.com/fugue-labs/gollem/auth/openai"
	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/codetool"
	"github.com/fugue-labs/gollem/ext/memory"
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

	// Use the routed model if model routing selected a different model for this turn.
	modelName := cfg.Model
	if runtime.RoutedModel != "" && runtime.RoutedModel != cfg.Model {
		modelName = runtime.RoutedModel
	}
	model, err := createModelWithName(cfg, modelName)
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
	// FetchURL is opt-in via GOLEM_ENABLE_FETCH_URL.
	if cfg.EnableFetchURL {
		toolOpts = append(toolOpts, codetool.WithFetchURL(defaultFetchURL()))
	}
	if runtime.EffectiveTeamMode && runtime.AskUserFunc != nil {
		toolOpts = append(toolOpts, codetool.WithAskUser(runtime.AskUserFunc))
	}
	if sandwichCfg, ok := reasoningSandwichConfig(cfg); ok {
		toolOpts = append(toolOpts, codetool.WithReasoningSandwichConfig(sandwichCfg))
	}
	if runner, err := maybeCodeRunner(cfg); err == nil && runner != nil {
		toolOpts = append(toolOpts, codetool.WithCodeMode(runner))
	}

	opts := codetool.AgentOptions(cfg.WorkingDir, toolOpts...)

	// In benchmark mode, overlay the base system prompt with aggressive directives.
	systemPrompt := strings.TrimSpace(golemSystemPrompt)
	if cfg.BenchmarkMode {
		systemPrompt += "\n\n" +
			"<benchmark_mode>\n" +
			"You are in benchmark mode. Act fast — a rough attempt you iterate on beats extended planning. " +
			"Minimize reading, maximize action. Write code based on what you already know and iterate.\n" +
			"</benchmark_mode>"
	}

	opts = append(opts,
		core.WithSystemPrompt[string](systemPrompt),
		core.WithDynamicSystemPrompt[string](func(_ context.Context, _ *core.RunContext) (string, error) {
			return buildRuntimePrompt(cfg, *runtime, activeSkills), nil
		}),
		// Token efficiency defaults: truncate large tool outputs and clean history.
		core.WithToolOutputTruncation[string](core.DefaultTruncationConfig()),
		core.WithHistoryProcessor[string](core.NormalizeHistory()),
	)

	// Register MCP tools if available.
	if len(runtime.MCPTools) > 0 {
		opts = append(opts, core.WithTools[string](runtime.MCPTools...))
	}

	// Wire persistent memory: tool for agent access + knowledge base for context injection.
	if runtime.MemoryStore != nil {
		namespace := []string{"golem", projectHash(cfg.WorkingDir)}
		memTool := memory.MemoryTool(runtime.MemoryStore, namespace...)
		memKB := memory.StoreKnowledgeBase(runtime.MemoryStore, namespace...)
		opts = append(opts, core.WithTools[string](memTool))
		opts = append(opts, core.WithKnowledgeBase[string](memKB))
	}

	// Register user-defined shell hooks.
	if hooksCfg := LoadHooksConfig(); len(hooksCfg.PreToolUse) > 0 || len(hooksCfg.PostToolUse) > 0 {
		opts = append(opts, core.WithHooks[string](BuildHook(hooksCfg, cfg.WorkingDir)))
	}

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
			if err == nil {
				return generated, nil
			}
			return "", nil
		}))
	}

	// Apply model-specific options, adjusting for routed fast models.
	isFastModel := runtime.RoutedModelTier == TierFast && modelName != cfg.Model
	switch cfg.Provider {
	case config.ProviderAnthropic, config.ProviderVertexAnthropic, config.ProviderVertexAI:
		if cfg.ThinkingBudget > 0 && !isFastModel {
			maxTokens := cfg.ThinkingBudget + thinkingBudgetResponsePaddingTokens
			opts = append(opts,
				core.WithThinkingBudget[string](cfg.ThinkingBudget),
				core.WithMaxTokens[string](maxTokens),
			)
		}
	case config.ProviderOpenAI, config.ProviderOpenAICompatible:
		if cfg.ReasoningEffort != "" && !isFastModel {
			opts = append(opts, core.WithReasoningEffort[string](cfg.ReasoningEffort))
			if strings.Contains(strings.ToLower(modelName), "codex") {
				opts = append(opts,
					core.WithMaxTokens[string](50000),
					core.WithTemperature[string](0),
				)
			}
		}
	}

	// Filter disabled tools from the agent's tool surface.
	if disabled := buildDisabledToolSet(cfg); len(disabled) > 0 {
		opts = append(opts, core.WithToolsPrepare[string](
			func(_ context.Context, _ *core.RunContext, tools []core.ToolDefinition) []core.ToolDefinition {
				filtered := make([]core.ToolDefinition, 0, len(tools))
				for _, t := range tools {
					if !disabled[t.Name] {
						filtered = append(filtered, t)
					}
				}
				return filtered
			},
		))
	}

	opts = append(opts, extra...)
	return core.NewAgent[string](model, opts...), nil
}

// buildDisabledToolSet combines config.DisabledTools with individual disable
// flags into a single set of tool names to exclude.
func buildDisabledToolSet(cfg *config.Config) map[string]bool {
	disabled := make(map[string]bool)
	for name := range cfg.DisabledTools {
		disabled[name] = true
	}
	// Individual flags override the disabled set.
	if cfg.DisableDelegate {
		disabled["delegate"] = true
	}
	if cfg.DisableCodeMode {
		disabled["execute_code"] = true
	}
	if !cfg.EnableFetchURL {
		disabled["fetch_url"] = true
	}
	return disabled
}

func buildRuntimePrompt(cfg *config.Config, runtime RuntimeState, activeSkills []skills.Skill) string {
	var b strings.Builder
	b.WriteString(RenderRuntimePrompt(BuildRuntimeReport(cfg, runtime, cfg.Validate(), nil)))

	// Project instructions from GOLEM.md / CLAUDE.md files.
	if instructions := FormatInstructions(runtime.Instructions); instructions != "" {
		b.WriteString("\n\n")
		b.WriteString(instructions)
	}

	// Git context.
	if gitCtx := FormatGitContext(runtime.Git); gitCtx != "" {
		b.WriteString("\n\n")
		b.WriteString(gitCtx)
	}

	if len(activeSkills) > 0 {
		names := make([]string, len(activeSkills))
		for i, s := range activeSkills {
			names[i] = s.Name
		}
		fmt.Fprintf(&b, "\nSkills: %s\n", strings.Join(names, ", "))
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

var (
	htmlTagPattern = regexp.MustCompile(`(?s)<[^>]+>`)
	htmlWhitespace = regexp.MustCompile(`\s+`)
	htmlScriptBlock = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	htmlStyleBlock  = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	htmlHeadBlock   = regexp.MustCompile(`(?is)<head\b[^>]*>.*?</head>`)
)

func defaultFetchURL() codetool.FetchURLFunc {
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return codetool.ValidateFetchURLSafety(req.URL.String())
		},
	}
	return func(ctx context.Context, rawURL string) (string, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", "Golem/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 200*1024))
		if err != nil {
			return "", err
		}
		content := stripHTML(string(body))
		if strings.TrimSpace(content) == "" {
			content = strings.TrimSpace(string(body))
		}
		return content, nil
	}
}

func stripHTML(raw string) string {
	cleaned := htmlScriptBlock.ReplaceAllString(raw, " ")
	cleaned = htmlStyleBlock.ReplaceAllString(cleaned, " ")
	cleaned = htmlHeadBlock.ReplaceAllString(cleaned, " ")
	cleaned = htmlTagPattern.ReplaceAllString(cleaned, " ")
	cleaned = html.UnescapeString(cleaned)
	cleaned = strings.ReplaceAll(cleaned, "\u00a0", " ")
	cleaned = htmlWhitespace.ReplaceAllString(cleaned, " ")
	return strings.TrimSpace(cleaned)
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

// chatgptTokenRefresher returns a TokenRefresher that auto-refreshes ChatGPT
// OAuth tokens and persists the updated credentials to disk. The refresher is
// synchronized with a mutex since it may be called from concurrent sessions.
func chatgptTokenRefresher(creds *openaiauth.Credentials) openai.TokenRefresher {
	var mu sync.Mutex
	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		refreshed, err := openaiauth.RefreshIfNeeded(creds)
		if err != nil {
			// Return the current token so the request can proceed (it may
			// still be valid even if refresh failed).
			return creds.AccessToken, nil
		}
		if refreshed.AccessToken != creds.AccessToken {
			// Token was refreshed; update shared credentials and persist.
			*creds = *refreshed
			_ = openaiauth.SaveCredentials(creds)
		}
		return creds.AccessToken, nil
	}
}

// CreateModel creates a gollem Model from the current configuration.
// Exported for use by TUI commands (e.g., /compact) that need a model
// outside of agent construction.
func CreateModel(cfg *config.Config) (core.Model, error) {
	return createModelWithName(cfg, cfg.Model)
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
		}
		if cfg.ChatGPTCreds != nil {
			opts = append(opts, openai.WithChatGPTAuth(cfg.ChatGPTCreds.AccessToken, cfg.ChatGPTCreds.AccountID))
			opts = append(opts, openai.WithTokenRefresher(chatgptTokenRefresher(cfg.ChatGPTCreds)))
			// ChatGPT backend uses HTTP (not WebSocket).
			opts = append(opts, openai.WithTransport("http"))
		} else if cfg.APIKey != "" {
			opts = append(opts, openai.WithAPIKey(cfg.APIKey))
			opts = append(opts,
				openai.WithPromptCacheRetention("24h"),
				openai.WithPromptCacheKey("golem"),
			)
		}
		if cfg.BaseURL != "" {
			opts = append(opts,
				openai.WithBaseURL(cfg.BaseURL),
				openai.WithTransport("http"),
			)
		} else if cfg.ChatGPTCreds == nil {
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
