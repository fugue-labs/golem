package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/ui/chat"
)

// replayTUIModel is a minimal bubbletea model wrapping replay logic
// for integration testing of the replay TUI flow.
type replayTUIModel struct {
	messages       []*chat.Message
	replayMode     bool
	replayTrace    *agent.ReplayTrace
	replayIdx      int
	replayStart    time.Time
	busy           bool
	activeToolName string
	activeToolArgs string
	input          string
	quitting       bool
}

func newReplayTUIModel() replayTUIModel {
	return replayTUIModel{}
}

func normalizeTeatestOutput(bts []byte) string {
	s := strings.ReplaceAll(string(bts), "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func outputContainsText(bts []byte, want string) bool {
	want = strings.ReplaceAll(want, "\r\n", "\n")
	return strings.Contains(normalizeTeatestOutput(bts), want)
}

func outputContainsLines(bts []byte, want string) bool {
	for _, line := range strings.Split(strings.ReplaceAll(want, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.Contains(normalizeTeatestOutput(bts), line) {
			return false
		}
	}
	return true
}

func outputContainsReplayEmptyState(bts []byte, includeListHint bool) bool {
	want := replayEmptyState(includeListHint)
	lines := strings.Split(strings.ReplaceAll(want, "\r\n", "\n"), "\n")
	if len(lines) < 2 {
		return outputContainsText(bts, want)
	}
	if !strings.Contains(normalizeTeatestOutput(bts), lines[0]) {
		return false
	}
	if !strings.Contains(normalizeTeatestOutput(bts), lines[1]) {
		return false
	}
	if !strings.Contains(normalizeTeatestOutput(bts), "/resume") {
		return false
	}
	if includeListHint && !strings.Contains(normalizeTeatestOutput(bts), "/replay list") {
		return false
	}
	return true
}

func outputContainsReplayStartSummary(bts []byte, trace *agent.ReplayTrace) bool {
	want := replayStartSummary(trace)
	for _, line := range strings.Split(strings.ReplaceAll(want, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.Contains(normalizeTeatestOutput(bts), line) {
			return false
		}
	}
	return true
}

func (m replayTUIModel) Init() tea.Cmd { return nil }

func (m replayTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "escape":
			if m.replayMode {
				m.stopReplay()
				return m, nil
			}
			return m, nil
		case "enter":
			return m.handleInput()
		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
			return m, nil
		default:
			if len(msg.Text) == 1 {
				m.input += msg.Text
			}
			return m, nil
		}

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

	case startReplayMsg:
		return m.startReplay(msg.trace)
	}

	return m, nil
}

// startReplayMsg is a tea.Msg that triggers replay start with a given trace.
type startReplayMsg struct {
	trace *agent.ReplayTrace
}

func (m replayTUIModel) View() tea.View {
	var b strings.Builder

	// Header: replay state.
	if m.replayMode {
		total := 0
		if m.replayTrace != nil {
			total = len(m.replayTrace.Events)
		}
		b.WriteString(fmt.Sprintf("REPLAY [%d/%d]", m.replayIdx, total))
		if m.busy {
			b.WriteString(" playing")
		}
		b.WriteString("\n")
	} else {
		b.WriteString("IDLE\n")
	}

	b.WriteString(strings.Repeat("-", 40) + "\n")

	// Messages.
	for _, msg := range m.messages {
		switch msg.Kind {
		case chat.KindUser:
			b.WriteString("USER: " + msg.Content + "\n")
		case chat.KindAssistant:
			b.WriteString("ASSISTANT: " + msg.Content + "\n")
		case chat.KindThinking:
			b.WriteString("THINKING: " + msg.Content + "\n")
		case chat.KindToolCall:
			status := "running"
			if msg.Status == chat.ToolSuccess {
				status = "done"
			} else if msg.Status == chat.ToolError {
				status = "error"
			}
			b.WriteString(fmt.Sprintf("TOOL[%s]: %s (%s)\n", status, msg.ToolName, msg.ToolArgs))
		case chat.KindSystem:
			b.WriteString("SYSTEM: " + msg.Content + "\n")
		case chat.KindError:
			b.WriteString("ERROR: " + msg.Content + "\n")
		}
	}

	b.WriteString(strings.Repeat("-", 40) + "\n")
	b.WriteString("> " + m.input)

	return tea.NewView(b.String())
}

// handleInput processes input text when Enter is pressed.
func (m replayTUIModel) handleInput() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input)
	m.input = ""

	if text == "" {
		return m, nil
	}

	if text == "/replay" || strings.HasPrefix(text, "/replay ") {
		msg, cmd := m.handleReplayCommand(text)
		m.messages = append(m.messages, msg)
		return m, cmd
	}

	m.messages = append(m.messages, &chat.Message{
		Kind:    chat.KindSystem,
		Content: fmt.Sprintf("unknown command: %s", text),
	})
	return m, nil
}

