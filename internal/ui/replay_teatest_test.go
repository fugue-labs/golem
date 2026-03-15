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

	for _, msg := range m.messages {
		switch msg.Kind {
		case chat.KindUser:
			b.WriteString("[USER] " + msg.Content + "\n")
		case chat.KindAssistant:
			b.WriteString("[ASSISTANT streaming] " + msg.Content + "\n")
		case chat.KindThinking:
			b.WriteString("[THINKING] " + msg.Content + "\n")
		case chat.KindToolCall:
			status := "running"
			if msg.Status == chat.ToolSuccess {
				status = "done"
			} else if msg.Status == chat.ToolError {
				status = "error"
			}
			line := fmt.Sprintf("[TOOL %s] %s (%s)", status, msg.ToolName, msg.ToolArgs)
			if msg.Content != "" {
				line += " => " + strings.ReplaceAll(msg.Content, "\n", " | ")
			}
			b.WriteString(line + "\n")
		case chat.KindSystem:
			b.WriteString("[SUMMARY] " + msg.Content + "\n")
		case chat.KindError:
			b.WriteString("[ERROR] " + msg.Content + "\n")
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
		return &chat.Message{Kind: chat.KindAssistant, Content: "No replay traces found."}, nil
	}

	return &chat.Message{Kind: chat.KindAssistant, Content: "No replay traces found. Traces are recorded automatically during sessions."}, nil
}

// startReplay initializes replay mode and begins feeding events.
func (m replayTUIModel) startReplay(trace *agent.ReplayTrace) (tea.Model, tea.Cmd) {
	m.replayMode = true
	m.replayTrace = trace
	m.replayIdx = 0
	m.replayStart = time.Now()
	m.busy = true

	msg := &chat.Message{
		Kind: chat.KindSystem,
		Content: fmt.Sprintf("Replaying session from %s (%s, %d events)",
			trace.StartTime.Format("Jan 2 15:04"),
			trace.Model,
			len(trace.Events),
		),
	}
	m.messages = append(m.messages, msg)

	return m, m.replayNext()
}

// replayNext returns a tea.Cmd that schedules the next replay event.
func (m *replayTUIModel) replayNext() tea.Cmd {
	if m.replayIdx >= len(m.replayTrace.Events) {
		return func() tea.Msg { return replayDoneMsg{} }
	}
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
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindUser, Content: data.Text})

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
		toolMsg := &chat.Message{Kind: chat.KindToolCall, CallID: data.CallID, ToolName: data.Name, ToolArgs: data.Args, Status: chat.ToolRunning, StartedAt: time.Now()}
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
		usageParts := []string{fmt.Sprintf("%d↓ %d↑", data.InputTokens, data.OutputTokens), fmt.Sprintf("%d tools", data.ToolCalls)}
		if data.Error != "" {
			usageParts = append(usageParts, "error: "+data.Error)
		}
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindSystem, Content: strings.Join(usageParts, " · ")})

	case agent.EventSystem:
		data, err := agent.DecodeEvent[agent.SystemEventData](event)
		if err != nil {
			break
		}
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindSystem, Content: data.Text})

	case agent.EventError:
		data, err := agent.DecodeEvent[agent.ErrorEventData](event)
		if err != nil {
			break
		}
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindError, Content: data.Text})
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
	m.messages = append(m.messages, &chat.Message{Kind: chat.KindSystem, Content: "Replay complete."})
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
	m.messages = append(m.messages, &chat.Message{Kind: chat.KindSystem, Content: "Replay stopped."})
}

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
	m.messages = append(m.messages, &chat.Message{Kind: chat.KindAssistant, Content: delta})
}

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
	m.messages = append(m.messages, &chat.Message{Kind: chat.KindThinking, Content: delta})
}

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
			msg.Content = errText
		} else {
			msg.Status = chat.ToolSuccess
			if result != "" {
				msg.Content = result
			}
		}
		return
	}
}

func makeEvent(kind agent.ReplayEventKind, offsetMs int64, payload any) agent.ReplayEvent {
	data, _ := json.Marshal(payload)
	return agent.ReplayEvent{Kind: kind, OffsetMs: offsetMs, Data: data}
}

func makeTrace(events []agent.ReplayEvent) *agent.ReplayTrace {
	return &agent.ReplayTrace{Version: 1, StartTime: time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC), Model: "test-model", Provider: "test-provider", WorkDir: "/tmp/test", Events: events}
}

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

