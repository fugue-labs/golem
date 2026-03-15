package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/ui/chat"
)

// replayTickMsg advances replay by one event.
type replayTickMsg struct{}

// replayDoneMsg signals replay has finished.
type replayDoneMsg struct{}

func replayEmptyState(includeListHint bool) string {
	lines := []string{
		"No replay traces found yet for this project.",
		"Traces are recorded automatically after agent runs.",
		"Run a prompt first, then use `/replay` for the latest trace.",
	}
	if includeListHint {
		lines = append(lines, "Use `/replay list` to inspect saved traces.")
	}
	lines = append(lines, "Use `/resume` after `/clear` to restore the last saved session state.")
	return strings.Join(lines, "\n")
}

func replayStartSummary(trace *agent.ReplayTrace) string {
	provider := strings.TrimSpace(trace.Provider)
	if provider == "" {
		provider = "unknown provider"
	}
	model := strings.TrimSpace(trace.Model)
	if model == "" {
		model = "unknown model"
	}
	stateSummary := replayTraceStateSummary(trace)
	return strings.Join([]string{
		fmt.Sprintf("Replaying session from %s.", trace.StartTime.Format("Jan 2 15:04")),
		fmt.Sprintf("Model: %s via %s.", model, provider),
		fmt.Sprintf("%d events. %s Press Esc to stop.", len(trace.Events), stateSummary),
	}, "\n")
}

func replayTraceStateSummary(trace *agent.ReplayTrace) string {
	if trace == nil || len(trace.Events) == 0 {
		return "No recorded messages yet."
	}

	hasUser := false
	hasAssistant := false
	hasThinking := false
	hasTool := false
	for _, event := range trace.Events {
		switch event.Kind {
		case agent.EventUserInput:
			hasUser = true
		case agent.EventTextDelta:
			hasAssistant = true
		case agent.EventThinkDelta:
			hasThinking = true
		case agent.EventToolCall, agent.EventToolResult:
			hasTool = true
		}
	}

	parts := make([]string, 0, 4)
	if hasUser {
		parts = append(parts, "Includes user prompts")
	}
	if hasAssistant {
		parts = append(parts, "assistant output")
	}
	if hasThinking {
		parts = append(parts, "thinking traces")
	}
	if hasTool {
		parts = append(parts, "tool activity")
	}
	if len(parts) == 0 {
		return "No recorded messages yet."
	}
	return joinReplaySummaryParts(parts) + "."
}

func joinReplaySummaryParts(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + ", and " + parts[len(parts)-1]
	}
}

func replayTraceInfoStateSummary(events int) string {
	if events <= 0 {
		return "No recorded events yet."
	}
	if events == 1 {
		return "One recorded event ready to replay."
	}
	return fmt.Sprintf("Recorded interaction ready to replay from %d events.", events)
}

// handleReplayCommand processes /replay commands.
func (m *Model) handleReplayCommand(text string) (*chat.Message, tea.Cmd) {
	if m.busy {
		return &chat.Message{Kind: chat.KindAssistant, Content: "Cannot replay while agent is running."}, nil
	}
	if m.replayMode {
		return &chat.Message{Kind: chat.KindAssistant, Content: "Replay already in progress. Press Esc to stop."}, nil
	}

	arg := strings.TrimSpace(strings.TrimPrefix(text, "/replay"))

	if arg == "list" {
		return m.listTraces(), nil
	}

	// Load trace — from specific file or latest.
	var trace *agent.ReplayTrace
	var err error
	if arg != "" {
		// Try as filename in session dir.
		dir, dirErr := agent.SessionDir(m.cfg.WorkingDir)
		if dirErr != nil {
			return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Session dir error: %v", dirErr)}, nil
		}
		trace, err = agent.LoadTrace(filepath.Join(dir, arg))
	} else {
		trace, err = agent.LoadLatestTrace(m.cfg.WorkingDir)
	}

	if err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to load trace: %v", err)}, nil
	}
	if trace == nil {
		return &chat.Message{Kind: chat.KindAssistant, Content: replayEmptyState(true)}, nil
	}

	return m.startReplay(trace)
}

