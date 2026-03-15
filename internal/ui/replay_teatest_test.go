package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/ui/chat"
)

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
	viewport       viewport.Model
	width          int
	height         int
}

func newReplayTUIModel() replayTUIModel {
	vp := viewport.New()
	vp.KeyMap = viewport.KeyMap{}
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 1
	return replayTUIModel{viewport: vp, width: 80, height: 24}
}

func (m replayTUIModel) Init() tea.Cmd { return nil }

func (m replayTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.syncViewport()
		return m, nil
	case tea.MouseWheelMsg:
		m.viewport, _ = m.viewport.Update(msg)
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "escape":
			if m.replayMode {
				m.stopReplay()
			}
			return m, nil
		case "pgup", "pgdown", "home", "end":
			m.viewport, _ = m.viewport.Update(msg)
			return m, nil
		case "enter":
			return m.handleInput()
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

type startReplayMsg struct{ trace *agent.ReplayTrace }

func (m *replayTUIModel) syncViewport() {
	bodyHeight := m.height - 3
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	if m.width < 1 {
		m.width = 1
	}
	m.viewport.SetWidth(m.width)
	m.viewport.SetHeight(bodyHeight)

	var lines []string
	for _, msg := range m.messages {
		switch msg.Kind {
		case chat.KindUser:
			lines = append(lines, "[USER] "+msg.Content)
		case chat.KindAssistant:
			prefix := "[ASSISTANT] "
			if msg.Streaming {
				prefix = "[ASSISTANT live] "
			}
			lines = append(lines, prefix+msg.Content)
		case chat.KindThinking:
			lines = append(lines, "[THINKING] "+msg.Content)
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
			lines = append(lines, line)
		case chat.KindSystem:
			lines = append(lines, "[SUMMARY] "+msg.Content)
		case chat.KindError:
			lines = append(lines, "[ERROR] "+msg.Content)
		}
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
}

func (m replayTUIModel) View() tea.View {
	m.syncViewport()
	state := "IDLE"
	if m.replayMode {
		total := 0
		if m.replayTrace != nil {
			total = len(m.replayTrace.Events)
		}
		state = fmt.Sprintf("REPLAY [%d/%d]", m.replayIdx, total)
	}
	body := state + "\n" + strings.Repeat("-", 40) + "\n" + m.viewport.View() + "\n" + strings.Repeat("-", 40) + "\n> " + m.input
	return tea.NewView(body)
}

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
	m.messages = append(m.messages, &chat.Message{Kind: chat.KindSystem, Content: "unknown command: " + text})
	return m, nil
}

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

func (m replayTUIModel) startReplay(trace *agent.ReplayTrace) (tea.Model, tea.Cmd) {
	m.replayMode = true
	m.replayTrace = trace
	m.replayIdx = 0
	m.replayStart = time.Now()
	m.busy = true
	m.messages = append(m.messages, &chat.Message{Kind: chat.KindSystem, Content: fmt.Sprintf("Replaying session from %s (%s, %d events)", trace.StartTime.Format("Jan 2 15:04"), trace.Model, len(trace.Events))})
	m.syncViewport()
	m.viewport.GotoBottom()
	return m, m.replayNext()
}

func (m *replayTUIModel) replayNext() tea.Cmd {
	if m.replayIdx >= len(m.replayTrace.Events) {
		return func() tea.Msg { return replayDoneMsg{} }
	}
	return func() tea.Msg { return replayTickMsg{} }
}

func (m replayTUIModel) handleReplayTick() (tea.Model, tea.Cmd) {
	if !m.replayMode || m.replayIdx >= len(m.replayTrace.Events) {
		return m, func() tea.Msg { return replayDoneMsg{} }
	}
	stickBottom := m.viewport.AtBottom()
	event := m.replayTrace.Events[m.replayIdx]
	m.replayIdx++

	switch event.Kind {
	case agent.EventUserInput:
		data, _ := agent.DecodeEvent[agent.UserInputData](event)
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindUser, Content: data.Text})
	case agent.EventTextDelta:
		data, _ := agent.DecodeEvent[agent.TextDeltaData](event)
		m.appendOrUpdateAssistant(data.Text)
	case agent.EventThinkDelta:
		data, _ := agent.DecodeEvent[agent.ThinkDeltaData](event)
		m.appendOrUpdateThinking(data.Text)
	case agent.EventToolCall:
		data, _ := agent.DecodeEvent[agent.ToolCallData](event)
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindToolCall, CallID: data.CallID, ToolName: data.Name, ToolArgs: data.Args, Status: chat.ToolRunning})
		m.activeToolName = data.Name
		m.activeToolArgs = data.Args
	case agent.EventToolResult:
		data, _ := agent.DecodeEvent[agent.ToolResultData](event)
		m.activeToolName = ""
		m.activeToolArgs = ""
		m.finishLastTool(data.CallID, data.Result, data.Error)
	case agent.EventAgentDone:
		data, _ := agent.DecodeEvent[agent.AgentDoneData](event)
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindSystem, Content: fmt.Sprintf("%d↓ %d↑ · %d tools", data.InputTokens, data.OutputTokens, data.ToolCalls)})
	case agent.EventSystem:
		data, _ := agent.DecodeEvent[agent.SystemEventData](event)
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindSystem, Content: data.Text})
	case agent.EventError:
		data, _ := agent.DecodeEvent[agent.ErrorEventData](event)
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindError, Content: data.Text})
	}

	m.syncViewport()
	if stickBottom {
		m.viewport.GotoBottom()
	}
	return m, m.replayNext()
}

