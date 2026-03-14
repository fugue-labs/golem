package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/eval"
	"github.com/fugue-labs/golem/internal/mission"
	"github.com/fugue-labs/golem/internal/skills"
	"github.com/fugue-labs/golem/internal/ui/chat"
	"github.com/fugue-labs/golem/internal/ui/checkpoint"
	uiinvariants "github.com/fugue-labs/golem/internal/ui/invariants"
	"github.com/fugue-labs/golem/internal/ui/plan"
	"github.com/fugue-labs/golem/internal/ui/spec"
	"github.com/fugue-labs/golem/internal/ui/styles"
	uiverification "github.com/fugue-labs/golem/internal/ui/verification"
	"github.com/fugue-labs/golem/internal/ui/watcher"
	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/codetool"
	"github.com/fugue-labs/gollem/ext/deep"
	"github.com/fugue-labs/gollem/ext/team"
)

// isContextCanceled checks whether err represents a context cancellation.
// It checks errors.Is first, then falls back to a string match because some
// HTTP transports (and the OpenAI SSE reader) may wrap context.Canceled in a
// way that breaks the error chain (e.g., using %v instead of %w).
func isContextCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "context canceled")
}

// modelPricing returns known model pricing for cost estimation.
// Rates are cost-per-token (e.g., $3/1M input = 0.000003).
func modelPricing() map[string]core.ModelPricing {
	return map[string]core.ModelPricing{
		// Anthropic — Claude 4.x family
		"claude-sonnet-4-20250514": {InputTokenCost: 0.000003, OutputTokenCost: 0.000015, CachedInputCost: 0.0000003, CacheWriteCost: 0.00000375},
		"claude-sonnet-4":          {InputTokenCost: 0.000003, OutputTokenCost: 0.000015, CachedInputCost: 0.0000003, CacheWriteCost: 0.00000375},
		"claude-sonnet-4-6":        {InputTokenCost: 0.000003, OutputTokenCost: 0.000015, CachedInputCost: 0.0000003, CacheWriteCost: 0.00000375},
		"claude-opus-4-20250514":   {InputTokenCost: 0.000015, OutputTokenCost: 0.000075, CachedInputCost: 0.0000015, CacheWriteCost: 0.00001875},
		"claude-opus-4":            {InputTokenCost: 0.000015, OutputTokenCost: 0.000075, CachedInputCost: 0.0000015, CacheWriteCost: 0.00001875},
		"claude-opus-4-6":          {InputTokenCost: 0.000015, OutputTokenCost: 0.000075, CachedInputCost: 0.0000015, CacheWriteCost: 0.00001875},
		"claude-haiku-4-5":         {InputTokenCost: 0.0000008, OutputTokenCost: 0.000004, CachedInputCost: 0.00000008, CacheWriteCost: 0.000001},
		"claude-haiku-3.5":         {InputTokenCost: 0.0000008, OutputTokenCost: 0.000004, CachedInputCost: 0.00000008, CacheWriteCost: 0.000001},
		// OpenAI
		"gpt-5.4":     {InputTokenCost: 0.000002, OutputTokenCost: 0.000008},
		"gpt-4o":      {InputTokenCost: 0.0000025, OutputTokenCost: 0.00001},
		"gpt-4o-mini": {InputTokenCost: 0.00000015, OutputTokenCost: 0.0000006},
		"o3":          {InputTokenCost: 0.00001, OutputTokenCost: 0.00004},
		"o4-mini":     {InputTokenCost: 0.0000011, OutputTokenCost: 0.0000044},
		// xAI
		"grok-3":           {InputTokenCost: 0.000003, OutputTokenCost: 0.000015},
		"grok-4-0709":      {InputTokenCost: 0.000003, OutputTokenCost: 0.000015},
		"grok-code-fast-1": {InputTokenCost: 0.000001, OutputTokenCost: 0.000005},
		// Google
		"gemini-2.5-pro":   {InputTokenCost: 0.00000125, OutputTokenCost: 0.00001},
		"gemini-2.5-flash": {InputTokenCost: 0.00000015, OutputTokenCost: 0.0000006},
	}
}

// modelContextWindow returns the context window size (in tokens) for known models.
func modelContextWindow(model string) int {
	windows := map[string]int{
		"claude-sonnet-4-20250514": 200000,
		"claude-sonnet-4":          200000,
		"claude-sonnet-4-6":        200000,
		"claude-opus-4-20250514":   200000,
		"claude-opus-4":            200000,
		"claude-opus-4-6":          200000,
		"claude-haiku-4-5":         200000,
		"claude-haiku-3.5":         200000,
		"gpt-5.4":                  200000,
		"gpt-4o":                   128000,
		"gpt-4o-mini":              128000,
		"o3":                       200000,
		"o4-mini":                  200000,
		"grok-3":                   131072,
		"grok-4-0709":              131072,
		"grok-code-fast-1":         131072,
		"gemini-2.5-pro":           1048576,
		"gemini-2.5-flash":         1048576,
	}
	if w, ok := windows[model]; ok {
		return w
	}
	return 200000 // default
}

// Agent event messages are defined in agent_runner.go.

// Model is the main BubbleTea model.
type Model struct {
	cfg     *config.Config
	runtime agent.RuntimeState
	sty     *styles.Styles
	agent   *core.Agent[string]
	runCtx  context.Context
	cancel  context.CancelFunc
	prog    *tea.Program

	// UI components.
	input     textarea.Model
	spinner   spinner.Model
	askUserCh chan askUserRequest

	// Skills.
	allSkills    []skills.Skill
	activeSkills []skills.Skill

	// State.
	messages   []*chat.Message
	history    []core.ModelMessage // gollem conversation history across turns
	scroll     int
	width      int
	height     int
	busy       bool
	usage      core.RunUsage
	startTime  time.Time
	runID      int
	hookRID    atomic.Int64 // hook-visible runID; read atomically by hooks from agent goroutine
	lastPrompt string

	// Plan/invariant/verification/spec state — mirrored from tool messages.
	planState          plan.State
	invariantState     uiinvariants.State
	verificationState  uiverification.State
	specState          spec.State
	toolState          map[string]any // raw tool state for restoration across runs
	lastRunSummary     *eval.RunSummary
	currentRunMessages []*chat.Message
	missionPlanRun     *missionPlanRun

	// Pending user messages queued while the agent is working.
	// Drained by middleware before each model turn.
	pendingMu   sync.Mutex
	pendingMsgs []string

	askMode      bool
	askQuestions []codetool.AskUserQuestion
	askAnswers   []codetool.AskUserAnswer
	askCurrent   int
	askRespCh    chan<- []codetool.AskUserAnswer
	askDone      chan struct{}

	// Input history for arrow-up recall.
	inputHistory []string
	historyIdx   int // -1 = current input, 0..N = browsing history

	// Cost tracking across the session.
	costTracker   *core.CostTracker
	sessionUsage  core.RunUsage // accumulated across all runs
	lastCost      float64       // cost at end of previous run (for per-request delta)
	originalModel string        // model before any budget-driven downgrade
	downgraded    bool          // true if model was auto-downgraded for budget
	budgetWarned  bool          // true if budget warning was already shown

	// Context window tracking.
	estimatedTokens int // estimated token count of current conversation

	// Active tool tracking — shown in the spinner while busy.
	activeToolName string
	activeToolArgs string

	// Team event bus for forwarding lifecycle events to chat.
	teamEventBus *core.EventBus

	// Initial prompt from command line.
	initialPrompt string

	// Slash command tab completion.
	tabMatches []string // current set of matching commands
	tabIdx     int      // current index in tabMatches

	// Checkpoint/rewind state — captures full session snapshots after each agent turn.
	checkpoints *checkpoint.Store
	turnCount   int // increments with each successful agent completion

	// Tool approval state.
	approvalCh     chan toolApprovalRequest
	approvalDone   chan struct{}
	approvalMode   bool
	approvalTool   string
	approvalArgs   string
	approvalRespCh chan<- bool
	approvalAlways map[string]bool // tools the user has permanently allowed this session

	// Application-scoped context (cancelled on quit).
	appCtx    context.Context
	appCancel context.CancelFunc

	// Mission orchestration state.
	missionCtrl     *mission.Controller
	activeMissionID string
	orchestrator    *mission.Orchestrator

	// File watcher for detecting external file changes.
	fileWatcher *watcher.Watcher

	// Replay trace recording and playback.
	trace       *agent.ReplayTrace // active recording trace (nil when not recording)
	replayMode  bool               // true during replay playback
	replayTrace *agent.ReplayTrace // trace being replayed
	replayIdx   int                // current event index during replay
	replayStart time.Time          // when replay started

	// Pace control state.
	pace                 paceState
	paceCheckpointActive bool            // currently paused at a checkpoint
	paceCheckpointResp   chan<- struct{} // channel to resume after checkpoint
	paceCheckpointCount  int             // tool count shown in checkpoint UI
}

