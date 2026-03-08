package agent

import (
	"github.com/fugue-labs/golem/internal/config"
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
	return buildRuntimeState(cfg, "")
}

func buildRuntimeState(cfg *config.Config, prompt string) RuntimeState {
	state := RuntimeState{}
	if cfg.DisableCodeMode {
		state.CodeModeStatus = "off"
	} else {
		state.CodeModeStatus = "pending"
	}
	state.EffectiveTeamMode, state.TeamModeReason = decideTeamMode(cfg, prompt)
	return state
}