// handleReplayCommand processes /replay commands.
func (m *replayTUIModel) handleReplayCommand(text string) (*chat.Message, tea.Cmd) {
	if m.busy {
		return &chat.Message{Kind: chat.KindAssistant, Content: "Cannot replay while agent is running."}, nil
	}
	if m.replayMode {
		return &chat.Message{Kind: chat.KindAssistant, Content: "Replay already in progress. Press Esc to stop."}, nil
	}

	arg := strings.TrimSpace(strings.TrimPrefix(text, "/replay"))

	if arg == "list" {
		return &chat.Message{Kind: chat.KindAssistant, Content: replayEmptyState(false)}, nil
	}

	// For test purposes, we don't load from files — traces are injected via startReplayMsg.
	return &chat.Message{Kind: chat.KindAssistant, Content: replayEmptyState(true)}, nil
}

// startReplay initializes replay mode and begins feeding events.
func (m replayTUIModel) startReplay(trace *agent.ReplayTrace) (tea.Model, tea.Cmd) {
	m.replayMode = true
	m.replayTrace = trace
	m.replayIdx = 0
	m.replayStart = time.Now()
	m.busy = true

	msg := &chat.Message{
		Kind:    chat.KindSystem,
		Content: replayStartSummary(trace),
	}
	m.messages = append(m.messages, msg)

	return m, m.replayNext()
}

// replayNext returns a tea.Cmd that schedules the next replay event.
func (m *replayTUIModel) replayNext() tea.Cmd {
	if m.replayIdx >= len(m.replayTrace.Events) {
		return func() tea.Msg { return replayDoneMsg{} }
	}
	// In tests, use zero delay to make replay instant.
	return func() tea.Msg { return replayTickMsg{} }
}

// handleReplayTick processes one replay event and schedules the next.
func (m replayTUIModel) handleReplayTick() (tea.Model, tea.Cmd) {
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

	case agent.EventTextDelta:
		data, err := agent.DecodeEvent[agent.TextDeltaData](event)
		if err != nil {
			break
		}
		m.appendOrUpdateAssistant(data.Text)

	case agent.EventThinkDelta:
		data, err := agent.DecodeEvent[agent.ThinkDeltaData](event)
		if err != nil {
			break
		}
		m.appendOrUpdateThinking(data.Text)

	case agent.EventToolCall:
		data, err := agent.DecodeEvent[agent.ToolCallData](event)
		if err != nil {
			break
		}
		toolMsg := &chat.Message{
			Kind:      chat.KindToolCall,
			CallID:    data.CallID,
			ToolName:  data.Name,
			ToolArgs:  data.Args,
			Status:    chat.ToolRunning,
			StartedAt: time.Now(),
		}
		m.messages = append(m.messages, toolMsg)
		m.activeToolName = data.Name
		m.activeToolArgs = data.Args

	case agent.EventToolResult:
		data, err := agent.DecodeEvent[agent.ToolResultData](event)
		if err != nil {
			break
		}
		m.activeToolName = ""
		m.activeToolArgs = ""
		m.finishLastTool(data.CallID, data.Result, data.Error)

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

	case agent.EventSystem:
		data, err := agent.DecodeEvent[agent.SystemEventData](event)
		if err != nil {
			break
		}
		m.messages = append(m.messages, &chat.Message{
			Kind:    chat.KindSystem,
			Content: data.Text,
		})

	case agent.EventError:
		data, err := agent.DecodeEvent[agent.ErrorEventData](event)
		if err != nil {
			break
		}
		m.messages = append(m.messages, &chat.Message{
			Kind:    chat.KindError,
			Content: data.Text,
		})
	}

	return m, m.replayNext()
}

