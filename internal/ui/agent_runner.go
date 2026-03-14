// agent_runner.go — Agent run lifecycle management.
//
// This file contains all Model methods related to starting, monitoring,
// and completing agent runs. It is the first step toward a standalone
// AgentRunner component (tui-ayi).
//
// Message types for agent events are also defined here:
// textDeltaMsg, thinkingDeltaMsg, toolCallMsg, toolResultMsg,
// runtimePreparedMsg, agentDoneMsg, compactDoneMsg, contextCompactedMsg,
// usageUpdateMsg.

package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/ui/chat"
	"github.com/fugue-labs/golem/internal/ui/watcher"
	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/codetool"
)

// ─── Agent event messages ───────────────────────────────────────────────────

// These messages flow from the agent goroutine to the TUI via p.Send().

type (
	textDeltaMsg struct {
		runID int
		text  string
	}
	thinkingDeltaMsg struct {
		runID int
		text  string
	}
	toolCallMsg struct {
		runID                       int
		callID, name, args, rawArgs string
	}
	toolResultMsg struct {
		runID     int
		callID    string
		name      string
		result    string
		errText   string
		toolState map[string]any
	}
	runtimePreparedMsg struct {
		runID   int
		prompt  string
		runtime agent.RuntimeState
		err     error
	}
	agentDoneMsg struct {
		runID     int
		usage     core.RunUsage
		messages  []core.ModelMessage
		toolState map[string]any
		err       error
	}
	compactDoneMsg struct {
		beforeCount int
		afterCount  int
		messages    []core.ModelMessage
		err         error
	}
	contextCompactedMsg struct {
		strategy     string
		msgsBefore   int
		msgsAfter    int
		tokensBefore int
		tokensAfter  int
	}
	usageUpdateMsg struct {
		runID       int
		inputTokens int // input tokens from the most recent model request (= context size)
	}
	teamEventMsg struct {
		text string // pre-formatted event description
	}
	askUserRequest struct {
		runID     int
		questions []codetool.AskUserQuestion
		response  chan<- []codetool.AskUserAnswer
	}
	askUserShutdownMsg struct{}
	fileChangeMsg      struct {
		events []watcher.Event
	}
)

// ─── Starting a run ─────────────────────────────────────────────────────────

// beginRun is the shared preamble for starting an agent run. It sets up run
// state (busy, runID, context) and returns the tea.Cmd that kicks off runtime
// preparation. initialMsgs are the messages to seed currentRunMessages with
// (typically the user message that triggered the run).
func (m *Model) beginRun(prompt string, initialMsgs []*chat.Message) tea.Cmd {
	m.busy = true
	m.startTime = time.Now()
	m.lastPrompt = prompt
	m.currentRunMessages = initialMsgs
	m.runID++
	m.hookRID.Store(int64(m.runID))
	ctx, cancel := context.WithCancel(context.Background())
	m.runCtx = ctx
	runID := m.runID
	m.cancel = func() {
		// Diagnostic: log the call site when the run context is canceled.
		// This helps identify unexpected cancellation during long runs
		// (see tui-mn1: planner context canceled unexpectedly).
		var buf [4096]byte
		n := runtime.Stack(buf[:], false)
		fmt.Fprintf(os.Stderr, "[golem] run %d context canceled from:\n%s\n", runID, buf[:n])
		cancel()
	}
	m.startRecording()
	m.recordEvent(agent.EventUserInput, agent.UserInputData{Text: prompt})
	return m.prepareRun(prompt)
}

// prepareRun returns a tea.Cmd that prepares the agent runtime asynchronously.
func (m *Model) prepareRun(prompt string) tea.Cmd {
	runID := m.runID
	ctx := m.runCtx
	cfg := m.cfg

	return func() tea.Msg {
		runtime, err := agent.PrepareRuntime(ctx, cfg, prompt)
		return runtimePreparedMsg{runID: runID, prompt: prompt, runtime: runtime, err: err}
	}
}