// New creates the initial app model.
func New(cfg *config.Config) *Model {
	ti := textarea.New()
	ti.Placeholder = "Ask anything… /help for commands"
	ti.ShowLineNumbers = false
	ti.SetHeight(1)
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	allSkills, _ := skills.LoadAll(skills.DefaultDir())

	ps := defaultPaceState()
	if mode, ok := parsePaceMode(cfg.PaceMode); ok {
		ps.mode = mode
	}
	if cfg.CheckpointInterval > 0 {
		ps.checkpointInterval = cfg.CheckpointInterval
	}
	ps.clarifyFirst = cfg.PaceClarifyFirst

	ctx, cancel := context.WithCancel(context.Background())

	return &Model{
		appCtx:         ctx,
		appCancel:      cancel,
		cfg:            cfg,
		runtime:        agent.InitialRuntimeState(cfg),
		input:          ti,
		spinner:        sp,
		askUserCh:      make(chan askUserRequest, 1),
		askDone:        make(chan struct{}),
		approvalCh:     make(chan toolApprovalRequest, 1),
		approvalDone:   make(chan struct{}),
		approvalAlways: make(map[string]bool),
		costTracker:    core.NewCostTracker(modelPricing()),
		teamEventBus:   core.NewEventBus(),
		checkpoints:    checkpoint.NewStore(cfg.WorkingDir),
		historyIdx:     -1,
		allSkills:      allSkills,
		pace:           ps,
	}
}

// SetProgram gives the model a reference to the tea.Program for sending async messages.
func (m *Model) SetProgram(p *tea.Program) {
	m.prog = p
	m.subscribeTeamEvents()
	m.startFileWatcher()
}

// startFileWatcher begins monitoring the working directory for external
// file changes. Events are forwarded to the BubbleTea event loop.
func (m *Model) startFileWatcher() {
	if m.cfg == nil || m.cfg.WorkingDir == "" || m.prog == nil {
		return
	}
	fw, err := watcher.New(m.cfg.WorkingDir)
	if err != nil {
		return // non-fatal — watcher is best-effort
	}
	m.fileWatcher = fw
	p := m.prog
	go func() {
		for events := range fw.Events() {
			p.Send(fileChangeMsg{events: events})
		}
	}()
}

// SetInitialPrompt sets a prompt to be sent automatically on first render.
func (m *Model) SetInitialPrompt(prompt string) {
	m.initialPrompt = prompt
}