// listTraces shows available trace files.
func (m *Model) listTraces() *chat.Message {
	traces, err := agent.ListTraces(m.cfg.WorkingDir)
	if err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to list traces: %v", err)}
	}
	if len(traces) == 0 {
		return &chat.Message{Kind: chat.KindAssistant, Content: replayEmptyState(false)}
	}

	var b strings.Builder
	b.WriteString("**Saved replay traces**\n\n")
	for i, t := range traces {
		provider := strings.TrimSpace(t.Provider)
		if provider == "" {
			provider = "unknown provider"
		}
		model := strings.TrimSpace(t.Model)
		if model == "" {
			model = "unknown model"
		}
		traceSummary := fmt.Sprintf("%s · %s via %s · %d events", t.Timestamp.Format("Jan 2 15:04"), model, provider, t.Events)
		stateSummary := replayTraceInfoStateSummary(t.Events)
		label := "saved"
		if i == len(traces)-1 {
			label = "latest"
		}
		fmt.Fprintf(&b, "- `%s` — %s · %s\n", t.Filename, traceSummary, label)
		fmt.Fprintf(&b, "  %s\n", stateSummary)
	}
	b.WriteString("\nUse `/replay <filename>` to replay a specific trace, `/replay` for the latest trace, or `/resume` after `/clear` to restore the latest saved session state.")
	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

// startReplay initializes replay mode and begins feeding events.
func (m *Model) startReplay(trace *agent.ReplayTrace) (*chat.Message, tea.Cmd) {
	m.replayMode = true
	m.replayTrace = trace
	m.replayIdx = 0
	m.replayStart = time.Now()
	m.busy = true

	msg := &chat.Message{
		Kind:    chat.KindSystem,
		Content: replayStartSummary(trace),
	}

	return msg, m.replayNext()
}

// replayNext returns a tea.Cmd that schedules the next replay event.
func (m *Model) replayNext() tea.Cmd {
	if m.replayIdx >= len(m.replayTrace.Events) {
		return func() tea.Msg { return replayDoneMsg{} }
	}

	event := m.replayTrace.Events[m.replayIdx]

	// Calculate delay: use real timing but cap at 2 seconds to keep replay snappy.
	var delay time.Duration
	if m.replayIdx > 0 {
		prev := m.replayTrace.Events[m.replayIdx-1]
		delay = time.Duration(event.OffsetMs-prev.OffsetMs) * time.Millisecond
		if delay > 2*time.Second {
			delay = 2 * time.Second
		}
		if delay < 0 {
			delay = 0
		}
	}

	// For text deltas, use minimal delay to feel like streaming.
	if event.Kind == agent.EventTextDelta || event.Kind == agent.EventThinkDelta {
		delay = 10 * time.Millisecond
	}

	if delay == 0 {
		return func() tea.Msg { return replayTickMsg{} }
	}
	return tea.Tick(delay, func(_ time.Time) tea.Msg {
		return replayTickMsg{}
	})
}