// handleReplayDone finalizes replay mode.
func (m replayTUIModel) handleReplayDone() (tea.Model, tea.Cmd) {
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
	return m, nil
}

// stopReplay cancels an active replay.
func (m *replayTUIModel) stopReplay() {
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
}

// appendOrUpdateAssistant mirrors the real Model's method.
func (m *replayTUIModel) appendOrUpdateAssistant(delta string) {
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
}

// appendOrUpdateThinking mirrors the real Model's method.
func (m *replayTUIModel) appendOrUpdateThinking(delta string) {
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

// finishLastTool mirrors the real Model's method.
func (m *replayTUIModel) finishLastTool(callID, result, errText string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := m.messages[i]
		if msg.Kind != chat.KindToolCall || msg.Status != chat.ToolRunning {
			continue
		}
		if callID != "" && msg.CallID != callID {
			continue
		}
		if errText != "" {
			msg.Status = chat.ToolError
		} else {
			msg.Status = chat.ToolSuccess
		}
		if result != "" {
			msg.Content = result
		}
		return
	}
}

// makeEvent builds a ReplayEvent with the given kind and payload.
func makeEvent(kind agent.ReplayEventKind, offsetMs int64, payload any) agent.ReplayEvent {
	data, _ := json.Marshal(payload)
	return agent.ReplayEvent{
		Kind:     kind,
		OffsetMs: offsetMs,
		Data:     data,
	}
}

// makeTrace builds a synthetic ReplayTrace for testing.
func makeTrace(events []agent.ReplayEvent) *agent.ReplayTrace {
	return &agent.ReplayTrace{
		Version:   1,
		StartTime: time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC),
		Model:     "test-model",
		Provider:  "test-provider",
		WorkDir:   "/tmp/test",
		Events:    events,
	}
}

// --- Tests ---