// subscribeTeamEvents registers event bus handlers that forward team lifecycle
// events to the BubbleTea message loop for display in the chat stream.
func (m *Model) subscribeTeamEvents() {
	bus := m.teamEventBus
	p := m.prog
	if bus == nil || p == nil {
		return
	}
	core.Subscribe(bus, func(e team.TeammateSpawnedEvent) {
		text := fmt.Sprintf("Spawned teammate %s", e.TeammateName)
		if e.Task != "" {
			task := e.Task
			if len(task) > 80 {
				task = task[:77] + "..."
			}
			text += ": " + task
		}
		p.Send(teamEventMsg{text: text})
	})
	core.Subscribe(bus, func(e team.TeammateTerminatedEvent) {
		text := fmt.Sprintf("Teammate %s stopped", e.TeammateName)
		if e.Reason != "" && e.Reason != "stopped" {
			text += " (" + e.Reason + ")"
		}
		p.Send(teamEventMsg{text: text})
	})
	core.Subscribe(bus, func(e team.TeammateIdleEvent) {
		text := fmt.Sprintf("Teammate %s idle", e.TeammateName)
		p.Send(teamEventMsg{text: text})
	})
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
		m.input.Focus(),
		m.spinner.Tick,
		m.waitForAskUser(),
		m.waitForToolApproval(),
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.sty = styles.New(msg.Color)
		m.spinner.Style = m.sty.SpinnerStyle
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(msg.Width - 4)

		// Fallback: initialize styles if BackgroundColorMsg never arrived
		// (e.g., in PTY environments that don't support OSC 11 queries).
		if m.sty == nil {
			m.sty = styles.New(nil)
			m.spinner.Style = m.sty.SpinnerStyle
		}

		// Fire initial prompt on first window size (startup).
		if m.initialPrompt != "" {
			prompt := m.initialPrompt
			m.initialPrompt = ""
			userMsg := &chat.Message{Kind: chat.KindUser, Content: prompt}
			m.messages = append(m.messages, userMsg)
			m.inputHistory = append(m.inputHistory, prompt)
			return m, m.beginRun(prompt, []*chat.Message{userMsg})
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case askUserRequest:
		if msg.runID != m.runID {
			return m, m.waitForAskUser()
		}
		m.beginAskMode(msg)
		return m, tea.Batch(m.input.Focus(), m.waitForAskUser())

	case askUserShutdownMsg:
		return m, nil

	case toolApprovalRequest:
		if msg.runID != m.runID {
			return m, m.waitForToolApproval()
		}
		// Auto-approve if user previously chose "always" for this tool.
		if m.approvalAlways[msg.toolName] {
			if msg.response != nil {
				select {
				case msg.response <- true:
				default:
				}
			}
			return m, m.waitForToolApproval()
		}
		m.beginApprovalMode(msg)
		return m, m.waitForToolApproval()

	case toolApprovalShutdownMsg:
		return m, nil

	case paceCheckpointRequest:
		if msg.runID != m.runID {
			// Stale checkpoint — resume immediately.
			if msg.response != nil {
				select {
				case msg.response <- struct{}{}:
				default:
				}
			}
			return m, nil
		}
		m.paceCheckpointActive = true
		m.paceCheckpointResp = msg.response
		m.paceCheckpointCount = msg.count
		m.input.Reset()
		m.input.SetHeight(1)
		m.input.Placeholder = "Type feedback or press Enter to continue"
		return m, m.input.Focus()

	case compactDoneMsg:
		if msg.err != nil {
			m.messages = append(m.messages, &chat.Message{
				Kind:    chat.KindError,
				Content: "compact failed: " + msg.err.Error(),
			})
		} else {
			m.history = msg.messages
			m.messages = append(m.messages, &chat.Message{
				Kind:    chat.KindAssistant,
				Content: fmt.Sprintf("Context compacted: %d messages → summary + %d recent", msg.beforeCount, msg.afterCount),
			})
		}
		m.scroll = 0
		return m, nil

	case teamEventMsg:
		m.messages = append(m.messages, &chat.Message{
			Kind:    chat.KindSystem,
			Content: msg.text,
		})
		m.scroll = 0
		return m, nil

	case orchestratorEventMsg:
		return m.handleOrchestratorEvent(mission.OrchestratorEvent(msg)), nil

	case fileChangeMsg:
		return m.handleFileChange(msg)

	case contextCompactedMsg:
		m.estimatedTokens = msg.tokensAfter
		// History processor normalization (image stripping, orphan cleanup)
		// is routine housekeeping — don't show a message for it.
		if msg.strategy == core.CompactionStrategyHistoryProcessor {
			return m, nil
		}
		label := "Auto-compact"
		if msg.strategy == core.CompactionStrategyEmergencyTruncation {
			label = "Emergency truncation"
		}
		summary := fmt.Sprintf("%s: %d→%d messages, ~%dk→%dk tokens",
			label, msg.msgsBefore, msg.msgsAfter,
			msg.tokensBefore/1000, msg.tokensAfter/1000)
		m.messages = append(m.messages, &chat.Message{
			Kind:    chat.KindSystem,
			Content: summary,
		})
		m.scroll = 0
		return m, nil

	// Replay events.
	case replayTickMsg:
		if m.replayMode {
			return m.handleReplayTick()
		}
		return m, nil

	case replayDoneMsg:
		if m.replayMode {
			return m.handleReplayDone()
		}
		return m, nil

	case usageUpdateMsg:
		if msg.runID == m.runID && msg.inputTokens > 0 {
			m.estimatedTokens = msg.inputTokens
		}
		return m, nil

	// Agent streaming events.
	case textDeltaMsg:
		if msg.runID != m.runID {
			return m, nil
		}
		m.recordEvent(agent.EventTextDelta, agent.TextDeltaData{Text: msg.text})
		m.appendOrUpdateAssistant(msg.text)
		m.scroll = 0
		return m, nil

	case thinkingDeltaMsg:
		if msg.runID != m.runID {
			return m, nil
		}
		m.recordEvent(agent.EventThinkDelta, agent.ThinkDeltaData{Text: msg.text})
		m.appendOrUpdateThinking(msg.text)
		m.scroll = 0
		return m, nil

	case toolCallMsg:
		if msg.runID != m.runID {
			return m, nil
		}
		m.recordEvent(agent.EventToolCall, agent.ToolCallData{
			CallID:  msg.callID,
			Name:    msg.name,
			Args:    msg.args,
			RawArgs: msg.rawArgs,
		})
		toolMsg := &chat.Message{
			Kind:      chat.KindToolCall,
			CallID:    msg.callID,
			ToolName:  msg.name,
			ToolArgs:  extractMainParam(msg.args),
			RawArgs:   msg.rawArgs,
			Status:    chat.ToolRunning,
			StartedAt: time.Now(),
		}
		m.messages = append(m.messages, toolMsg)
		m.currentRunMessages = append(m.currentRunMessages, toolMsg)
		m.activeToolName = msg.name
		m.activeToolArgs = extractMainParam(msg.args)
		m.scroll = 0
		return m, nil

	case toolResultMsg:
		if msg.runID != m.runID {
			return m, nil
		}
		m.recordEvent(agent.EventToolResult, agent.ToolResultData{
			CallID: msg.callID,
			Name:   msg.name,
			Result: msg.result,
			Error:  msg.errText,
		})
		m.activeToolName = ""
		m.activeToolArgs = ""
		m.finishLastTool(msg.callID, msg.name, msg.result, msg.errText)
		m.applyWorkflowToolState(msg.toolState)
		// Auto-mark verification stale when a successful mutating tool completes,
		// so the UI reflects staleness immediately rather than waiting for the
		// model to explicitly call "verification stale".
		if msg.errText == "" && isMutatingToolName(msg.name) && m.verificationState.HasEntries() {
			m.verificationState.MarkAllStale(msg.name)
		}
		// Mark agent-modified files so the file watcher suppresses them.
		if msg.errText == "" && isMutatingToolName(msg.name) && m.fileWatcher != nil {
			m.markAgentFiles(msg.callID, msg.name)
		}
		// Auto-advance spec phase based on tool state changes.
		m.advanceSpecPhase()
		m.scroll = 0
		return m, nil

	case runtimePreparedMsg:
		if msg.runID != m.runID {
			return m, nil
		}
		return m.handleRuntimePrepared(msg)

	case agentDoneMsg:
		if msg.runID != m.runID {
			return m, nil
		}
		// Record agent completion in trace.
		errText := ""
		if msg.err != nil && !isContextCanceled(msg.err) {
			errText = msg.err.Error()
		}
		m.recordEvent(agent.EventAgentDone, agent.AgentDoneData{
			InputTokens:  msg.usage.InputTokens,
			OutputTokens: msg.usage.OutputTokens,
			ToolCalls:    msg.usage.ToolCalls,
			Error:        errText,
		})
		m.flushTrace()

		// Ring terminal bell if the task took >5 seconds.
		if time.Since(m.startTime) > 5*time.Second {
			fmt.Print("\a")
		}
		m.resetAskState()
		m.busy = false
		m.activeToolName = ""
		m.activeToolArgs = ""
		m.runCtx = nil
		m.cancel = nil
		m.agent = nil
		m.usage = msg.usage
		m.sessionUsage.IncrRun(msg.usage)
		if msg.err != nil && !isContextCanceled(msg.err) {
			errMsg := &chat.Message{
				Kind:    chat.KindError,
				Content: msg.err.Error(),
			}
			m.messages = append(m.messages, errMsg)
			m.currentRunMessages = append(m.currentRunMessages, errMsg)
		}
		m.completeMissionPlanRun(msg.err, msg.messages)
		if msg.err == nil && msg.messages != nil {
			m.history = msg.messages
			m.toolState = msg.toolState
			m.applyWorkflowToolState(msg.toolState)
			// Auto-save session after each successful run.
			go func() {
				_ = agent.SaveSession(m.cfg.WorkingDir, msg.messages, m.messages, msg.toolState, m.sessionUsage, m.cfg.Model, string(m.cfg.Provider), m.lastPrompt, m.planState, m.invariantState, m.verificationState, m.specState)
			}()

			// Create a checkpoint capturing the full session state at this turn.
			m.turnCount++
			m.checkpoints.Save(checkpoint.Checkpoint{
				Turn:              m.turnCount,
				Prompt:            m.lastPrompt,
				History:           msg.messages,
				Messages:          m.messages,
				ToolState:         msg.toolState,
				PlanState:         m.planState,
				InvariantState:    m.invariantState,
				VerificationState: m.verificationState,
				SessionUsage:      m.sessionUsage,
				LastCost:          m.lastCost,
			})
			// Auto-advance spec phase based on final tool state.
			m.advanceSpecPhase()
		}
		validation := config.ValidationResult{}
		if m.cfg != nil {
			validation = m.cfg.Validate()
		}
		summary := eval.BuildRunSummary(
			m.lastPrompt,
			agent.BuildRuntimeReport(m.cfg, m.runtime, validation, msg.err),
			m.currentRunMessages,
			m.verificationState,
			msg.usage,
			msg.err,
		)
		m.lastRunSummary = &summary

		// Update estimated token count from conversation history.
		if msg.messages != nil {
			m.estimatedTokens = core.EstimateTokens(msg.messages)
		}

		// Show per-request usage summary.
		elapsed := time.Since(m.startTime)

		// Count file modifications from this run.
		filesModified := countFilesModified(m.currentRunMessages)

		usageParts := []string{
			fmt.Sprintf("%d↓ %d↑", msg.usage.InputTokens, msg.usage.OutputTokens),
			fmt.Sprintf("%d tools", msg.usage.ToolCalls),
			formatDuration(elapsed),
		}
		if filesModified > 0 {
			usageParts = append(usageParts, fmt.Sprintf("%d files changed", filesModified))
		}
		if cost := m.costTracker.TotalCost() - m.lastCost; cost > 0 {
			if cost < 0.01 {
				usageParts = append(usageParts, fmt.Sprintf("$%.4f", cost))
			} else {
				usageParts = append(usageParts, fmt.Sprintf("$%.2f", cost))
			}
		}
		m.lastCost = m.costTracker.TotalCost()
		ctxPct := 0
		if ctxWindow := modelContextWindow(m.cfg.Model); ctxWindow > 0 {
			ctxPct = msg.usage.InputTokens * 100 / ctxWindow
		}
		if ctxPct > 0 {
			usageParts = append(usageParts, fmt.Sprintf("ctx %d%%", ctxPct))
		}
		usageMsg := &chat.Message{
			Kind:    chat.KindSystem,
			Content: strings.Join(usageParts, " · "),
		}
		m.messages = append(m.messages, usageMsg)

		// Warn when approaching context window limits.
		if ctxPct >= 80 {
			warnMsg := &chat.Message{
				Kind:    chat.KindSystem,
				Content: fmt.Sprintf("Context window %d%% full — consider running /compact", ctxPct),
			}
			m.messages = append(m.messages, warnMsg)
		}

		// Post-run budget check: warn or downgrade for next run.
		if budget := m.cfg.EffectiveBudget(); budget > 0 {
			cost := m.costTracker.TotalCost()
			pct := cost / budget
			if pct >= m.cfg.BudgetWarnPct && !m.budgetWarned {
				m.budgetWarned = true
				remaining := budget - cost
				if remaining < 0 {
					remaining = 0
				}
				m.messages = append(m.messages, &chat.Message{
					Kind:    chat.KindSystem,
					Content: fmt.Sprintf("Budget warning: %.0f%% used ($%.4f of $%.2f) — $%.4f remaining. Model may downgrade on next run.", pct*100, cost, budget, remaining),
				})
			}
		}

		cmds = append(cmds, m.input.Focus())
		return m, tea.Batch(cmds...)
	}

	// Forward to textarea (always — user can type while agent works).
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.approvalMode {
		return m.handleApprovalKey(msg)
	}
	if m.askMode {
		return m.handleAskKey(msg)
	}
	key := msg.String()

	switch key {
	case "ctrl+c":
		if m.busy && m.cancel != nil {
			m.cancelActiveRun(true)
			return m, m.input.Focus()
		}
		m.shutdownAskLoop()
		m.shutdownApprovalLoop()
		m.cleanupSession()
		return m, tea.Quit

	case "escape":
		if m.replayMode {
			m.stopReplay()
			return m, m.input.Focus()
		}
		if m.busy && m.cancel != nil {
			m.cancelActiveRun(true)
			return m, m.input.Focus()
		}

	case "ctrl+l":
		// Clear transcript (like terminal Ctrl+L).
		if !m.busy {
			m.clearSessionState()
			return m, m.input.Focus()
		}

	case "shift+enter":
		// Insert newline for multiline input.
		m.input.InsertString("\n")
		h := min(5, strings.Count(m.input.Value(), "\n")+2)
		m.input.SetHeight(h)
		return m, nil

	case "tab":
		text := m.input.Value()
		if strings.HasPrefix(text, "/") && !strings.Contains(text, " ") {
			return m.completeSlashCommand(text)
		}
		return m, nil

	case "enter":
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m, nil
		}
		if text == "/quit" || text == "/exit" {
			if m.busy && m.cancel != nil {
				m.cancelActiveRun(false)
			}
			m.shutdownAskLoop()
			m.cleanupSession()
			return m, tea.Quit
		}
		if text == "/clear" {
			m.clearSessionState()
			m.input.Reset()
			return m, m.input.Focus()
		}
		if text == "/help" {
			m.input.Reset()
			m.messages = append(m.messages, m.renderHelpMessage())
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/plan" {
			m.input.Reset()
			m.messages = append(m.messages, m.renderPlanSummaryMessage())
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/invariants" {
			m.input.Reset()
			m.messages = append(m.messages, m.renderInvariantSummaryMessage())
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/runtime" {
			m.input.Reset()
			m.messages = append(m.messages, m.renderRuntimeSummaryMessage())
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/verify" {
			m.input.Reset()
			m.messages = append(m.messages, m.renderVerificationSummaryMessage())
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/skills" {
			m.input.Reset()
			m.messages = append(m.messages, m.renderSkillsList()...)
			m.scroll = 0
			return m, m.input.Focus()
		}
		if strings.HasPrefix(text, "/skill ") {
			name := strings.TrimSpace(strings.TrimPrefix(text, "/skill "))
			m.input.Reset()
			m.messages = append(m.messages, m.activateSkill(name))
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/compact" {
			m.input.Reset()
			return m, m.compactContext()
		}
		if text == "/cost" {
			m.input.Reset()
			m.messages = append(m.messages, m.renderCostSummaryMessage())
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/budget" {
			m.input.Reset()
			m.messages = append(m.messages, m.renderBudgetMessage())
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/resume" {
			m.input.Reset()
			msg := m.resumeSession()
			m.messages = append(m.messages, msg)
			m.scroll = 0
			return m, m.input.Focus()
		}
		if strings.HasPrefix(text, "/model") {
			m.input.Reset()
			m.messages = append(m.messages, m.handleModelCommand(text))
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/search" || strings.HasPrefix(text, "/search ") {
			m.input.Reset()
			m.messages = append(m.messages, m.handleSearchCommand(text))
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/diff" {
			m.input.Reset()
			m.messages = append(m.messages, m.renderDiffMessage())
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/undo" || strings.HasPrefix(text, "/undo ") {
			m.input.Reset()
			m.messages = append(m.messages, m.handleUndo(text))
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/rewind" || strings.HasPrefix(text, "/rewind ") {
			m.input.Reset()
			m.messages = append(m.messages, m.handleRewind(text))
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/doctor" {
			m.input.Reset()
			m.messages = append(m.messages, m.renderDoctorMessage())
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/config" {
			m.input.Reset()
			m.messages = append(m.messages, m.renderConfigMessage())
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/team" {
			m.input.Reset()
			m.messages = append(m.messages, m.renderTeamMessage())
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/context" {
			m.input.Reset()
			m.messages = append(m.messages, m.renderContextMessage())
			m.scroll = 0
			return m, m.input.Focus()
		}
		if text == "/mission" || strings.HasPrefix(text, "/mission ") {
			m.input.Reset()
			msg, cmd := m.handleMissionCommand(text)
			m.messages = append(m.messages, msg)
			m.scroll = 0
			if cmd != nil {
				return m, tea.Batch(cmd, m.input.Focus())
			}
			return m, m.input.Focus()
		}
		if text == "/replay" || strings.HasPrefix(text, "/replay ") {
			m.input.Reset()
			msg, cmd := m.handleReplayCommand(text)
			m.messages = append(m.messages, msg)
			m.scroll = 0
			if cmd != nil {
				return m, tea.Batch(cmd, m.input.Focus())
			}
			return m, m.input.Focus()
		}
		if text == "/spec" || strings.HasPrefix(text, "/spec ") {
			m.input.Reset()
			return m.handleSpecCommand(text)
		}
		// Catch unknown slash commands.
		if strings.HasPrefix(text, "/") && !strings.Contains(text, " ") && !m.busy {
			m.input.Reset()
			m.messages = append(m.messages, &chat.Message{
				Kind:    chat.KindError,
				Content: fmt.Sprintf("Unknown command: %s. Try /help for available commands.", text),
			})
			m.scroll = 0
			return m, m.input.Focus()
		}

		m.input.Reset()
		m.input.SetHeight(1)
		m.scroll = 0

		// Push to input history for arrow-up recall.
		m.inputHistory = append(m.inputHistory, text)
		m.historyIdx = -1

		userMsg := &chat.Message{
			Kind:    chat.KindUser,
			Content: text,
		}
		m.messages = append(m.messages, userMsg)

		if m.busy {
			// Queue the message — middleware will inject it before the next model turn.
			m.pendingMu.Lock()
			m.pendingMsgs = append(m.pendingMsgs, text)
			m.pendingMu.Unlock()
			return m, nil
		}

		// Check budget and potentially downgrade model before starting.
		if budgetMsgs := m.checkBudgetAndDowngrade(); len(budgetMsgs) > 0 {
			m.messages = append(m.messages, budgetMsgs...)
		}

		return m, m.beginRun(text, []*chat.Message{userMsg})

	case "up":
		// Recall input history when idle; scroll when busy.
		if !m.busy && len(m.inputHistory) > 0 {
			if m.historyIdx == -1 {
				m.historyIdx = len(m.inputHistory) - 1
			} else if m.historyIdx > 0 {
				m.historyIdx--
			}
			m.input.Reset()
			m.input.InsertString(m.inputHistory[m.historyIdx])
			return m, nil
		}
		m.scroll++

	case "down":
		if !m.busy && m.historyIdx >= 0 {
			if m.historyIdx < len(m.inputHistory)-1 {
				m.historyIdx++
				m.input.Reset()
				m.input.InsertString(m.inputHistory[m.historyIdx])
			} else {
				m.historyIdx = -1
				m.input.Reset()
			}
			return m, nil
		}
		if m.scroll > 0 {
			m.scroll--
		}

	case "pgup":
		m.scroll += 10

	case "pgdown":
		m.scroll = max(0, m.scroll-10)

	case "home":
		// Scroll to top.
		m.scroll = 999999 // clamped in renderChat

	case "end":
		// Scroll to bottom.
		m.scroll = 0
	}

	// Forward unhandled keys to the textarea.
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// slashCommands is the sorted list of available slash commands for tab completion.
var slashCommands = []string{
	"/budget", "/clear", "/compact", "/config", "/context", "/cost", "/diff",
	"/doctor", "/exit", "/help", "/invariants", "/mission", "/model",
	"/plan", "/quit", "/replay", "/resume", "/rewind", "/runtime",
	"/search", "/skill", "/skills", "/spec", "/team", "/undo", "/verify",
}

