package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/skills"
	"github.com/fugue-labs/golem/internal/ui/chat"
	uiinvariants "github.com/fugue-labs/golem/internal/ui/invariants"
	"github.com/fugue-labs/golem/internal/ui/plan"
	"github.com/fugue-labs/golem/internal/ui/styles"
	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/codetool"
	"github.com/fugue-labs/gollem/ext/deep"
)

// Agent event messages sent to the TUI via p.Send from the goroutine.
type (
	textDeltaMsg     struct{ runID int; text string }
	thinkingDeltaMsg struct{ runID int; text string }
	toolCallMsg      struct{ runID int; callID, name, args, rawArgs string }
	toolResultMsg struct {
		runID     int
		callID    string
		name      string
		result    string
		errText   string
		toolState map[string]any
	}
	agentDoneMsg struct {
		runID     int
		usage     core.RunUsage
		messages  []core.ModelMessage
		toolState map[string]any
		err       error
	}
)

// Model is the main BubbleTea model.
type Model struct {
	cfg     *config.Config
	runtime agent.RuntimeState
	sty     *styles.Styles
	agent   *core.Agent[string]
	cancel  context.CancelFunc
	prog    *tea.Program

	// UI components.
	input   textarea.Model
	spinner spinner.Model

	// Skills.
	allSkills    []skills.Skill
	activeSkills []skills.Skill

	// State.
	messages []*chat.Message
	history  []core.ModelMessage // gollem conversation history across turns
	scroll   int
	width     int
	height    int
	busy      bool
	usage     core.RunUsage
	startTime time.Time
	runID     int
	hookRID   atomic.Int32 // hook-visible runID; read atomically by hooks from agent goroutine

	// Plan/invariant state — mirrored from tool messages.
	planState      plan.State
	invariantState uiinvariants.State

	// Pending user messages queued while the agent is working.
	// Drained by middleware before each model turn.
	pendingMu   sync.Mutex
	pendingMsgs []string
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

	return &Model{
		cfg:       cfg,
		runtime:   agent.InitialRuntimeState(cfg),
		input:     ti,
		spinner:   sp,
		allSkills: allSkills,
	}
}

