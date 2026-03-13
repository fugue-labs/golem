package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/codetool"
	"github.com/fugue-labs/gollem/ext/mcp"
	"github.com/fugue-labs/gollem/ext/memory"
	"github.com/fugue-labs/gollem/modelutil"
)

// RuntimeState captures mutable runtime-only execution decisions derived for a
// specific run. It is kept separate from config.Config so UI/runtime reporting
// does not require mutating user configuration.
type RuntimeState struct {
	EffectiveTeamMode bool
	TeamModeReason    string
	RouterModelName   string
	CodeModeStatus    string
	CodeModeError     string
	OpenImageStatus   string
	WebSearchStatus   string
	FetchURLStatus    string
	AskUserStatus     string
	AskUserFunc       codetool.AskUserFunc

	// Project instructions discovered from GOLEM.md / CLAUDE.md files.
	Instructions []InstructionFile

	// Git context gathered from the working directory.
	Git *GitInfo

	// MCP server state.
	MCPManager    *mcp.Manager
	MCPTools      []core.Tool
	MCPServers    []string // connected server names
	MCPStatus     string   // "off", "on", "error"

	// Memory store for persistent project-scoped memories.
	MemoryStore memory.Store

	// Session holds the persistent session handle for interactive TUIs.
	// Call Session.Cleanup() when the session ends (e.g., /clear).
	Session *codetool.Session

	// EventBus publishes team lifecycle events to the TUI. When non-nil,
	// it is passed to the codetool layer so team events (spawn, idle,
	// terminated, messages) are observable by subscribers.
	EventBus *core.EventBus
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
	openImageStatus := "off"
	if modelutil.GetProfile(model).SupportsVision {
		openImageStatus = "on"
	}

	routerModel := model
	routerFallback := ""
	if resolved, err := resolveRouterModelFunc(cfg, model); err != nil {
		routerFallback = fmt.Sprintf(" router_fallback=%q", compactError(err, 120))
	} else if resolved != nil {
		routerModel = resolved
	}

	state = buildRuntimeState(ctx, cfg, prompt, routerModel)
	state.CodeModeStatus, state.CodeModeError = resolveCodeModeStatus(cfg)
	state.OpenImageStatus = openImageStatus
	if routerFallback != "" && strings.HasPrefix(state.TeamModeReason, "auto router") {
		state.TeamModeReason += routerFallback
	}

	// Discover project instructions.
	state.Instructions = DiscoverInstructions(cfg.WorkingDir)

	// Gather git context.
	state.Git = GatherGitInfo(cfg.WorkingDir)

	// Set up persistent memory store.
	if memStore, _, _, memErr := SetupMemory(cfg.WorkingDir); memErr == nil {
		state.MemoryStore = memStore
	}

	// Connect MCP servers.
	mcpCfg, err := LoadMCPConfig()
	if err == nil && len(mcpCfg.Servers) > 0 {
		mgr, tools, servers, mcpErr := ConnectMCPServers(ctx, mcpCfg)
		if mcpErr != nil {
			state.MCPStatus = "error"
		} else {
			state.MCPManager = mgr
			state.MCPTools = tools
			state.MCPServers = servers
			state.MCPStatus = "on"
		}
	} else {
		state.MCPStatus = "off"
	}

	return state, nil
}

func baselineRuntimeState(cfg *config.Config) RuntimeState {
	state := RuntimeState{}
	if cfg != nil {
		state.RouterModelName = strings.TrimSpace(cfg.RouterModel)
		if state.RouterModelName == "" {
			state.RouterModelName = strings.TrimSpace(cfg.Model)
		}
	}
	if cfg != nil && cfg.DisableCodeMode {
		state.CodeModeStatus = "off"
	} else {
		state.CodeModeStatus = "pending"
	}
	state.OpenImageStatus = "pending"
	state.WebSearchStatus = "off"
	state.FetchURLStatus = "on"
	state.AskUserStatus = "off"
	if cfg != nil && cfg.TeamMode != "off" {
		state.AskUserStatus = "pending"
	}
	return state
}

func buildRuntimeState(ctx context.Context, cfg *config.Config, prompt string, routerModel core.Model) RuntimeState {
	state := baselineRuntimeState(cfg)
	if routerModel != nil {
		state.RouterModelName = strings.TrimSpace(routerModel.ModelName())
	}
	state.EffectiveTeamMode, state.TeamModeReason = decideTeamMode(ctx, cfg, prompt, routerModel)
	if cfg != nil && cfg.TeamMode == "auto" && strings.TrimSpace(prompt) == "" {
		state.AskUserStatus = "pending"
	} else {
		state.AskUserStatus = onOff(state.EffectiveTeamMode)
	}
	return state
}

func resolveCodeModeStatus(cfg *config.Config) (string, string) {
	if cfg == nil || cfg.DisableCodeMode {
		return "off", ""
	}
	runner, err := maybeCodeRunner(cfg)
	if err != nil {
		return "unavailable", err.Error()
	}
	if runner != nil {
		return "on", ""
	}
	return "off", ""
}