func (m *Model) completeSlashCommand(text string) (tea.Model, tea.Cmd) {
	prefix := strings.TrimSpace(text)

	// If exact match to previous completion, cycle to next.
	if len(m.tabMatches) > 0 && prefix == m.tabMatches[m.tabIdx] {
		m.tabIdx = (m.tabIdx + 1) % len(m.tabMatches)
		m.input.Reset()
		m.input.InsertString(m.tabMatches[m.tabIdx])
		return m, nil
	}

	// Build new match set.
	var matches []string
	for _, cmd := range slashCommands {
		if strings.HasPrefix(cmd, prefix) {
			matches = append(matches, cmd)
		}
	}
	if len(matches) == 0 {
		return m, nil
	}

	m.tabMatches = matches
	m.tabIdx = 0
	m.input.Reset()
	m.input.InsertString(matches[0])
	return m, nil
}

// drainPending returns and clears any queued user messages.
func (m *Model) drainPending() []string {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	if len(m.pendingMsgs) == 0 {
		return nil
	}
	msgs := m.pendingMsgs
	m.pendingMsgs = nil
	return msgs
}

func (m *Model) clearSessionState() {
	m.cancelActiveRun(false)
	m.resetAskState()
	m.busy = false
	m.messages = nil
	m.history = nil
	m.scroll = 0
	m.usage = core.RunUsage{}
	m.startTime = time.Time{}
	m.runCtx = nil
	m.cleanupSession()
	if m.cfg != nil {
		m.runtime = agent.InitialRuntimeState(m.cfg)
	} else {
		m.runtime = agent.RuntimeState{}
	}
	m.agent = nil
	m.planState = plan.State{}
	m.invariantState = uiinvariants.State{}
	m.verificationState = uiverification.State{}
	m.toolState = nil
	m.lastPrompt = ""
	m.lastRunSummary = nil
	m.currentRunMessages = nil
	m.checkpoints.Clear()
	m.turnCount = 0
	m.pendingMu.Lock()
	m.pendingMsgs = nil
	m.pendingMu.Unlock()
	// Restore original model if it was downgraded.
	if m.downgraded && m.originalModel != "" {
		m.cfg.Model = m.originalModel
	}
	m.originalModel = ""
	m.downgraded = false
	m.budgetWarned = false
}