// handleReplayTick processes one replay event and schedules the next.
func (m *Model) handleReplayTick() (tea.Model, tea.Cmd) {
	if !m.replayMode || m.replayIdx >= len(m.replayTrace.Events) {
		return m, func() tea.Msg { return replayDoneMsg{} }
	}

	event := m.replayTrace.Events[m.replayIdx]
	m.replayIdx++

	switch event.Kind {
	case agent.EventUserInput:
		data, err := agent.DecodeEvent[agent.UserInputData](event)
		if err != nil {
			break
		}
		m.messages = append(m.messages, &chat.Message{
			Kind:    chat.KindUser,
			Content: data.Text,
		})
		m.scroll = 0

	case agent.EventTextDelta:
		data, err := agent.DecodeEvent[agent.TextDeltaData](event)
		if err != nil {
			break
		}
		m.appendOrUpdateAssistant(data.Text)
		m.scroll = 0

	case agent.EventThinkDelta:
		data, err := agent.DecodeEvent[agent.ThinkDeltaData](event)
		if err != nil {
			break
		}
		m.appendOrUpdateThinking(data.Text)
		m.scroll = 0

	case agent.EventToolCall:
		data, err := agent.DecodeEvent[agent.ToolCallData](event)
		if err != nil {
			break
		}
		toolMsg := &chat.Message{
			Kind:      chat.KindToolCall,
			CallID:    data.CallID,
			ToolName:  data.Name,
			ToolArgs:  extractMainParam(data.Args),
			RawArgs:   data.RawArgs,
			Status:    chat.ToolRunning,
			StartedAt: time.Now(),
		}
		m.messages = append(m.messages, toolMsg)
		m.activeToolName = data.Name
		m.activeToolArgs = extractMainParam(data.Args)
		m.scroll = 0

	case agent.EventToolResult:
		data, err := agent.DecodeEvent[agent.ToolResultData](event)
		if err != nil {
			break
		}
		m.activeToolName = ""
		m.activeToolArgs = ""
		m.finishLastTool(data.CallID, data.Name, data.Result, data.Error)
		m.scroll = 0

	case agent.EventAgentDone:
		data, err := agent.DecodeEvent[agent.AgentDoneData](event)
		if err != nil {
			break
		}
		usageParts := []string{
			fmt.Sprintf("%d↓ %d↑", data.InputTokens, data.OutputTokens),
			fmt.Sprintf("%d tools", data.ToolCalls),
		}
		if data.Error != "" {
			usageParts = append(usageParts, "error: "+data.Error)
		}
		m.messages = append(m.messages, &chat.Message{
			Kind:    chat.KindSystem,
			Content: strings.Join(usageParts, " · "),
		})
		m.scroll = 0

	case agent.EventSystem:
		data, err := agent.DecodeEvent[agent.SystemEventData](event)
		if err != nil {
			break
		}
		m.messages = append(m.messages, &chat.Message{
			Kind:    chat.KindSystem,
			Content: data.Text,
		})
		m.scroll = 0

	case agent.EventError:
		data, err := agent.DecodeEvent[agent.ErrorEventData](event)
		if err != nil {
			break
		}
		m.messages = append(m.messages, &chat.Message{
			Kind:    chat.KindError,
			Content: data.Text,
		})
		m.scroll = 0
	}

	return m, m.replayNext()
}

// handleReplayDone finalizes replay mode.
func (m *Model) handleReplayDone() (tea.Model, tea.Cmd) {
	m.replayMode = false
	m.replayTrace = nil
	m.replayIdx = 0
	m.busy = false
	m.activeToolName = ""
	m.activeToolArgs = ""
	m.messages = append(m.messages, &chat.Message{
		Kind:    chat.KindSystem,
		Content: "Replay complete.",
	})
	m.scroll = 0
	return m, m.input.Focus()
}

// stopReplay cancels an active replay.
func (m *Model) stopReplay() {
	m.replayMode = false
	m.replayTrace = nil
	m.replayIdx = 0
	m.busy = false
	m.activeToolName = ""
	m.activeToolArgs = ""
	m.messages = append(m.messages, &chat.Message{
		Kind:    chat.KindSystem,
		Content: "Replay stopped.",
	})
	m.scroll = 0
}

// recordEvent records a single event to the active trace if recording.
func (m *Model) recordEvent(kind agent.ReplayEventKind, payload any) {
	if m.trace != nil {
		m.trace.Record(kind, payload)
	}
}

// startRecording begins trace recording for the current session.
func (m *Model) startRecording() {
	m.trace = agent.NewTrace(m.cfg.Model, string(m.cfg.Provider), m.cfg.WorkingDir)
}

// flushTrace saves the current trace to disk and resets it.
func (m *Model) flushTrace() {
	if m.trace != nil && len(m.trace.Events) > 0 {
		go func(t *agent.ReplayTrace, wd string) {
			_ = t.Save(wd)
		}(m.trace, m.cfg.WorkingDir)
	}
	m.trace = nil
}