// handleRuntimePrepared wires up the agent with hooks, middleware, and
// cost tracking, then starts the run.
func (m *Model) handleRuntimePrepared(msg runtimePreparedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, func() tea.Msg {
			return agentDoneMsg{runID: msg.runID, err: msg.err}
		}
	}

	// Purge stale team: if all non-leader teammates are stopped, nil the
	// team so a fresh one is created for this run. This prevents stopped
	// members from accumulating across runs and blocking name reuse.
	if sess := m.runtime.Session; sess != nil && sess.Team != nil {
		m.purgeStaleTeam(sess)
	}

	msg.runtime.Session = m.runtime.Session
	msg.runtime.EventBus = m.teamEventBus
	if msg.runtime.EffectiveTeamMode {
		msg.runtime.AskUserFunc = makeAskUserFunc(msg.runID, m.askUserCh)
	}
	var extraOpts []core.AgentOption[string]
	extraOpts = append(extraOpts,
		core.WithHooks[string](m.agentHooks()),
		core.WithAgentMiddleware[string](m.steeringMiddleware()),
		core.WithAgentMiddleware[string](diffuseReadLoopMiddleware(6)),
		core.WithCostTracker[string](m.costTracker),
	)
	if m.cfg.PermissionMode == "suggest" {
		extraOpts = append(extraOpts,
			core.WithToolApproval[string](makeToolApprovalFunc(msg.runID, m.approvalCh)),
		)
	}
	a, err := agent.NewWithRuntime(
		m.cfg,
		&msg.runtime,
		m.activeSkills,
		extraOpts...,
	)
	if err != nil {
		return m, func() tea.Msg {
			return agentDoneMsg{runID: msg.runID, err: err}
		}
	}

	m.agent = a
	m.runtime = msg.runtime

	// Show model routing info when a different model was selected for this turn.
	if msg.runtime.RoutedModel != "" && msg.runtime.RoutedModel != m.cfg.Model {
		routeMsg := &chat.Message{
			Kind:    chat.KindSystem,
			Content: fmt.Sprintf("model: %s (%s)", msg.runtime.RoutedModel, msg.runtime.RoutingReason),
		}
		m.messages = append(m.messages, routeMsg)
		m.currentRunMessages = append(m.currentRunMessages, routeMsg)
	}

	return m, m.runAgent(msg.prompt)
}

// agentHooks returns the hook callbacks that stream events to the TUI.
func (m *Model) agentHooks() core.Hook {
	p := m.prog
	return core.Hook{
		OnModelResponse: func(_ context.Context, _ *core.RunContext, resp *core.ModelResponse) {
			rid := int(m.hookRID.Load())
			if p == nil || resp == nil {
				return
			}
			for _, part := range resp.Parts {
				switch pt := part.(type) {
				case core.TextPart:
					if pt.Content != "" {
						p.Send(textDeltaMsg{runID: rid, text: pt.Content})
					}
				case core.ThinkingPart:
					if pt.Content != "" {
						p.Send(thinkingDeltaMsg{runID: rid, text: pt.Content})
					}
				}
			}
			// Send real provider input token count for accurate context
			// window tracking in the status bar.
			if resp.Usage.InputTokens > 0 {
				p.Send(usageUpdateMsg{runID: rid, inputTokens: resp.Usage.InputTokens})
			}
		},
		OnToolStart: func(_ context.Context, _ *core.RunContext, toolCallID, toolName, argsJSON string) {
			if p != nil {
				rid := int(m.hookRID.Load())
				p.Send(toolCallMsg{runID: rid, callID: toolCallID, name: toolName, args: argsJSON, rawArgs: argsJSON})
			}
		},
		OnToolEnd: func(_ context.Context, rc *core.RunContext, toolCallID, toolName, result string, err error) {
			if p != nil {
				rid := int(m.hookRID.Load())
				errText := ""
				if err != nil {
					errText = err.Error()
				}
				p.Send(toolResultMsg{runID: rid, callID: toolCallID, name: toolName, result: result, errText: errText, toolState: rc.ToolState()})
			}
		},
		OnContextCompaction: func(_ context.Context, _ *core.RunContext, stats core.ContextCompactionStats) {
			if p != nil {
				p.Send(contextCompactedMsg{
					strategy:     stats.Strategy,
					msgsBefore:   stats.MessagesBefore,
					msgsAfter:    stats.MessagesAfter,
					tokensBefore: stats.EstimatedTokensBefore,
					tokensAfter:  stats.EstimatedTokensAfter,
				})
			}
		},
	}
}

// runAgent returns a tea.Cmd that executes the agent.
func (m *Model) runAgent(prompt string) tea.Cmd {
	runID := m.runID
	a := m.agent
	history := m.history
	toolState := m.toolState
	ctx := m.runCtx
	if ctx == nil {
		ctx = context.Background()
	}

	return func() tea.Msg {
		var runOpts []core.RunOption
		if len(history) > 0 {
			runOpts = append(runOpts, core.WithMessages(history...))
		}
		if len(toolState) > 0 {
			runOpts = append(runOpts, core.WithToolState(toolState))
		}
		result, err := a.Run(ctx, prompt, runOpts...)
		if err != nil {
			return agentDoneMsg{runID: runID, err: err}
		}
		return agentDoneMsg{runID: runID, usage: result.Usage, messages: result.Messages, toolState: result.ToolState}
	}
}

// ─── Middleware ──────────────────────────────────────────────────────────────

// steeringMiddleware injects queued user messages before each model turn.
func (m *Model) steeringMiddleware() core.AgentMiddleware {
	return core.RequestOnlyMiddleware(func(
		ctx context.Context,
		messages []core.ModelMessage,
		settings *core.ModelSettings,
		params *core.ModelRequestParameters,
		next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error),
	) (*core.ModelResponse, error) {
		if pending := m.drainPending(); len(pending) > 0 {
			for _, text := range pending {
				messages = append(messages, core.ModelRequest{
					Parts: []core.ModelRequestPart{
						core.UserPromptPart{
							Content:   text,
							Timestamp: time.Now(),
						},
					},
					Timestamp: time.Now(),
				})
			}
		}
		return next(ctx, messages, settings, params)
	})
}