// SetProgram gives the model a reference to the tea.Program for sending async messages.
func (m *Model) SetProgram(p *tea.Program) {
	m.prog = p
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
		m.input.Focus(),
		m.spinner.Tick,
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
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	// Agent streaming events.
	case textDeltaMsg:
		if msg.runID != m.runID {
			return m, nil
		}
		m.appendOrUpdateAssistant(msg.text)
		m.scroll = 0
		return m, nil

	case thinkingDeltaMsg:
		if msg.runID != m.runID {
			return m, nil
		}
		m.appendOrUpdateThinking(msg.text)
		m.scroll = 0
		return m, nil

	case toolCallMsg:
		if msg.runID != m.runID {
			return m, nil
		}
		m.messages = append(m.messages, &chat.Message{
			Kind:     chat.KindToolCall,
			CallID:   msg.callID,
			ToolName: msg.name,
			ToolArgs: extractMainParam(msg.args),
			RawArgs:  msg.rawArgs,
			Status:   chat.ToolRunning,
		})
		m.scroll = 0
		return m, nil

	case toolResultMsg:
		if msg.runID != m.runID {
			return m, nil
		}
		m.finishLastTool(msg.callID, msg.name, msg.result, msg.errText)
		if currentPlan, ok := deep.PlanFromToolState(msg.toolState); ok {
			m.planState = plan.FromDeepPlan(currentPlan)
		}
		if currentInv, ok := codetool.InvariantsFromToolState(msg.toolState); ok {
			m.invariantState = uiinvariants.FromToolState(currentInv)
		}
		m.scroll = 0
		return m, nil

	case agentDoneMsg:
		if msg.runID != m.runID {
			return m, nil
		}
		m.busy = false
		m.cancel = nil
		m.usage = msg.usage
		if msg.err != nil && !errors.Is(msg.err, context.Canceled) {
			m.messages = append(m.messages, &chat.Message{
				Kind:    chat.KindError,
				Content: msg.err.Error(),
			})
		} else if msg.messages != nil {
			m.history = msg.messages
			if currentPlan, ok := deep.PlanFromToolState(msg.toolState); ok {
				m.planState = plan.FromDeepPlan(currentPlan)
			}
			if currentInv, ok := codetool.InvariantsFromToolState(msg.toolState); ok {
				m.invariantState = uiinvariants.FromToolState(currentInv)
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
	key := msg.String()

	switch {
	case key == "ctrl+c":
		if m.busy && m.cancel != nil {
			m.cancel()
			m.cancel = nil
			m.busy = false
			m.runID++
			m.agent = nil // old Run() may still be in-flight; force new agent
			// Clean up old session resources in background (can't block —
			// Run() goroutine is still winding down).
			go m.runtime.Session.Cleanup()
			return m, m.input.Focus()
		}
		m.runtime.Session.Cleanup()
		return m, tea.Quit

	case key == "escape":
		if m.busy && m.cancel != nil {
			m.cancel()
			m.cancel = nil
			m.busy = false
			m.runID++
			m.agent = nil // old Run() may still be in-flight; force new agent
			go m.runtime.Session.Cleanup()
			return m, m.input.Focus()
		}

	case key == "shift+enter":
		// Insert newline for multiline input.
		m.input.InsertString("\n")
		h := min(5, strings.Count(m.input.Value(), "\n")+2)
		m.input.SetHeight(h)
		return m, nil

	case key == "enter":
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m, nil
		}
		if text == "/quit" || text == "/exit" {
			if m.busy && m.cancel != nil {
				m.cancel()
				m.cancel = nil
			}
			m.runtime.Session.Cleanup()
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
		m.input.Reset()
		m.input.SetHeight(1)
		m.scroll = 0

		m.messages = append(m.messages, &chat.Message{
			Kind:    chat.KindUser,
			Content: text,
		})

		if m.busy {
			// Queue the message — middleware will inject it before the next model turn.
			m.pendingMu.Lock()
			m.pendingMsgs = append(m.pendingMsgs, text)
			m.pendingMu.Unlock()
			return m, nil
		}

		m.busy = true
		m.startTime = time.Now()
		m.runID++
		m.hookRID.Store(int32(m.runID))
		return m, m.runAgent(text)

	case key == "up":
		m.scroll++

	case key == "down":
		if m.scroll > 0 {
			m.scroll--
		}

	case key == "pgup":
		m.scroll += 10

	case key == "pgdown":
		m.scroll = max(0, m.scroll-10)
	}

	// Forward unhandled keys to the textarea.
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
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
	if m.busy && m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.runID++
	m.hookRID.Store(int32(m.runID))
	m.busy = false
	m.messages = nil
	m.history = nil
	m.scroll = 0
	m.usage = core.RunUsage{}
	m.startTime = time.Time{}
	m.runtime.Session.Cleanup()
	if m.cfg != nil {
		m.runtime = agent.InitialRuntimeState(m.cfg)
	} else {
		m.runtime = agent.RuntimeState{}
	}
	m.agent = nil
	m.planState = plan.State{}
	m.invariantState = uiinvariants.State{}
	m.pendingMu.Lock()
	m.pendingMsgs = nil
	m.pendingMu.Unlock()
}

// steeringMiddleware injects queued user messages before each model turn.
func (m *Model) steeringMiddleware() core.AgentMiddleware {
	return func(
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
	}
}

func (m *Model) runAgent(prompt string) tea.Cmd {
	p := m.prog
	runID := m.runID

	// Ensure agent exists with hooks for TUI visibility.
	// The agent is kept alive across prompts so that stateful resources
	// (LSP connections, planning/invariant state, middleware counters,
	// background processes) persist naturally. Only /clear or skill
	// toggles nil the agent.
	if m.agent == nil {
		hooks := core.Hook{
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
		}
		a, runtime, err := agent.New(m.cfg, prompt, m.activeSkills,
			core.WithHooks[string](hooks),
			core.WithAgentMiddleware[string](m.steeringMiddleware()),
		)
		if err != nil {
			return func() tea.Msg {
				return agentDoneMsg{runID: runID, err: err}
			}
		}
		m.agent = a
		m.runtime = runtime
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	a := m.agent
	history := m.history

	return func() tea.Msg {
		var runOpts []core.RunOption
		if len(history) > 0 {
			runOpts = append(runOpts, core.WithMessages(history...))
		}
		result, err := a.Run(ctx, prompt, runOpts...)
		if err != nil {
			return agentDoneMsg{runID: runID, err: err}
		}
		return agentDoneMsg{runID: runID, usage: result.Usage, messages: result.Messages, toolState: result.ToolState}
	}
}

func (m *Model) appendOrUpdateAssistant(delta string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Kind == chat.KindAssistant {
			m.messages[i].Content += delta
			return
		}
		// Don't look past user messages.
		if m.messages[i].Kind == chat.KindUser {
			break
		}
	}
	m.messages = append(m.messages, &chat.Message{
		Kind:    chat.KindAssistant,
		Content: delta,
	})
}

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
}

func (m *Model) finishLastTool(callID, name, result, errText string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := m.messages[i]
		if msg.Kind != chat.KindToolCall || msg.Status != chat.ToolRunning {
			continue
		}
		// Match by call ID when available, fall back to name match.
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
		// Store result content inline on the tool call message so
		// it renders directly below its header.
		if result != "" {
			msg.Content = result
		}
		break
	}
	if errText != "" {
		m.messages = append(m.messages, &chat.Message{
			Kind:    chat.KindError,
			Content: fmt.Sprintf("%s: %s", name, errText),
		})
	}
}

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

