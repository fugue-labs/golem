package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/skills"
	"github.com/fugue-labs/golem/internal/ui/chat"
	"github.com/fugue-labs/golem/internal/ui/styles"
	"github.com/fugue-labs/gollem/core"
)

// Agent event messages sent to the TUI via p.Send from the goroutine.
type (
	textDeltaMsg     struct{ text string }
	thinkingDeltaMsg struct{ text string }
	toolCallMsg      struct{ name, args, rawArgs string }
	toolResultMsg    struct{ name, result string }
	agentDoneMsg     struct{ usage core.RunUsage; err error }
)

// Model is the main BubbleTea model.
type Model struct {
	cfg    *config.Config
	sty    *styles.Styles
	agent  *core.Agent[string]
	cancel context.CancelFunc
	prog   *tea.Program

	// UI components.
	input   textarea.Model
	spinner spinner.Model

	// Skills.
	allSkills    []skills.Skill
	activeSkills []skills.Skill

	// State.
	messages  []*chat.Message
	scroll    int
	width     int
	height    int
	busy      bool
	usage     core.RunUsage
	startTime time.Time
}

// New creates the initial app model.
func New(cfg *config.Config) *Model {
	ti := textarea.New()
	ti.Placeholder = "Ask anything... (Enter to send)"
	ti.ShowLineNumbers = false
	ti.SetHeight(1)
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	allSkills, _ := skills.LoadAll(skills.DefaultDir())

	return &Model{
		cfg:       cfg,
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
		m.appendOrUpdateAssistant(msg.text)
		m.scroll = 0
		return m, nil

	case thinkingDeltaMsg:
		m.appendOrUpdateThinking(msg.text)
		m.scroll = 0
		return m, nil

	case toolCallMsg:
		m.messages = append(m.messages, &chat.Message{
			Kind:     chat.KindToolCall,
			ToolName: msg.name,
			ToolArgs: extractMainParam(msg.args),
			RawArgs:  msg.rawArgs,
			Status:   chat.ToolRunning,
		})
		m.scroll = 0
		return m, nil

	case toolResultMsg:
		m.finishLastTool(msg.name, msg.result)
		m.scroll = 0
		return m, nil

	case agentDoneMsg:
		m.busy = false
		m.usage = msg.usage
		if msg.err != nil {
			m.messages = append(m.messages, &chat.Message{
				Kind:    chat.KindError,
				Content: msg.err.Error(),
			})
		}
		cmds = append(cmds, m.input.Focus())
		return m, tea.Batch(cmds...)
	}

	// Forward to textarea when not busy.
	if !m.busy {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch {
	case key == "ctrl+c":
		if m.busy && m.cancel != nil {
			m.cancel()
			m.busy = false
			return m, m.input.Focus()
		}
		return m, tea.Quit

	case key == "escape":
		if m.busy && m.cancel != nil {
			m.cancel()
			m.busy = false
			return m, m.input.Focus()
		}

	case key == "shift+enter" && !m.busy:
		// Insert newline for multiline input.
		m.input.InsertString("\n")
		h := min(5, strings.Count(m.input.Value(), "\n")+2)
		m.input.SetHeight(h)
		return m, nil

	case key == "enter" && !m.busy:
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m, nil
		}
		if text == "/quit" || text == "/exit" {
			return m, tea.Quit
		}
		if text == "/clear" {
			m.messages = nil
			m.scroll = 0
			m.input.Reset()
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
		m.input.Blur()
		m.busy = true
		m.startTime = time.Now()
		m.scroll = 0

		m.messages = append(m.messages, &chat.Message{
			Kind:    chat.KindUser,
			Content: text,
		})

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

	// Forward unhandled keys to the textarea when not busy.
	if !m.busy {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *Model) runAgent(prompt string) tea.Cmd {
	p := m.prog

	// Ensure agent exists with hooks for TUI visibility.
	if m.agent == nil {
		hooks := core.Hook{
			OnModelResponse: func(_ context.Context, _ *core.RunContext, resp *core.ModelResponse) {
				if p == nil || resp == nil {
					return
				}
				for _, part := range resp.Parts {
					switch pt := part.(type) {
					case core.TextPart:
						if pt.Content != "" {
							p.Send(textDeltaMsg{text: pt.Content})
						}
					case core.ThinkingPart:
						if pt.Content != "" {
							p.Send(thinkingDeltaMsg{text: pt.Content})
						}
					}
				}
			},
			OnToolStart: func(_ context.Context, _ *core.RunContext, toolName, argsJSON string) {
				if p != nil {
					p.Send(toolCallMsg{name: toolName, args: argsJSON, rawArgs: argsJSON})
				}
			},
			OnToolEnd: func(_ context.Context, _ *core.RunContext, toolName, result string, _ error) {
				if p != nil {
					p.Send(toolResultMsg{name: toolName, result: result})
				}
			},
		}
		a, err := agent.New(m.cfg, m.activeSkills, core.WithHooks[string](hooks))
		if err != nil {
			return func() tea.Msg {
				return agentDoneMsg{err: err}
			}
		}
		m.agent = a
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	a := m.agent

	return func() tea.Msg {
		result, err := a.Run(ctx, prompt)
		if err != nil {
			return agentDoneMsg{err: err}
		}
		return agentDoneMsg{usage: result.Usage}
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

func (m *Model) finishLastTool(name, result string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Kind == chat.KindToolCall && m.messages[i].ToolName == name && m.messages[i].Status == chat.ToolRunning {
			m.messages[i].Status = chat.ToolSuccess
			break
		}
	}
	if result != "" {
		m.messages = append(m.messages, &chat.Message{
			Kind:     chat.KindToolResult,
			ToolName: name,
			Content:  result,
		})
	}
}

func extractMainParam(argsJSON string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	for _, key := range []string{"command", "file_path", "pattern", "path", "content"} {
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
	inputHeight := 1
	if !m.busy {
		inputHeight = strings.Count(m.input.Value(), "\n") + 2
		if inputHeight > 6 {
			inputHeight = 6
		}
	}
	chatHeight := m.height - 3 - inputHeight
	if chatHeight < 1 {
		chatHeight = 1
	}
	sections = append(sections, m.renderChat(chatHeight))

	// Input area.
	if m.busy {
		sections = append(sections, m.renderBusyInput())
	} else {
		sections = append(sections, m.renderInput())
	}

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

func (m *Model) renderChat(height int) string {
	if len(m.messages) == 0 {
		greeting := m.sty.Muted.Render("  What can I help you with?")
		padding := strings.Repeat("\n", max(0, height-2))
		return padding + greeting
	}

	var allLines []string
	for _, msg := range m.messages {
		rendered := chat.RenderMessage(msg, m.sty, m.width, m.messages)
		if rendered != "" {
			allLines = append(allLines, strings.Split(rendered, "\n")...)
			allLines = append(allLines, "")
		}
	}

	// Clamp scroll to valid range.
	maxScroll := len(allLines) - height
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}

	endIdx := len(allLines) - m.scroll
	if endIdx < 0 {
		endIdx = 0
	}
	if endIdx > len(allLines) {
		endIdx = len(allLines)
	}
	startIdx := endIdx - height
	if startIdx < 0 {
		startIdx = 0
	}

	visible := allLines[startIdx:endIdx]
	for len(visible) < height {
		visible = append([]string{""}, visible...)
	}

	return strings.Join(visible, "\n")
}

func (m *Model) renderInput() string {
	prompt := m.sty.Input.Prompt.Render(" > ")
	return prompt + m.input.View()
}

func (m *Model) renderBusyInput() string {
	elapsed := time.Since(m.startTime).Truncate(time.Second)
	sp := m.spinner.View()
	return m.sty.Muted.Render(fmt.Sprintf(" %s Working... %s  (Esc to cancel)", sp, elapsed))
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

	provider := m.sty.StatusBar.Provider.Render(string(m.cfg.Provider) + "/" + m.cfg.Model)
	leftParts = append(leftParts, divider, provider)

	left := lipgloss.JoinHorizontal(lipgloss.Top, leftParts...)

	// Help hints on the right.
	var hints string
	if m.busy {
		hints = m.sty.StatusBar.Key.Render("esc ") + m.sty.StatusBar.Value.Render("cancel")
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
	// Check if already active; if so, deactivate.
	for i, s := range m.activeSkills {
		if strings.EqualFold(s.Name, name) {
			m.activeSkills = append(m.activeSkills[:i], m.activeSkills[i+1:]...)
			m.agent = nil // force agent recreation without this skill
			return &chat.Message{
				Kind:    chat.KindAssistant,
				Content: fmt.Sprintf("Deactivated skill `%s`.", s.Name),
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
		Content: fmt.Sprintf("Activated skill `%s`. The agent will now use this skill's instructions.", s.Name),
	}
}