// Agent lifecycle methods (prepareRun, handleRuntimePrepared, agentHooks,
// runAgent, steeringMiddleware, checkBudgetAndDowngrade) are in agent_runner.go.

// advanceSpecPhase automatically advances the spec workflow phase based on
// changes to plan and invariant state from agent tool calls.
func (m *Model) advanceSpecPhase() {
	if !m.specState.IsActive() {
		return
	}
	// Sync task progress from plan state.
	if m.planState.HasTasks() {
		completed, total := m.planState.Progress()
		m.specState.SetTaskProgress(completed, total)
	}
	// Phase transitions based on accumulated state.
	switch m.specState.Phase {
	case spec.PhaseDraft:
		// When invariants are extracted, the agent has analyzed the spec.
		if m.invariantState.HasItems() {
			m.specState.AdvanceGate("Spec Approval")
			m.specState.SetPhase(spec.PhaseApproved)
		}
	case spec.PhaseApproved:
		// When plan tasks appear, decomposition has happened.
		if m.planState.HasTasks() {
			m.specState.AdvanceGate("Task Decomposition")
			m.specState.SetPhase(spec.PhaseDecomposed)
		}
	case spec.PhaseDecomposed, spec.PhaseAccepted:
		// Once implementation starts (tasks in progress).
		if m.planState.HasTasks() {
			completed, total := m.planState.Progress()
			if completed > 0 || total > 0 {
				m.specState.SetPhase(spec.PhaseImplementing)
			}
		}
	case spec.PhaseImplementing:
		// When all tasks are completed, move to reviewing.
		if m.planState.HasTasks() {
			completed, total := m.planState.Progress()
			if total > 0 && completed == total {
				m.specState.AdvanceGate("Final Diff Review")
				m.specState.SetPhase(spec.PhaseReviewing)
			}
		}
	}
}

// Message stream helpers (appendOrUpdateAssistant, appendOrUpdateThinking,
// finishLastTool, extractMainParam, countFilesModified) are in agent_runner.go.

// extractJSONField extracts a string from a JSON object.
func extractJSONField(jsonStr, field string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return ""
	}
	if v, ok := m[field]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// contextBar renders a compact visual bar for context window usage.
func contextBar(pct int) string {
	const width = 8
	filled := pct * width / 100
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("▰", filled) + strings.Repeat("▱", width-filled)
	return bar
}

func (m *Model) View() tea.View {
	if m.sty == nil {
		return tea.NewView("Loading...")
	}

	var sections []string

	// Header.
	sections = append(sections, m.renderHeader())

	// Chat messages area (header=2 + input + status=1 + padding).
	inputHeight := m.currentInputHeight()
	chatHeight := m.height - 3 - inputHeight
	if chatHeight < 1 {
		chatHeight = 1
	}

	const panelWidth = 38
	const minWidthForPanel = 110
	showPanel := m.hasWorkflowPanel() && m.width >= minWidthForPanel

	chatWidth := m.width
	if showPanel {
		chatWidth = m.width - panelWidth
	}

	chatSection := m.renderChat(chatHeight, chatWidth)
	if showPanel {
		// Both sides have exact dimensions — join line-by-line.
		chatLines := strings.Split(chatSection, "\n")
		panelLines := strings.Split(m.renderWorkflowPanel(chatHeight, panelWidth), "\n")
		combined := make([]string, chatHeight)
		for i := range combined {
			cl, pl := "", ""
			if i < len(chatLines) {
				cl = chatLines[i]
			}
			if i < len(panelLines) {
				pl = panelLines[i]
			}
			combined[i] = cl + pl
		}
		chatSection = strings.Join(combined, "\n")
	}
	sections = append(sections, chatSection)

	// Input area — always show textarea so user can type while agent works.
	sections = append(sections, m.renderInput())

	// Status bar.
	sections = append(sections, m.renderStatusBar())

	v := tea.NewView(strings.Join(sections, "\n"))
	v.AltScreen = true
	return v
}

func (m *Model) renderHeader() string {
	title := m.sty.StatusBar.Accent.Render(" GOLEM ")
	model := m.sty.Header.Model.Render(styles.ModelIcon + " " + m.cfg.Model)
	provider := ""
	if m.cfg.Provider != "" {
		provider = m.sty.Header.Separator.Render(" · ") + m.sty.Header.Provider.Render(string(m.cfg.Provider))
	}
	leftTop := title + " " + model + provider

	var locationParts []string
	if dir := m.cfg.ShortDir(); dir != "" {
		locationParts = append(locationParts, dir)
	}
	if m.runtime.Git != nil {
		if branch := m.runtime.Git.BranchDisplay(); branch != "" {
			locationParts = append(locationParts, branch)
		}
	}
	rightTop := ""
	if len(locationParts) > 0 {
		rightTop = m.sty.Header.WorkingDir.Render(truncateText(strings.Join(locationParts, " · "), max(18, m.width/3)))
	}

	lowerLeft := m.sty.Muted.Render(" " + truncateText(m.renderContextSummary(), max(26, m.width/2)))
	lowerRight := m.sty.HalfMuted.Render(truncateText(m.currentActivitySummary(), max(26, m.width/2)))

	return joinShellLine(leftTop, rightTop, m.width) + "\n" + joinShellLine(lowerLeft, lowerRight, m.width)
}

