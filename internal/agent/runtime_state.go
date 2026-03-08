package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/codetool"
)

// RuntimeState captures mutable runtime-only execution decisions derived for a
// specific run. It is kept separate from config.Config so UI/runtime reporting
// does not require mutating user configuration.
type RuntimeState struct {
	EffectiveTeamMode bool
	TeamModeReason    string
	CodeModeStatus    string
	CodeModeError     string

	// Session holds the persistent session handle for interactive TUIs.
	// Call Session.Cleanup() when the session ends (e.g., /clear).
	Session *codetool.Session
}

// InitialRuntimeState returns the baseline runtime state before an agent run is
// constructed. Forced team-mode settings are still reflected here.
func InitialRuntimeState(cfg *config.Config) RuntimeState {
	return buildRuntimeState(context.Background(), cfg, "", nil)
}

// PrepareRuntime resolves the effective runtime for a specific prompt without
// constructing the coding agent yet. This keeps prompt routing cheap to test
// and lets UIs classify asynchronously before creating/reusing an agent.
func PrepareRuntime(ctx context.Context, cfg *config.Config, prompt string) (RuntimeState, error) {
	state := baselineRuntimeState(cfg)

	model, err := createModel(cfg)
	if err != nil {
		return state, fmt.Errorf("creating model: %w", err)
	}

	routerModel := model
	routerFallback := ""
	if resolved, err := resolveRouterModelFunc(cfg, model); err != nil {
		routerFallback = fmt.Sprintf(" router_fallback=%q", compactError(err, 120))
	} else if resolved != nil {
		routerModel = resolved
	}

	state = buildRuntimeState(ctx, cfg, prompt, routerModel)
	if routerFallback != "" && strings.HasPrefix(state.TeamModeReason, "auto router") {
		state.TeamModeReason += routerFallback
	}
	return state, nil
}

func baselineRuntimeState(cfg *config.Config) RuntimeState {
	state := RuntimeState{}
	if cfg.DisableCodeMode {
		state.CodeModeStatus = "off"
	} else {
		state.CodeModeStatus = "pending"
	}
	return state
}

func buildRuntimeState(ctx context.Context, cfg *config.Config, prompt string, routerModel core.Model) RuntimeState {
	state := baselineRuntimeState(cfg)
	state.EffectiveTeamMode, state.TeamModeReason = decideTeamMode(ctx, cfg, prompt, routerModel)
	return state
}