func TestTeatestReplayListCommand(t *testing.T) {
	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Type("/replay list")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "No replay traces found.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

func TestTeatestReplayNoTraces(t *testing.T) {
	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 24))

	tm.Type("/replay")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "No replay traces found.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

func TestTeatestReplayStartAndComplete(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{
		makeEvent(agent.EventUserInput, 0, agent.UserInputData{Text: "hello world"}),
		makeEvent(agent.EventTextDelta, 100, agent.TextDeltaData{Text: "Hi there! "}),
		makeEvent(agent.EventTextDelta, 110, agent.TextDeltaData{Text: "How can I help?"}),
	})

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	tm.Send(startReplayMsg{trace: trace})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Replay complete.") &&
			strings.Contains(s, "[USER] hello world") &&
			strings.Contains(s, "Hi there! How can I help?")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

func TestTeatestReplayThinkingAndToolStates(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{
		makeEvent(agent.EventSystem, 0, agent.SystemEventData{Text: "Session started"}),
		makeEvent(agent.EventUserInput, 100, agent.UserInputData{Text: "list files"}),
		makeEvent(agent.EventThinkDelta, 200, agent.ThinkDeltaData{Text: "I'll use bash"}),
		makeEvent(agent.EventToolCall, 300, agent.ToolCallData{CallID: "tc_1", Name: "bash", Args: "ls -la"}),
		makeEvent(agent.EventToolResult, 500, agent.ToolResultData{CallID: "tc_1", Name: "bash", Result: "file1.go\nfile2.go"}),
		makeEvent(agent.EventTextDelta, 600, agent.TextDeltaData{Text: "Found 2 Go files."}),
		makeEvent(agent.EventAgentDone, 700, agent.AgentDoneData{InputTokens: 2000, OutputTokens: 800, ToolCalls: 1}),
	})

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	tm.Send(startReplayMsg{trace: trace})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "[SUMMARY] Session started") &&
			strings.Contains(s, "[USER] list files") &&
			strings.Contains(s, "[THINKING] I'll use bash") &&
			strings.Contains(s, "[TOOL done] bash (ls -la) => file1.go | file2.go") &&
			strings.Contains(s, "[ASSISTANT streaming] Found 2 Go files.") &&
			strings.Contains(s, "2000") &&
			strings.Contains(s, "1 tools") &&
			strings.Contains(s, "Replay complete.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

func TestTeatestReplayErrorEvent(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{makeEvent(agent.EventError, 0, agent.ErrorEventData{Text: "API rate limit exceeded"})})

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	tm.Send(startReplayMsg{trace: trace})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "[ERROR] API rate limit exceeded") && strings.Contains(s, "Replay complete.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

func TestTeatestReplayStopEscape(t *testing.T) {
	var events []agent.ReplayEvent
	events = append(events, makeEvent(agent.EventUserInput, 0, agent.UserInputData{Text: "start"}))
	for i := 0; i < 50; i++ {
		events = append(events, makeEvent(agent.EventTextDelta, int64(100+i*10), agent.TextDeltaData{Text: fmt.Sprintf("chunk%d ", i)}))
	}
	trace := makeTrace(events)

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	tm.Send(startReplayMsg{trace: trace})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEscape})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "IDLE") && (strings.Contains(s, "Replay stopped.") || strings.Contains(s, "Replay complete."))
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

func TestTeatestReplayStateDisplay(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{
		makeEvent(agent.EventUserInput, 0, agent.UserInputData{Text: "test"}),
		makeEvent(agent.EventTextDelta, 100, agent.TextDeltaData{Text: "response"}),
	})

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	tm.Send(startReplayMsg{trace: trace})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "IDLE") && strings.Contains(s, "Replay complete.") && strings.Contains(s, "[USER] test") && strings.Contains(s, "response")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

func TestTeatestReplayEmptyTrace(t *testing.T) {
	trace := makeTrace(nil)

	m := newReplayTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	tm.Send(startReplayMsg{trace: trace})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Replaying session from") && strings.Contains(s, "0 events") && strings.Contains(s, "Replay complete.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

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

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "[ASSISTANT streaming] Hello World!") && strings.Contains(s, "Replay complete.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}