// ─── Budget management ──────────────────────────────────────────────────────

// checkBudgetAndDowngrade checks whether the session is approaching the budget
// cap and, if so, auto-downgrades to a cheaper model.
func (m *Model) checkBudgetAndDowngrade() []*chat.Message {
	budget := m.cfg.EffectiveBudget()
	if budget <= 0 {
		return nil
	}

	cost := m.costTracker.TotalCost()
	pct := cost / budget
	var msgs []*chat.Message

	// Warning threshold.
	if pct >= m.cfg.BudgetWarnPct && !m.budgetWarned {
		m.budgetWarned = true
		remaining := budget - cost
		msgs = append(msgs, &chat.Message{
			Kind:    chat.KindSystem,
			Content: fmt.Sprintf("Budget warning: %.0f%% used ($%.4f of $%.2f) — $%.4f remaining", pct*100, cost, budget, remaining),
		})
	}

	// Downgrade threshold (90% of budget).
	if pct >= 0.9 {
		var target string
		if m.cfg.FallbackModel != "" {
			if m.cfg.Model != m.cfg.FallbackModel {
				target = m.cfg.FallbackModel
			}
		} else {
			target = config.CheaperModel(m.cfg.Provider, m.cfg.Model)
		}

		if target != "" {
			if m.originalModel == "" {
				m.originalModel = m.cfg.Model
			}
			old := m.cfg.Model
			m.cfg.Model = target
			m.downgraded = true
			msgs = append(msgs, &chat.Message{
				Kind:    chat.KindSystem,
				Content: fmt.Sprintf("Budget cap approaching — downgraded from %s to %s for cost savings", old, target),
			})
		}
	}

	return msgs
}

// ─── Message stream helpers ─────────────────────────────────────────────────

// appendOrUpdateAssistant appends a text delta to the last assistant message
// or creates a new one.
func (m *Model) appendOrUpdateAssistant(delta string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Kind == chat.KindAssistant {
			m.messages[i].Content += delta
			return
		}
		if m.messages[i].Kind == chat.KindUser {
			break
		}
	}
	m.messages = append(m.messages, &chat.Message{
		Kind:    chat.KindAssistant,
		Content: delta,
	})
	msg := m.messages[len(m.messages)-1]
	m.currentRunMessages = append(m.currentRunMessages, msg)
}

// appendOrUpdateThinking appends a thinking delta to the last thinking message
// or creates a new one.
func (m *Model) appendOrUpdateThinking(delta string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Kind == chat.KindThinking {
			m.messages[i].Content += delta
			return
		}
		if m.messages[i].Kind == chat.KindUser || m.messages[i].Kind == chat.KindAssistant {
			break
		}
	}
	m.messages = append(m.messages, &chat.Message{
		Kind:    chat.KindThinking,
		Content: delta,
	})
	msg := m.messages[len(m.messages)-1]
	m.currentRunMessages = append(m.currentRunMessages, msg)
}

// finishLastTool marks the most recent tool call message as completed.
func (m *Model) finishLastTool(callID, name, result, errText string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := m.messages[i]
		if msg.Kind != chat.KindToolCall || msg.Status != chat.ToolRunning {
			continue
		}
		if callID != "" && msg.CallID != callID {
			continue
		}
		if callID == "" && msg.ToolName != name {
			continue
		}
		if errText != "" {
			msg.Status = chat.ToolError
		} else {
			msg.Status = chat.ToolSuccess
		}
		if !msg.StartedAt.IsZero() {
			msg.Duration = time.Since(msg.StartedAt)
		}
		if result != "" {
			msg.Content = result
		}
		break
	}
	if errText != "" {
		errMsg := &chat.Message{
			Kind:    chat.KindError,
			Content: fmt.Sprintf("%s: %s", name, errText),
		}
		m.messages = append(m.messages, errMsg)
		m.currentRunMessages = append(m.currentRunMessages, errMsg)
	}
}

// extractMainParam pulls the primary parameter from a tool call's JSON args
// for display in the spinner.
func extractMainParam(argsJSON string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	for _, key := range []string{"command", "file_path", "path", "pattern", "task", "description", "content"} {
		if v, ok := args[key]; ok {
			s := fmt.Sprintf("%v", v)
			if len(s) > 80 {
				s = s[:80] + "..."
			}
			return s
		}
	}
	return ""
}

// countFilesModified counts unique files changed by edit/write/multi_edit in a run.
func countFilesModified(messages []*chat.Message) int {
	seen := make(map[string]bool)
	for _, msg := range messages {
		if msg.Kind != chat.KindToolCall {
			continue
		}
		switch msg.ToolName {
		case "edit", "write":
			if p := extractJSONField(msg.RawArgs, "file_path"); p != "" {
				seen[p] = true
			} else if p := extractJSONField(msg.RawArgs, "path"); p != "" {
				seen[p] = true
			}
		case "multi_edit":
			if p := extractJSONField(msg.RawArgs, "file_path"); p != "" {
				seen[p] = true
			}
		}
	}
	return len(seen)
}

// formatDuration returns a human-friendly duration string.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}