func joinShellLine(left, right string, width int) string {
	if right == "" {
		return left
	}
	if width <= 0 {
		return left + " " + right
	}
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := width - leftW - rightW
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

func truncateText(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes == 1 {
		return string(runes[:1])
	}
	return string(runes[:maxRunes-1]) + "…"
}

func (m *Model) renderContextSummary() string {
	var parts []string
	if m.runtime.Git != nil && m.runtime.Git.IsRepo {
		if m.runtime.Git.IsDirty {
			parts = append(parts, "uncommitted changes")
		} else {
			parts = append(parts, "repo connected")
		}
	}
	if len(m.runtime.Instructions) > 0 {
		parts = append(parts, fmt.Sprintf("%d instructions", len(m.runtime.Instructions)))
	}
	if len(m.runtime.MCPServers) > 0 {
		parts = append(parts, fmt.Sprintf("%d MCP", len(m.runtime.MCPServers)))
	}
	if m.runtime.MemoryStore != nil {
		parts = append(parts, "memory")
	}
	if len(m.activeSkills) > 0 {
		parts = append(parts, fmt.Sprintf("%d skills", len(m.activeSkills)))
	}
	if m.cfg.PermissionMode != "" && m.cfg.PermissionMode != "auto" {
		parts = append(parts, "approvals "+m.cfg.PermissionMode)
	}
	if len(parts) == 0 {
		return "Ready for prompts and slash commands"
	}
	return strings.Join(parts, " · ")
}

func (m *Model) currentActivitySummary() string {
	switch {
	case m.approvalMode:
		tool := m.approvalTool
		if tool == "" {
			tool = "tool"
		}
		return "Approval required · " + tool + " · y allow · n deny · a always"
	case m.askMode:
		total := len(m.askQuestions)
		if total == 0 {
			return "Waiting for your answer · Enter answer · Esc cancel"
		}
		current := m.askCurrent + 1
		if current > total {
			current = total
		}
		return fmt.Sprintf("Question %d/%d · Enter answer · Esc cancel", current, total)
	case m.busy:
		parts := []string{"Working"}
		if m.activeToolName != "" {
			toolLabel := m.activeToolName
			if m.activeToolArgs != "" {
				toolLabel += " " + truncateText(m.activeToolArgs, 24)
			}
			parts = append(parts, toolLabel)
		}
		if !m.startTime.IsZero() {
			parts = append(parts, time.Since(m.startTime).Truncate(time.Second).String())
		}
		if queued := m.pendingCount(); queued > 0 {
			parts = append(parts, fmt.Sprintf("%d queued", queued))
		}
		parts = append(parts, "Esc cancels")
		return strings.Join(parts, " · ")
	default:
		return "Ready · /help for commands · /search <query>"
	}
}

func (m *Model) renderChat(height, width int) string {
	if len(m.messages) == 0 {
		return m.renderWelcome(height, width)
	}

	// Phase 1: Compute line counts per message using cached renders.
	// This is cheap because unchanged messages hit the render cache.
	type msgInfo struct {
		lines int // lines including trailing gap line
	}
	infos := make([]msgInfo, len(m.messages))
	totalLines := 0
	for i, msg := range m.messages {
		msg.Render(m.sty, width, m.messages)
		n := msg.Lines()
		if n > 0 {
			n++ // gap line between messages
		}
		infos[i] = msgInfo{lines: n}
		totalLines += n
	}

	// Phase 2: Clamp scroll.
	maxScroll := totalLines - height
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}

	// Phase 3: Find which messages are visible.
	// We show lines [totalLines - m.scroll - height, totalLines - m.scroll).
	endLine := totalLines - m.scroll
	startLine := endLine - height
	if startLine < 0 {
		startLine = 0
	}

	// Walk messages to find visible range.
	var visible []string
	linePos := 0
	for i, info := range infos {
		msgEnd := linePos + info.lines
		if linePos >= endLine {
			break // past viewport
		}
		if msgEnd <= startLine {
			linePos = msgEnd
			continue // before viewport
		}

		// This message is (partially) visible — use cached render.
		rendered := m.messages[i].Render(m.sty, width, m.messages)
		if rendered == "" {
			linePos = msgEnd
			continue
		}
		msgLines := strings.Split(rendered, "\n")
		msgLines = append(msgLines, "") // gap line

		// Determine which lines of this message are visible.
		for j, line := range msgLines {
			globalLine := linePos + j
			if globalLine >= startLine && globalLine < endLine {
				visible = append(visible, line)
			}
		}
		linePos = msgEnd
	}

	// Pad to fill viewport height.
	for len(visible) < height {
		visible = append([]string{""}, visible...)
	}

	// Pad every line to exact width so JoinHorizontal places the
	// panel at a fixed column regardless of which messages are visible.
	for i, line := range visible {
		if w := lipgloss.Width(line); w < width {
			visible[i] = line + strings.Repeat(" ", width-w)
		}
	}

	return strings.Join(visible, "\n")
}

func (m *Model) renderWelcome(height, width int) string {
	bodyWidth := max(28, width-4)
	var lines []string
	lines = append(lines, "")
	lines = append(lines, m.sty.StatusBar.Accent.Render(" GOLEM ")+" "+m.sty.Bold.Render("Ship changes with more confidence"))
	lines = append(lines, m.sty.Muted.Render("  "+truncateText(m.renderContextSummary(), bodyWidth)))
	lines = append(lines, "")
	lines = append(lines, m.sty.Bold.Render("  Start here"))
	starterRows := []string{
		"/help — browse commands and keybindings",
		"/search <query> — search across all saved sessions",
		"/doctor — inspect local setup before a long run",
		"Describe the change you want and press Enter to start",
	}
	for _, row := range starterRows {
		lines = append(lines, m.sty.Muted.Render("  "+truncateText(row, bodyWidth)))
	}
	lines = append(lines, "")
	lines = append(lines, m.sty.Bold.Render("  Keys"))
	lines = append(lines, m.sty.Muted.Render("  "+truncateText("Enter send · Shift+Enter newline · Tab complete · Esc cancel · Ctrl+L clear", bodyWidth)))
	lines = append(lines, m.sty.Muted.Render("  "+truncateText("PgUp/PgDn scroll · ↑/↓ recall input history", bodyWidth)))

	content := strings.Join(lines, "\n")
	contentLines := strings.Count(content, "\n") + 1
	padding := strings.Repeat("\n", max(0, height-contentLines-1))
	return padding + content
}

func (m *Model) renderInput() string {
	if m.approvalMode {
		return m.renderApproval()
	}
	if m.askMode {
		return m.renderAskInput()
	}
	return m.renderInputBusyOrIdle()
}

func (m *Model) renderInputBusyOrIdle() string {
	prompt := m.sty.Input.Prompt.Render(" " + styles.PromptIcon + " ")
	body := prompt + m.input.View()
	box := m.sty.Input.Focused.Width(max(24, m.width)).Render(body)
	if !m.busy {
		return box
	}

	elapsed := time.Since(m.startTime).Truncate(time.Second)
	sp := m.spinner.View()
	activity := "Preparing response"
	if m.activeToolName != "" {
		activity = "Running " + m.activeToolName
		if m.activeToolArgs != "" {
			activity += " " + truncateText(m.activeToolArgs, 40)
		}
	}
	meta := []string{activity, elapsed.String()}
	if queued := m.pendingCount(); queued > 0 {
		meta = append(meta, strconv.Itoa(queued)+" queued")
	}
	meta = append(meta, "type and press Enter to steer", "Esc cancels")
	status := m.sty.Muted.Render("  " + sp + " " + strings.Join(meta, " · "))
	return status + "\n" + box
}

func (m *Model) applyWorkflowToolState(toolState map[string]any) {
	m.toolState = toolState
	if currentPlan, ok := deep.PlanFromToolState(toolState); ok {
		m.planState = plan.FromDeepPlan(currentPlan)
	}
	if currentInv, ok := codetool.InvariantsFromToolState(toolState); ok {
		m.invariantState = uiinvariants.FromToolState(currentInv)
	}
	if currentVerify, ok := codetool.VerificationFromToolState(toolState); ok {
		m.verificationState = uiverification.FromToolState(currentVerify)
	}
}

func (m *Model) restoreSessionState(session *agent.SessionData, msgs []core.ModelMessage) error {
	m.history = msgs
	m.sessionUsage = session.Usage
	m.lastPrompt = session.Prompt
	m.applyWorkflowToolState(session.ToolState)

	if len(session.Transcript) > 0 {
		var transcript []*chat.Message
		if err := agent.RestoreJSON(session.Transcript, &transcript); err != nil {
			return fmt.Errorf("restoring transcript: %w", err)
		}
		m.messages = transcript
	}
	if len(session.PlanState) > 0 {
		if err := agent.RestoreJSON(session.PlanState, &m.planState); err != nil {
			return fmt.Errorf("restoring plan state: %w", err)
		}
	}
	if len(session.InvariantState) > 0 {
		if err := agent.RestoreJSON(session.InvariantState, &m.invariantState); err != nil {
			return fmt.Errorf("restoring invariant state: %w", err)
		}
	}
	if len(session.VerificationState) > 0 {
		if err := agent.RestoreJSON(session.VerificationState, &m.verificationState); err != nil {
			return fmt.Errorf("restoring verification state: %w", err)
		}
	}
	if len(session.SpecState) > 0 {
		if err := agent.RestoreJSON(session.SpecState, &m.specState); err != nil {
			return fmt.Errorf("restoring spec state: %w", err)
		}
	}
	m.advanceSpecPhase()
	m.turnCount = 0
	return nil
}