func (m replayTUIModel) handleReplayDone() (tea.Model, tea.Cmd) {
	m.replayMode = false
	m.replayTrace = nil
	m.replayIdx = 0
	m.busy = false
	m.activeToolName = ""
	m.activeToolArgs = ""
	m.messages = append(m.messages, &chat.Message{Kind: chat.KindSystem, Content: "Replay complete."})
	m.syncViewport()
	m.viewport.GotoBottom()
	return m, nil
}

func (m *replayTUIModel) stopReplay() {
	m.replayMode = false
	m.replayTrace = nil
	m.replayIdx = 0
	m.busy = false
	m.activeToolName = ""
	m.activeToolArgs = ""
	m.messages = append(m.messages, &chat.Message{Kind: chat.KindSystem, Content: "Replay stopped."})
	m.syncViewport()
	m.viewport.GotoBottom()
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
			msg.Content = result
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

func runReplay(trace *agent.ReplayTrace) replayTUIModel {
	m := newReplayTUIModel()
	model, _ := m.startReplay(trace)
	m = model.(replayTUIModel)
	for m.replayMode {
		next, cmd := m.handleReplayTick()
		m = next.(replayTUIModel)
		if cmd == nil {
			break
		}
		if _, ok := cmd().(replayDoneMsg); ok {
			final, _ := m.handleReplayDone()
			m = final.(replayTUIModel)
			break
		}
	}
	return m
}

func TestReplayCommandGuards(t *testing.T) {
	m := newReplayTUIModel()
	m.busy = true
	msg, _ := m.handleReplayCommand("/replay")
	if !strings.Contains(msg.Content, "Cannot replay") {
		t.Fatalf("unexpected busy message: %q", msg.Content)
	}

	m = newReplayTUIModel()
	m.replayMode = true
	msg, _ = m.handleReplayCommand("/replay")
	if !strings.Contains(msg.Content, "already in progress") {
		t.Fatalf("unexpected replay-in-progress message: %q", msg.Content)
	}

	m = newReplayTUIModel()
	msg, _ = m.handleReplayCommand("/replay list")
	if msg.Content != "No replay traces found." {
		t.Fatalf("unexpected list message: %q", msg.Content)
	}
}

func TestReplayFlowRendersStreamingAndToolState(t *testing.T) {
	trace := makeTrace([]agent.ReplayEvent{
		makeEvent(agent.EventSystem, 0, agent.SystemEventData{Text: "Session started"}),
		makeEvent(agent.EventUserInput, 10, agent.UserInputData{Text: "list files"}),
		makeEvent(agent.EventThinkDelta, 20, agent.ThinkDeltaData{Text: "I'll use bash"}),
		makeEvent(agent.EventToolCall, 30, agent.ToolCallData{CallID: "tc_1", Name: "bash", Args: "ls -la"}),
		makeEvent(agent.EventToolResult, 40, agent.ToolResultData{CallID: "tc_1", Result: "file1.go\nfile2.go"}),
		makeEvent(agent.EventTextDelta, 50, agent.TextDeltaData{Text: "Found 2 Go files."}),
		makeEvent(agent.EventAgentDone, 60, agent.AgentDoneData{InputTokens: 2000, OutputTokens: 800, ToolCalls: 1}),
	})

	m := runReplay(trace)
	out := m.View().Content
	for _, want := range []string{"IDLE", "[SUMMARY] Session started", "[USER] list files", "[THINKING] I'll use bash", "[TOOL done] bash (ls -la) => file1.go | file2.go", "[ASSISTANT] Found 2 Go files.", "2000↓ 800↑ · 1 tools", "Replay complete."} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestReplayStopEscape(t *testing.T) {
	m := newReplayTUIModel()
	model, _ := m.startReplay(makeTrace([]agent.ReplayEvent{makeEvent(agent.EventUserInput, 0, agent.UserInputData{Text: "start"})}))
	m = model.(replayTUIModel)
	updated, _ := m.Update(tea.KeyPressMsg{Text: "escape"})
	m = updated.(replayTUIModel)
	out := m.View().Content
	if !strings.Contains(out, "IDLE") || !strings.Contains(out, "Replay stopped.") {
		t.Fatalf("unexpected stop output:\n%s", out)
	}
}

func TestReplayViewportScrollAndResize(t *testing.T) {
	m := newReplayTUIModel()
	m.width = 60
	m.height = 12
	for i := 0; i < 40; i++ {
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindAssistant, Content: fmt.Sprintf("line %02d", i+1)})
	}
	m.syncViewport()
	m.viewport.GotoBottom()
	start := m.viewport.YOffset()
	updated, _ := m.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
	m = updated.(replayTUIModel)
	if m.viewport.YOffset() >= start {
		t.Fatalf("expected replay viewport to scroll upward: before=%d after=%d", start, m.viewport.YOffset())
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 48, Height: 9})
	m = updated.(replayTUIModel)
	if m.viewport.Height() != 6 {
		t.Fatalf("viewport height=%d, want 6", m.viewport.Height())
	}
	if !strings.Contains(m.View().Content, "line") {
		t.Fatalf("replay viewport lost transcript after resize\n%s", m.View().Content)
	}
}
