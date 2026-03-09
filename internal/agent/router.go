package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/gollem/core"
)

const routerSystemPrompt = `You are a routing classifier for a terminal coding agent.
Decide whether the user's latest request is simple or complex, and whether team mode (delegating to sub-agents) is worth the overhead.
Prefer simple / should_enable_team_mode=false for straightforward questions, codebase lookups, single-file edits, narrow bug fixes, and other work that one agent can handle directly.
Set complexity=complex only when the request is meaningfully multi-step, broad, or coordination-heavy.
Return only the structured result.`

// promptRoute is the initial structured router surface.
// Future capabilities can extend this with additional prompt-level signals such as
// memory_candidate, but keep the first version narrowly scoped to complexity/team mode.
type promptRoute struct {
	Complexity           string `json:"complexity" jsonschema:"description=Task complexity classification,enum=simple,enum=complex"`
	ShouldEnableTeamMode bool   `json:"should_enable_team_mode" jsonschema:"description=Set true only when team mode delegation is worth the overhead for this request."`
	Confidence           string `json:"confidence" jsonschema:"description=Classifier confidence,enum=low,enum=medium,enum=high"`
	Summary              string `json:"summary" jsonschema:"description=Brief explanation under 120 characters."`
}

var (
	promptRouterFunc       = runPromptRouter
	resolveRouterModelFunc = resolveRouterModel
)

func resolveRouterModel(cfg *config.Config, activeModel core.Model) (core.Model, error) {
	if activeModel == nil {
		return nil, nil
	}
	routerModel := strings.TrimSpace(cfg.RouterModel)
	if routerModel == "" || routerModel == strings.TrimSpace(cfg.Model) {
		return activeModel, nil
	}
	return createModelWithName(cfg, routerModel)
}

func decideTeamMode(ctx context.Context, cfg *config.Config, prompt string, routerModel core.Model) (bool, string) {
	if cfg.DisableDelegate {
		return false, "delegate disabled"
	}
	switch cfg.TeamMode {
	case "on":
		return true, "forced on"
	case "off":
		return false, "forced off"
	}
	if strings.TrimSpace(prompt) == "" {
		return false, "auto router pending"
	}
	if routerModel == nil {
		return false, "auto router unavailable: model unavailable"
	}

	route, err := promptRouterFunc(ctx, cfg, routerModel, prompt)
	if err != nil {
		return false, "auto router unavailable: " + compactError(err, 160)
	}

	reason := fmt.Sprintf(
		"auto router model=%s complexity=%s confidence=%s",
		routerModel.ModelName(),
		route.Complexity,
		route.Confidence,
	)
	if route.Summary != "" {
		reason += fmt.Sprintf(" summary=%q", route.Summary)
	}
	return route.ShouldEnableTeamMode, reason
}

func runPromptRouter(ctx context.Context, cfg *config.Config, model core.Model, prompt string) (promptRoute, error) {
	var zero promptRoute
	if model == nil {
		return zero, errors.New("model unavailable")
	}

	classifierCtx, cancel := context.WithTimeout(ctx, routerTimeout(cfg))
	defer cancel()

	opts := []core.AgentOption[promptRoute]{
		core.WithSystemPrompt[promptRoute](routerSystemPrompt),
		core.WithMaxRetries[promptRoute](0),
		core.WithUsageLimits[promptRoute](core.UsageLimits{RequestLimit: core.IntPtr(1)}),
		core.WithMaxTokens[promptRoute](400),
		core.WithTemperature[promptRoute](0),
	}
	if supportsNativeRouterOutput(cfg) {
		opts = append(opts, core.WithOutputOptions[promptRoute](core.WithOutputMode(core.OutputModeNative)))
	}
	if cfg.Provider == config.ProviderOpenAI {
		opts = append(opts, core.WithReasoningEffort[promptRoute]("low"))
	}

	agent := core.NewAgent[promptRoute](model, opts...)
	result, err := agent.Run(classifierCtx, buildPromptRouterPrompt(prompt))
	if err != nil {
		return zero, err
	}
	return sanitizePromptRoute(result.Output), nil
}

func sanitizePromptRoute(route promptRoute) promptRoute {
	route.Complexity = strings.ToLower(strings.TrimSpace(route.Complexity))
	switch route.Complexity {
	case "complex", "simple":
	default:
		route.Complexity = "simple"
	}

	route.Confidence = strings.ToLower(strings.TrimSpace(route.Confidence))
	switch route.Confidence {
	case "high", "medium", "low":
	default:
		route.Confidence = "low"
	}

	route.Summary = strings.Join(strings.Fields(strings.TrimSpace(route.Summary)), " ")
	if len(route.Summary) > 120 {
		route.Summary = route.Summary[:120]
	}
	return route
}

func buildPromptRouterPrompt(prompt string) string {
	return fmt.Sprintf(
		"Classify the user's latest request for orchestration routing.\n\n"+
			"Set should_enable_team_mode=true only when delegation to sub-agents is likely worth the coordination overhead.\n\n"+
			"User request:\n%s",
		trimForRouter(prompt, 4000),
	)
}

func supportsNativeRouterOutput(cfg *config.Config) bool {
	switch cfg.Provider {
	case config.ProviderVertexAI:
		return true
	default:
		return false
	}
}

func routerTimeout(cfg *config.Config) time.Duration {
	const (
		maxRouterTimeout = 20 * time.Second
		minRouterTimeout = 2 * time.Second
	)
	if cfg == nil || cfg.Timeout <= 0 {
		return maxRouterTimeout
	}

	timeout := cfg.Timeout / 4
	if timeout <= 0 {
		timeout = cfg.Timeout
	}
	if timeout > maxRouterTimeout {
		return maxRouterTimeout
	}
	if timeout < minRouterTimeout {
		if cfg.Timeout < minRouterTimeout {
			return cfg.Timeout
		}
		return minRouterTimeout
	}
	return timeout
}

func trimForRouter(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}

func compactError(err error, max int) string {
	if err == nil {
		return ""
	}
	s := strings.Join(strings.Fields(strings.TrimSpace(err.Error())), " ")
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}