func (m *Model) View() tea.View {
	if m.sty == nil {
		return tea.NewView("Loading...")
	}

	var sections []string

	// Header.
	sections = append(sections, m.renderHeader())

	// Chat messages area (header=2 + input + status=1 + padding).
	inputHeight := strings.Count(m.input.Value(), "\n") + 2
	if inputHeight > 6 {
		inputHeight = 6
	}
	if m.busy {
		inputHeight++ // spinner status line above the textarea
	}
	chatHeight := m.height - 3 - inputHeight
	if chatHeight < 1 {
		chatHeight = 1
	}

	const panelWidth = 32
	const minWidthForPanel = 100
	showPanel := m.planState.HasTasks() && m.width >= minWidthForPanel

	chatWidth := m.width
	if showPanel {
		chatWidth = m.width - panelWidth
	}

	chatSection := m.renderChat(chatHeight, chatWidth)
	if showPanel {
		// Both sides have exact dimensions — join line-by-line.
		chatLines := strings.Split(chatSection, "\n")
		panelLines := strings.Split(m.renderPlanPanel(chatHeight, panelWidth), "\n")
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
	model := m.sty.Header.Model.Render(m.cfg.Model)
	provider := m.sty.Header.Provider.Render(string(m.cfg.Provider))
	sep := m.sty.Header.Separator.Render(" • ")
	dir := m.sty.Header.WorkingDir.Render(m.cfg.ShortDir())

	header := fmt.Sprintf(" %s%s%s%s%s", model, sep, provider, sep, dir)
	line := m.sty.Subtle.Render(strings.Repeat(styles.Separator, m.width))
	return header + "\n" + line
}

func (m *Model) renderChat(height, width int) string {
	if len(m.messages) == 0 {
		greeting := m.sty.Muted.Render("  Ask anything, or try /help for commands.")
		padding := strings.Repeat("\n", max(0, height-2))
		return padding + greeting
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

func (m *Model) renderInput() string {
	if m.busy {
		elapsed := time.Since(m.startTime).Truncate(time.Second)
		sp := m.spinner.View()
		status := m.sty.Muted.Render(fmt.Sprintf(" %s %s", sp, elapsed))
		prompt := m.sty.Input.Prompt.Render(" > ")
		return status + "\n" + prompt + m.input.View()
	}
	prompt := m.sty.Input.Prompt.Render(" > ")
	return prompt + m.input.View()
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
			m.sty.StatusBar.Value.Render(fmt.Sprintf("%d", m.usage.ToolCalls))
		leftParts = append(leftParts, divider, tools)
	}

	if len(m.activeSkills) > 0 {
		skills := m.sty.StatusBar.Key.Render("skills ") +
			m.sty.StatusBar.Value.Render(fmt.Sprintf("%d", len(m.activeSkills)))
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

	if m.scroll > 0 {
		scrolled := m.sty.StatusBar.Key.Render("scroll ") +
			m.sty.StatusBar.Value.Render(fmt.Sprintf("+%d", m.scroll))
		leftParts = append(leftParts, divider, scrolled)
	}

	provider := m.sty.StatusBar.Provider.Render(string(m.cfg.Provider) + "/" + m.cfg.Model)
	leftParts = append(leftParts, divider, provider)

	left := lipgloss.JoinHorizontal(lipgloss.Top, leftParts...)

	// Help hints on the right.
	var hints string
	if m.busy {
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

func (m *Model) renderPlanPanel(height, width int) string {
	completed, total := m.planState.Progress()
	contentWidth := width - 2 // border (1) + space (1)

	// Border prefix for each line: "│ " in the border color.
	borderStr := lipgloss.NewStyle().Foreground(m.sty.BgSubtle).Render(styles.BorderThin) + " "

	// Title line with progress.
	title := m.sty.Panel.Title.Render("Plan")
	progress := m.sty.Panel.Progress.Render(fmt.Sprintf("%d/%d %s", completed, total, styles.CheckIcon))
	titleGap := contentWidth - lipgloss.Width(title) - lipgloss.Width(progress)
	if titleGap < 1 {
		titleGap = 1
	}
	titleLine := title + strings.Repeat(" ", titleGap) + progress

	sep := m.sty.Panel.Separator.Render(strings.Repeat(styles.Separator, contentWidth))

	// Task lines.
	maxTasks := height - 2 // title + separator
	if maxTasks < 1 {
		maxTasks = 1
	}

	var taskLines []string
	for i, t := range m.planState.Tasks {
		if total > maxTasks && i >= maxTasks-1 {
			remaining := total - i
			taskLines = append(taskLines, m.sty.Muted.Render(
				fmt.Sprintf("... +%d more", remaining)))
			break
		}

		var icon string
		switch t.Status {
		case "completed":
			icon = m.sty.Panel.IconCompleted.Render(styles.CheckIcon)
		case "in_progress":
			icon = m.sty.Panel.IconInProgress.Render(styles.InProgressIcon)
		case "blocked":
			icon = m.sty.Panel.IconBlocked.Render(styles.BlockedIcon)
		default:
			icon = m.sty.Panel.IconPending.Render(styles.HollowIcon)
		}

		desc := t.Description
		maxDesc := contentWidth - 4 // space + icon + space + breathing
		if maxDesc > 0 {
			desc = ansi.Truncate(desc, maxDesc, "...")
		}

		if t.Status == "completed" {
			desc = m.sty.Panel.TaskDone.Render(desc)
		} else {
			desc = m.sty.Panel.TaskText.Render(desc)
		}

		taskLines = append(taskLines, fmt.Sprintf(" %s %s", icon, desc))
	}

	// Pad to exactly maxTasks lines.
	for len(taskLines) < maxTasks {
		taskLines = append(taskLines, "")
	}

	// Assemble all content lines, then apply border prefix and pad to width.
	allLines := make([]string, 0, height)
	allLines = append(allLines, titleLine)
	allLines = append(allLines, sep)
	allLines = append(allLines, taskLines...)

	// Ensure exactly height lines.
	for len(allLines) < height {
		allLines = append(allLines, "")
	}
	if len(allLines) > height {
		allLines = allLines[:height]
	}

	// Prepend border, pad each line to exact width.
	for i, line := range allLines {
		full := borderStr + line
		if w := lipgloss.Width(full); w < width {
			full += strings.Repeat(" ", width-w)
		}
		allLines[i] = full
	}

	return strings.Join(allLines, "\n")
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