func (m *Model) resumeSession() *chat.Message {
	if m.busy {
		return &chat.Message{Kind: chat.KindAssistant, Content: "Cannot resume while agent is running."}
	}
	if len(m.history) > 0 {
		return &chat.Message{Kind: chat.KindAssistant, Content: "Session already has history. Use `/clear` first to resume a previous session."}
	}
	session, err := agent.LoadLatestSession(m.cfg.WorkingDir)
	if err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to load session: %v", err)}
	}
	if session == nil {
		return &chat.Message{Kind: chat.KindAssistant, Content: "No previous session found for this project."}
	}
	msgs, err := session.RestoreMessages()
	if err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to restore messages: %v", err)}
	}
	if err := m.restoreSessionState(session, msgs); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to restore session state: %v", err)}
	}
	return &chat.Message{
		Kind:    chat.KindAssistant,
		Content: fmt.Sprintf("Resumed session from %s (%d messages, %d requests).", session.Timestamp.Format("Jan 2 15:04"), len(msgs), session.Usage.Requests),
	}
}

func (m *Model) compactContext() tea.Cmd {
	history := m.history
	return func() tea.Msg {
		if len(history) < 3 {
			return compactDoneMsg{err: fmt.Errorf("not enough history to compact (%d messages)", len(history))}
		}
		model, err := agent.CreateModel(m.cfg)
		if err != nil {
			return compactDoneMsg{err: fmt.Errorf("creating model for compaction: %w", err)}
		}
		beforeCount := len(history)
		compressed, err := compactMessages(context.Background(), history, model, 4)
		if err != nil {
			return compactDoneMsg{err: err}
		}
		afterCount := len(compressed)
		return compactDoneMsg{
			beforeCount: beforeCount,
			afterCount:  afterCount,
			messages:    compressed,
		}
	}
}

// compactMessages summarizes older messages, keeping the first message and the
// last keepN messages. The model is used to generate a summary of the middle
// messages.
func compactMessages(ctx context.Context, messages []core.ModelMessage, model core.Model, keepN int) ([]core.ModelMessage, error) {
	if keepN >= len(messages) {
		return messages, nil
	}

	firstMsg := messages[0]

	// Ensure recent messages start with a ModelRequest (user role) to maintain
	// proper alternation required by providers like Anthropic.
	startRecent := len(messages) - keepN
	if startRecent > 1 {
		if _, isResp := messages[startRecent].(core.ModelResponse); isResp {
			startRecent--
		}
	}

	oldMessages := messages[1:startRecent]
	recentMessages := messages[startRecent:]
	if len(oldMessages) == 0 {
		return messages, nil
	}

	// Build a summary prompt from old messages.
	var sb strings.Builder
	sb.WriteString("Summarize this conversation concisely, preserving:\n")
	sb.WriteString("- What files were created, edited, or read (include EXACT file paths)\n")
	sb.WriteString("- What commands were run and their results\n")
	sb.WriteString("- Key decisions made and current approach\n")
	sb.WriteString("- What approaches were tried and whether they succeeded or failed\n")
	sb.WriteString("- Current state: what's done, what's remaining\n\n")
	for _, msg := range oldMessages {
		switch m := msg.(type) {
		case core.ModelRequest:
			for _, part := range m.Parts {
				switch p := part.(type) {
				case core.UserPromptPart:
					content := p.Content
					if len(content) > 500 {
						content = content[:500] + "..."
					}
					fmt.Fprintf(&sb, "User: %s\n", content)
				case core.ToolReturnPart:
					content := fmt.Sprintf("%v", p.Content)
					if len(content) > 800 {
						content = content[:800] + "..."
					}
					fmt.Fprintf(&sb, "[Tool result: %s] %s\n", p.ToolName, content)
				}
			}
		case core.ModelResponse:
			if text := m.TextContent(); text != "" {
				if len(text) > 500 {
					text = text[:500] + "..."
				}
				fmt.Fprintf(&sb, "Assistant: %s\n", text)
			}
			for _, part := range m.Parts {
				if tc, ok := part.(core.ToolCallPart); ok {
					args := tc.ArgsJSON
					if len(args) > 500 {
						args = args[:500] + "..."
					}
					fmt.Fprintf(&sb, "[Tool call: %s] %s\n", tc.ToolName, args)
				}
			}
		}
	}

	summaryReq := []core.ModelMessage{
		core.ModelRequest{
			Parts:     []core.ModelRequestPart{core.UserPromptPart{Content: sb.String(), Timestamp: time.Now()}},
			Timestamp: time.Now(),
		},
	}
	summaryResp, err := model.Request(ctx, summaryReq, nil, &core.ModelRequestParameters{AllowTextOutput: true})
	if err != nil {
		return messages, err
	}
	if summaryResp.TextContent() == "" {
		return messages, nil
	}

	summaryMsg := core.ModelResponse{
		Parts: []core.ModelResponsePart{
			core.TextPart{Content: "[Conversation Summary] " + summaryResp.TextContent()},
		},
		Timestamp: time.Now(),
	}

	result := make([]core.ModelMessage, 0, 2+len(recentMessages))
	result = append(result, firstMsg)
	result = append(result, summaryMsg)
	result = append(result, recentMessages...)
	return result, nil
}

func (m *Model) renderStatusBar() string {
	accent := m.sty.StatusBar.Accent.Render(" GOLEM ")
	divider := m.sty.StatusBar.Divider.Render(" │ ")

	var leftParts []string
	leftParts = append(leftParts, accent)

	if m.usage.Requests > 0 {
		tokens := m.sty.StatusBar.Key.Render("tokens ") +
			m.sty.StatusBar.Value.Render(fmt.Sprintf("%d↓ %d↑", m.usage.InputTokens, m.usage.OutputTokens))
		leftParts = append(leftParts, divider, tokens)

		if m.usage.CacheReadTokens > 0 || m.usage.CacheWriteTokens > 0 {
			cache := m.sty.StatusBar.Key.Render("cache ") +
				m.sty.StatusBar.Value.Render(fmt.Sprintf("%d↺ %d⊕", m.usage.CacheReadTokens, m.usage.CacheWriteTokens))
			leftParts = append(leftParts, divider, cache)
		}

		tools := m.sty.StatusBar.Key.Render("tools ") +
			m.sty.StatusBar.Value.Render(strconv.Itoa(m.usage.ToolCalls))
		leftParts = append(leftParts, divider, tools)
	}

	if len(m.activeSkills) > 0 {
		skills := m.sty.StatusBar.Key.Render("skills ") +
			m.sty.StatusBar.Value.Render(strconv.Itoa(len(m.activeSkills)))
		leftParts = append(leftParts, divider, skills)
	}

	if completed, total := m.planState.Progress(); total > 0 {
		plan := m.sty.StatusBar.Key.Render("plan ") +
			m.sty.StatusBar.Value.Render(fmt.Sprintf("%d/%d", completed, total))
		leftParts = append(leftParts, divider, plan)
	}

	if hardTotal, hardPass, hardFail, hardUnresolved, _, _, _ := m.invariantState.Counts(); hardTotal > 0 || len(m.invariantState.Items) > 0 || m.invariantState.Extracted {
		inv := m.sty.StatusBar.Key.Render("inv ") +
			m.sty.StatusBar.Value.Render(fmt.Sprintf("%d✓ %d✗ %d?", hardPass, hardFail, hardUnresolved))
		leftParts = append(leftParts, divider, inv)
	}

	if queued := m.pendingCount(); queued > 0 {
		queue := m.sty.StatusBar.Key.Render("queued ") +
			m.sty.StatusBar.Value.Render(strconv.Itoa(queued))
		leftParts = append(leftParts, divider, queue)
	}

	if m.verificationState.HasEntries() {
		verify := m.sty.StatusBar.Key.Render("verify ") +
			m.sty.StatusBar.Value.Render(m.verificationState.Badge())
		leftParts = append(leftParts, divider, verify)
	}

	if m.scroll > 0 {
		scrolled := m.sty.StatusBar.Key.Render("scroll ") +
			m.sty.StatusBar.Value.Render(fmt.Sprintf("+%d", m.scroll))
		leftParts = append(leftParts, divider, scrolled)
	}

	// Context window usage — use real provider token count when available.
	if ctxWindow := modelContextWindow(m.cfg.Model); ctxWindow > 0 {
		tokenCount := m.estimatedTokens
		if tokenCount > 0 {
			pct := tokenCount * 100 / ctxWindow
			bar := contextBar(pct)
			ctxLabel := fmt.Sprintf("%s %d%%", bar, pct)
			ctxPart := m.sty.StatusBar.Key.Render("ctx ") +
				m.sty.StatusBar.Value.Render(ctxLabel)
			leftParts = append(leftParts, divider, ctxPart)
		}
	}

	// Team status in status bar.
	if session := m.runtime.Session; session != nil && session.Team != nil {
		members := activeTeamMembers(session.Team.Members())
		running, idle := 0, 0
		for _, mi := range members {
			switch mi.State.String() {
			case "running":
				running++
			case "idle":
				idle++
			}
		}
		if len(members) > 1 { // >1 because leader is always a member
			teamLabel := fmt.Sprintf("%d↑ %d○", running, idle)
			teamPart := m.sty.StatusBar.Key.Render("team ") +
				m.sty.StatusBar.Value.Render(teamLabel)
			leftParts = append(leftParts, divider, teamPart)
		}
	}

	if cost := m.costTracker.TotalCost(); cost > 0 {
		costStr := fmt.Sprintf("$%.2f", cost)
		if cost < 0.01 {
			costStr = fmt.Sprintf("$%.4f", cost)
		}
		// Show budget remaining if configured.
		if budget := m.cfg.EffectiveBudget(); budget > 0 {
			remaining := budget - cost
			if remaining < 0 {
				remaining = 0
			}
			costStr += fmt.Sprintf("/$%.2f", budget)
			costPart := m.sty.StatusBar.Value.Render(costStr)
			leftParts = append(leftParts, divider, costPart)
		} else {
			costPart := m.sty.StatusBar.Value.Render(costStr)
			leftParts = append(leftParts, divider, costPart)
		}
	}

	// Show downgrade indicator when model was auto-downgraded.
	if m.downgraded && m.originalModel != "" {
		downgrade := m.sty.StatusBar.Key.Render("downgraded ") +
			m.sty.StatusBar.Value.Render(m.originalModel+" → "+m.cfg.Model)
		leftParts = append(leftParts, divider, downgrade)
	}

	left := lipgloss.JoinHorizontal(lipgloss.Top, leftParts...)

	// Help hints on the right.
	var hints string
	if m.askMode {
		hints = m.sty.StatusBar.Key.Render("enter ") + m.sty.StatusBar.Value.Render("answer") +
			m.sty.StatusBar.Divider.Render(" │ ") +
			m.sty.StatusBar.Key.Render("esc ") + m.sty.StatusBar.Value.Render("cancel")
	} else if m.busy {
		hints = m.sty.StatusBar.Key.Render("enter ") + m.sty.StatusBar.Value.Render("steer") +
			m.sty.StatusBar.Divider.Render(" │ ") +
			m.sty.StatusBar.Key.Render("esc ") + m.sty.StatusBar.Value.Render("cancel")
	} else {
		hints = m.sty.StatusBar.Key.Render("enter ") + m.sty.StatusBar.Value.Render("send") +
			m.sty.StatusBar.Divider.Render(" │ ") +
			m.sty.StatusBar.Key.Render("shift+enter ") + m.sty.StatusBar.Value.Render("newline") +
			m.sty.StatusBar.Divider.Render(" │ ") +
			m.sty.StatusBar.Key.Render("ctrl+c ") + m.sty.StatusBar.Value.Render("quit")
	}
	hints += " "

	// Calculate gap between left and right.
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(hints)
	gap := m.width - leftW - rightW
	if gap < 0 {
		gap = 0
	}

	content := left + strings.Repeat(" ", gap) + hints
	return m.sty.StatusBar.Base.Width(m.width).Render(content)
}