// TestTeatestReplayCommandWhileBusy verifies /replay rejects while busy.
func TestTeatestReplayCommandWhileBusy(t *testing.T) {
	m := newReplayTUIModel()
	m.busy = true
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Type("/replay")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Cannot replay while agent is running.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplayCommandAlreadyPlaying verifies /replay rejects if already replaying.
func TestTeatestReplayCommandAlreadyPlaying(t *testing.T) {
	m := newReplayTUIModel()
	m.replayMode = true
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Type("/replay")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Replay already in progress. Press Esc to stop.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplayListCommand verifies /replay list works.
func TestTeatestReplayListCommand(t *testing.T) {
	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Type("/replay list")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return outputContainsReplayEmptyState(bts, false)
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplayNoTraces verifies /replay with no available traces.
func TestTeatestReplayNoTraces(t *testing.T) {
	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 24))

	tm.Type("/replay")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return outputContainsReplayEmptyState(bts, true)
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplayStartAndComplete verifies a full replay from start to completion.
func TestTeatestReplayStartAndComplete(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{
		makeEvent(agent.EventUserInput, 0, agent.UserInputData{Text: "hello world"}),
		makeEvent(agent.EventTextDelta, 100, agent.TextDeltaData{Text: "Hi there! "}),
		makeEvent(agent.EventTextDelta, 110, agent.TextDeltaData{Text: "How can I help?"}),
	})

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Start replay via message.
	tm.Send(startReplayMsg{trace: trace})

	// Wait for full replay completion — all assertions in one WaitFor to avoid
	// consuming the output buffer across multiple calls.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Replay complete.") &&
			strings.Contains(s, "USER: hello world") &&
			strings.Contains(s, "Hi there! How can I help?")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplayUserInputEvent verifies user input events render correctly.
func TestTeatestReplayUserInputEvent(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{
		makeEvent(agent.EventUserInput, 0, agent.UserInputData{Text: "first prompt"}),
		makeEvent(agent.EventUserInput, 500, agent.UserInputData{Text: "second prompt"}),
	})

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(startReplayMsg{trace: trace})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "USER: first prompt") &&
			strings.Contains(s, "USER: second prompt") &&
			strings.Contains(s, "Replay complete.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplayThinkingEvent verifies thinking deltas are rendered.
func TestTeatestReplayThinkingEvent(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{
		makeEvent(agent.EventUserInput, 0, agent.UserInputData{Text: "think about this"}),
		makeEvent(agent.EventThinkDelta, 50, agent.ThinkDeltaData{Text: "Let me "}),
		makeEvent(agent.EventThinkDelta, 60, agent.ThinkDeltaData{Text: "consider..."}),
		makeEvent(agent.EventTextDelta, 200, agent.TextDeltaData{Text: "Here's my answer."}),
	})

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(startReplayMsg{trace: trace})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "THINKING: Let me consider...") &&
			strings.Contains(s, "ASSISTANT: Here's my answer.") &&
			strings.Contains(s, "Replay complete.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplayToolCallAndResult verifies tool call/result events.
func TestTeatestReplayToolCallAndResult(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{
		makeEvent(agent.EventUserInput, 0, agent.UserInputData{Text: "read file"}),
		makeEvent(agent.EventToolCall, 100, agent.ToolCallData{
			CallID: "call_1", Name: "read", Args: "/tmp/test.go",
		}),
		makeEvent(agent.EventToolResult, 200, agent.ToolResultData{
			CallID: "call_1", Name: "read", Result: "package main",
		}),
		makeEvent(agent.EventTextDelta, 300, agent.TextDeltaData{Text: "File contains Go code."}),
	})

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(startReplayMsg{trace: trace})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "TOOL[done]: read") &&
			strings.Contains(s, "ASSISTANT: File contains Go code.") &&
			strings.Contains(s, "Replay complete.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplayToolError verifies tool error events render correctly.
func TestTeatestReplayToolError(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{
		makeEvent(agent.EventUserInput, 0, agent.UserInputData{Text: "run cmd"}),
		makeEvent(agent.EventToolCall, 100, agent.ToolCallData{
			CallID: "call_err", Name: "bash", Args: "exit 1",
		}),
		makeEvent(agent.EventToolResult, 200, agent.ToolResultData{
			CallID: "call_err", Name: "bash", Result: "", Error: "exit status 1",
		}),
	})

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(startReplayMsg{trace: trace})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "TOOL[error]: bash") &&
			strings.Contains(s, "Replay complete.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplayAgentDone verifies agent done events with usage stats.
func TestTeatestReplayAgentDone(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{
		makeEvent(agent.EventUserInput, 0, agent.UserInputData{Text: "do task"}),
		makeEvent(agent.EventTextDelta, 100, agent.TextDeltaData{Text: "Done."}),
		makeEvent(agent.EventAgentDone, 200, agent.AgentDoneData{
			InputTokens: 1000, OutputTokens: 500, ToolCalls: 3,
		}),
	})

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(startReplayMsg{trace: trace})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "1000") &&
			strings.Contains(s, "500") &&
			strings.Contains(s, "3 tools") &&
			strings.Contains(s, "Replay complete.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplayAgentDoneWithError verifies agent done with error.
func TestTeatestReplayAgentDoneWithError(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{
		makeEvent(agent.EventAgentDone, 0, agent.AgentDoneData{
			InputTokens: 100, OutputTokens: 50, ToolCalls: 0, Error: "context deadline exceeded",
		}),
	})

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(startReplayMsg{trace: trace})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "error: context deadline exceeded") &&
			strings.Contains(s, "Replay complete.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplaySystemEvent verifies system events render.
func TestTeatestReplaySystemEvent(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{
		makeEvent(agent.EventSystem, 0, agent.SystemEventData{Text: "Session initialized"}),
		makeEvent(agent.EventSystem, 100, agent.SystemEventData{Text: "Loading context..."}),
	})

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(startReplayMsg{trace: trace})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "SYSTEM: Session initialized") &&
			strings.Contains(s, "SYSTEM: Loading context...") &&
			strings.Contains(s, "Replay complete.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplayErrorEvent verifies error events render.
func TestTeatestReplayErrorEvent(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{
		makeEvent(agent.EventError, 0, agent.ErrorEventData{Text: "API rate limit exceeded"}),
	})

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(startReplayMsg{trace: trace})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "ERROR: API rate limit exceeded") &&
			strings.Contains(s, "Replay complete.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplayStopEscape verifies Esc stops replay mid-stream.
func TestTeatestReplayStopEscape(t *testing.T) {
	// Build a trace with many events so we can stop mid-replay.
	var events []agent.ReplayEvent
	events = append(events, makeEvent(agent.EventUserInput, 0, agent.UserInputData{Text: "start"}))
	for i := 0; i < 50; i++ {
		events = append(events, makeEvent(agent.EventTextDelta, int64(100+i*10),
			agent.TextDeltaData{Text: fmt.Sprintf("chunk%d ", i)}))
	}
	trace := makeTrace(events)

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(startReplayMsg{trace: trace})

	// Press Escape to stop.
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEscape})

	// Wait for "Replay stopped." or "Replay complete." — either is valid since
	// the replay may have already finished by the time Esc is processed.
	// Also verify IDLE state in single WaitFor.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "IDLE") &&
			(strings.Contains(s, "Replay stopped.") || strings.Contains(s, "Replay complete."))
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplayStateDisplay verifies the header shows correct replay state.
func TestTeatestReplayStateDisplay(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{
		makeEvent(agent.EventUserInput, 0, agent.UserInputData{Text: "test"}),
		makeEvent(agent.EventTextDelta, 100, agent.TextDeltaData{Text: "response"}),
	})

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Start replay.
	tm.Send(startReplayMsg{trace: trace})

	// After completion, should return to IDLE with replay complete.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "IDLE") &&
			strings.Contains(s, "Replay complete.") &&
			strings.Contains(s, "USER: test") &&
			strings.Contains(s, "response")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplayEmptyTrace verifies replaying a trace with zero events.
func TestTeatestReplayEmptyTrace(t *testing.T) {
	trace := makeTrace(nil)

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(startReplayMsg{trace: trace})

	// Should immediately complete.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := normalizeTeatestOutput(bts)
		return outputContainsReplayStartSummary(bts, trace) &&
			strings.Contains(s, "Replay complete.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplayTextDeltaStreaming verifies text deltas accumulate on the same message.
func TestTeatestReplayTextDeltaStreaming(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{
		makeEvent(agent.EventUserInput, 0, agent.UserInputData{Text: "hello"}),
		makeEvent(agent.EventTextDelta, 10, agent.TextDeltaData{Text: "Hello"}),
		makeEvent(agent.EventTextDelta, 20, agent.TextDeltaData{Text: " World"}),
		makeEvent(agent.EventTextDelta, 30, agent.TextDeltaData{Text: "!"}),
	})

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(startReplayMsg{trace: trace})

	// Verify all deltas accumulated into single assistant message.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "ASSISTANT: Hello World!") &&
			strings.Contains(s, "Replay complete.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestReplayFullSession verifies a realistic multi-turn session replay.
func TestTeatestReplayFullSession(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{
		makeEvent(agent.EventSystem, 0, agent.SystemEventData{Text: "Session started"}),
		makeEvent(agent.EventUserInput, 100, agent.UserInputData{Text: "list files"}),
		makeEvent(agent.EventThinkDelta, 200, agent.ThinkDeltaData{Text: "I'll use bash to list files"}),
		makeEvent(agent.EventToolCall, 300, agent.ToolCallData{
			CallID: "tc_1", Name: "bash", Args: "ls -la",
		}),
		makeEvent(agent.EventToolResult, 500, agent.ToolResultData{
			CallID: "tc_1", Name: "bash", Result: "file1.go\nfile2.go",
		}),
		makeEvent(agent.EventTextDelta, 600, agent.TextDeltaData{Text: "Found 2 Go files."}),
		makeEvent(agent.EventAgentDone, 700, agent.AgentDoneData{
			InputTokens: 2000, OutputTokens: 800, ToolCalls: 1,
		}),
	})

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(startReplayMsg{trace: trace})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "SYSTEM: Session started") &&
			strings.Contains(s, "USER: list files") &&
			strings.Contains(s, "THINKING: I'll use bash to list files") &&
			strings.Contains(s, "TOOL[done]: bash") &&
			strings.Contains(s, "ASSISTANT: Found 2 Go files.") &&
			strings.Contains(s, "2000") &&
			strings.Contains(s, "1 tools") &&
			strings.Contains(s, "Replay complete.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}