func (m *Model) renderSkillsList() []*chat.Message {
	if len(m.allSkills) == 0 {
		return []*chat.Message{{
			Kind:    chat.KindAssistant,
			Content: "No skills found in `~/.claude/skills/`.",
		}}
	}

	var b strings.Builder
	b.WriteString("**Available skills** — activate with `/skill <name>`\n\n")

	activeSet := make(map[string]bool)
	for _, s := range m.activeSkills {
		activeSet[s.Name] = true
	}

	for _, s := range m.allSkills {
		marker := "  "
		if activeSet[s.Name] {
			marker = "* "
		}
		b.WriteString(marker)
		b.WriteString("`")
		b.WriteString(s.Name)
		b.WriteString("`")
		if s.Description != "" {
			desc := s.Description
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			b.WriteString(" — ")
			b.WriteString(desc)
		}
		b.WriteString("\n")
	}

	if len(m.activeSkills) > 0 {
		b.WriteString("\n`*` = active")
	}

	return []*chat.Message{{
		Kind:    chat.KindAssistant,
		Content: b.String(),
	}}
}

func (m *Model) activateSkill(name string) *chat.Message {
	pending := ""
	if m.busy {
		pending = " (takes effect on next prompt)"
	}
	// Check if already active; if so, deactivate.
	for i, s := range m.activeSkills {
		if strings.EqualFold(s.Name, name) {
			m.activeSkills = append(m.activeSkills[:i], m.activeSkills[i+1:]...)
			m.agent = nil // force agent recreation without this skill
			return &chat.Message{
				Kind:    chat.KindAssistant,
				Content: fmt.Sprintf("Deactivated skill `%s`.%s", s.Name, pending),
			}
		}
	}

	s := skills.Find(m.allSkills, name)
	if s == nil {
		return &chat.Message{
			Kind:    chat.KindError,
			Content: fmt.Sprintf("Skill %q not found. Use /skills to list available skills.", name),
		}
	}

	m.activeSkills = append(m.activeSkills, *s)
	m.agent = nil // force agent recreation with new skill
	return &chat.Message{
		Kind:    chat.KindAssistant,
		Content: fmt.Sprintf("Activated skill `%s`. The agent will now use this skill's instructions.%s", s.Name, pending),
	}
}

// formatDuration is in agent_runner.go.

// isMutatingToolName returns true for tools that modify repo files.
// Used to auto-mark verification entries stale in the UI.
func isMutatingToolName(name string) bool {
	switch name {
	case "edit", "multi_edit", "write":
		return true
	default:
		return false
	}
}

// handleFileChange processes batched external file change events from the watcher.
func (m *Model) handleFileChange(msg fileChangeMsg) (tea.Model, tea.Cmd) {
	if len(msg.events) == 0 {
		return m, nil
	}

	// Mark verification entries stale due to external changes.
	if m.verificationState.HasEntries() {
		m.verificationState.MarkAllStale("external file change")
	}

	// Build a summary of changed files.
	paths := make([]string, 0, len(msg.events))
	for _, ev := range msg.events {
		paths = append(paths, ev.Path)
	}
	var summary string
	if len(paths) == 1 {
		summary = fmt.Sprintf("External change detected: %s", paths[0])
	} else if len(paths) <= 5 {
		summary = fmt.Sprintf("External changes detected: %s", strings.Join(paths, ", "))
	} else {
		summary = fmt.Sprintf("External changes detected: %s and %d more",
			strings.Join(paths[:3], ", "), len(paths)-3)
	}

	m.messages = append(m.messages, &chat.Message{
		Kind:    chat.KindSystem,
		Content: summary,
	})
	m.scroll = 0

	// If the agent is busy, queue a steering message so it knows about the
	// external modifications and can avoid overwriting them.
	if m.busy {
		notice := fmt.Sprintf("[SYSTEM: External file changes detected — files modified outside the agent: %s. Re-read these files before editing to avoid overwriting user changes.]",
			strings.Join(paths, ", "))
		m.pendingMu.Lock()
		m.pendingMsgs = append(m.pendingMsgs, notice)
		m.pendingMu.Unlock()
	}

	return m, nil
}

// markAgentFiles tells the file watcher which files the agent just modified,
// so it can suppress the resulting fsnotify events.
func (m *Model) markAgentFiles(callID, toolName string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := m.messages[i]
		if msg.Kind != chat.KindToolCall {
			continue
		}
		if (callID != "" && msg.CallID == callID) || (callID == "" && msg.ToolName == toolName) {
			switch toolName {
			case "edit", "write":
				if p := extractJSONField(msg.RawArgs, "file_path"); p != "" {
					if !filepath.IsAbs(p) && m.cfg != nil {
						p = filepath.Join(m.cfg.WorkingDir, p)
					}
					m.fileWatcher.MarkAgentFile(p)
				}
			case "multi_edit":
				if p := extractJSONField(msg.RawArgs, "file_path"); p != "" {
					if !filepath.IsAbs(p) && m.cfg != nil {
						p = filepath.Join(m.cfg.WorkingDir, p)
					}
					m.fileWatcher.MarkAgentFile(p)
				}
			}
			return
		}
	}
}
