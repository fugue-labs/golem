package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/ui/chat"
	"github.com/fugue-labs/golem/internal/ui/styles"
)

func makeEvent(kind agent.ReplayEventKind, offsetMs int64, payload any) agent.ReplayEvent {
	data, _ := json.Marshal(payload)
	return agent.ReplayEvent{Kind: kind, OffsetMs: offsetMs, Data: data}
}

func makeTrace(workDir string, events []agent.ReplayEvent) *agent.ReplayTrace {
	return &agent.ReplayTrace{
		Version:   1,
		StartTime: time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC),
		Model:     "test-model",
		Provider:  "test-provider",
		WorkDir:   workDir,
		Events:    events,
	}
}

func newReplayTestModel(t *testing.T) *Model {
	t.Helper()
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", WorkingDir: t.TempDir()})
	m.sty = styles.New(nil)
	m.width = 80
	m.height = 20
	m.ensureTranscriptViewport(m.width, m.height)
	return m
}

func drainReplay(t *testing.T, m *Model) *Model {
	t.Helper()
	for {
		if !m.replayMode {
			return m
		}
		next, cmd := m.handleReplayTick()
		m = next.(*Model)
		if cmd == nil {
			return m
		}
		if _, ok := cmd().(replayDoneMsg); ok {
			final, _ := m.handleReplayDone()
			return final.(*Model)
		}
	}
}

func findReplayMessage(messages []*chat.Message, kind chat.MessageKind, want string) *chat.Message {
	for _, msg := range messages {
		if msg.Kind == kind && strings.Contains(msg.Content, want) {
			return msg
		}
	}
	return nil
}

func findReplayToolCall(messages []*chat.Message, callID string) *chat.Message {
	for _, msg := range messages {
		if msg.Kind == chat.KindToolCall && msg.CallID == callID {
			return msg
		}
	}
	return nil
}

func scrollReplayUp(m *Model, steps int) {
	for i := 0; i < steps; i++ {
		m.handleTranscriptViewportMsg(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
	}
}

func TestReplayCommandGuards(t *testing.T) {
	m := newReplayTestModel(t)
	m.busy = true
	msg, _ := m.handleReplayCommand("/replay")
	if !strings.Contains(msg.Content, "Cannot replay") {
		t.Fatalf("unexpected busy message: %q", msg.Content)
	}

	m = newReplayTestModel(t)
	m.replayMode = true
	msg, _ = m.handleReplayCommand("/replay")
	if !strings.Contains(msg.Content, "already in progress") {
		t.Fatalf("unexpected replay-in-progress message: %q", msg.Content)
	}

	m = newReplayTestModel(t)
	msg, _ = m.handleReplayCommand("/replay list")
	if msg.Content != "No replay traces found." {
		t.Fatalf("unexpected list message: %q", msg.Content)
	}
}

func TestReplayFlowRendersStreamingAndToolState(t *testing.T) {
	m := newReplayTestModel(t)
	trace := makeTrace(m.cfg.WorkingDir, []agent.ReplayEvent{
		makeEvent(agent.EventSystem, 0, agent.SystemEventData{Text: "Session started"}),
		makeEvent(agent.EventUserInput, 10, agent.UserInputData{Text: "list files"}),
		makeEvent(agent.EventThinkDelta, 20, agent.ThinkDeltaData{Text: "I'll use bash"}),
		makeEvent(agent.EventToolCall, 30, agent.ToolCallData{CallID: "tc_1", Name: "bash", Args: "{\"command\":\"ls -la\"}", RawArgs: "{\"command\":\"ls -la\"}"}),
		makeEvent(agent.EventToolResult, 40, agent.ToolResultData{CallID: "tc_1", Name: "bash", Result: "file1.go\nfile2.go"}),
		makeEvent(agent.EventTextDelta, 50, agent.TextDeltaData{Text: "Found 2 Go files."}),
		makeEvent(agent.EventAgentDone, 60, agent.AgentDoneData{InputTokens: 2000, OutputTokens: 800, ToolCalls: 1}),
	})

	startMsg, _ := m.startReplay(trace)
	m.appendMessage(startMsg, true)
	m = drainReplay(t, m)

	if m.replayMode || m.busy {
		t.Fatalf("replay state not cleared: replay=%v busy=%v", m.replayMode, m.busy)
	}
	if findReplayMessage(m.messages, chat.KindSystem, "Session started") == nil {
		t.Fatal("expected replay transcript to include the session-start system message")
	}
	if findReplayMessage(m.messages, chat.KindUser, "list files") == nil {
		t.Fatal("expected replay transcript to include the replayed user input")
	}
	if findReplayMessage(m.messages, chat.KindThinking, "I'll use bash") == nil {
		t.Fatal("expected replay transcript to include the thinking delta")
	}
	tool := findReplayToolCall(m.messages, "tc_1")
	if tool == nil {
		t.Fatal("expected replay transcript to include the tool call")
	}
	if tool.ToolName != "bash" || tool.ToolArgs != "ls -la" {
		t.Fatalf("unexpected tool summary: %#v", tool)
	}
	if tool.Status != chat.ToolSuccess {
		t.Fatalf("tool status = %v, want success", tool.Status)
	}
	if !strings.Contains(tool.Content, "file1.go") || !strings.Contains(tool.Content, "file2.go") {
		t.Fatalf("tool result missing expected files: %q", tool.Content)
	}
	assistant := findReplayMessage(m.messages, chat.KindAssistant, "Found 2 Go files.")
	if assistant == nil {
		t.Fatal("expected replay transcript to include the assistant response")
	}
	if assistant.Streaming {
		t.Fatal("assistant message should not still be marked streaming after replay completion")
	}
	if findReplayMessage(m.messages, chat.KindSystem, "2000↓ 800↑ · 1 tools") == nil {
		t.Fatal("expected replay transcript to include the usage summary")
	}
	if got := m.messages[len(m.messages)-1].Content; got != "Replay complete." {
		t.Fatalf("last replay message = %q, want Replay complete.", got)
	}

	tail := stripANSI(strings.Join(m.visibleChatLines(12, 80), "\n"))
	for _, want := range []string{"Found 2 Go files.", "Replay complete."} {
		if !strings.Contains(tail, want) {
			t.Fatalf("expected bounded viewport tail to include %q\n%s", want, tail)
		}
	}
}

func TestReplayStopEscape(t *testing.T) {
	m := newReplayTestModel(t)
	startMsg, _ := m.startReplay(makeTrace(m.cfg.WorkingDir, []agent.ReplayEvent{makeEvent(agent.EventUserInput, 0, agent.UserInputData{Text: "start"})}))
	m.appendMessage(startMsg, true)
	updated, _ := m.Update(tea.KeyPressMsg{Text: "escape"})
	m = updated.(*Model)
	if m.replayMode || m.busy {
		t.Fatalf("replay stop did not clear state: replay=%v busy=%v", m.replayMode, m.busy)
	}
	if got := m.messages[len(m.messages)-1].Content; got != "Replay stopped." {
		t.Fatalf("last replay message = %q, want Replay stopped.", got)
	}
}

func TestReplayViewportStickyBottomAndResize(t *testing.T) {
	m := newReplayTestModel(t)
	m.width = 60
	m.height = 12
	for i := 0; i < 40; i++ {
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindAssistant, Content: fmt.Sprintf("line %02d", i+1)})
	}
	m.ensureTranscriptViewport(m.width, m.height)
	m.pinTranscriptToBottom()

	trace := makeTrace(m.cfg.WorkingDir, []agent.ReplayEvent{makeEvent(agent.EventTextDelta, 0, agent.TextDeltaData{Text: " live tail"})})
	_, _ = m.startReplay(trace)
	next, _ := m.handleReplayTick()
	m = next.(*Model)
	if m.scroll != 0 {
		t.Fatalf("expected sticky-bottom replay update to stay pinned, scroll=%d", m.scroll)
	}
	if !m.transcriptViewport.AtBottom() {
		t.Fatal("expected viewport to remain at bottom after replay append")
	}

	scrollReplayUp(m, 1)
	before := m.scroll
	if before == 0 {
		t.Fatal("expected manual scroll before resize test")
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 48, Height: 9})
	m = updated.(*Model)
	if m.scroll == 0 {
		t.Fatal("resize unexpectedly snapped replay transcript to bottom")
	}
	if m.transcriptViewport.Height() < 1 {
		t.Fatalf("viewport height=%d, want positive", m.transcriptViewport.Height())
	}
}

func TestReplayDonePreservesScrolledPosition(t *testing.T) {
	m := newReplayTestModel(t)
	m.width = 72
	m.height = 12
	fillTranscriptMessages(m, 60)
	m.ensureTranscriptViewport(m.width, m.height)

	startMsg, _ := m.startReplay(makeTrace(m.cfg.WorkingDir, nil))
	m.appendMessage(startMsg, true)
	scrollReplayUp(m, 2)
	before := m.scroll
	if before == 0 {
		t.Fatal("expected replay transcript to be scrolled off bottom before completion")
	}

	next, _ := m.handleReplayDone()
	m = next.(*Model)
	if m.scroll == 0 {
		t.Fatal("replay completion unexpectedly snapped transcript to bottom")
	}
	if m.scroll < before-2 {
		t.Fatalf("replay completion moved scroll too far: got %d want >= %d", m.scroll, before-2)
	}
}

func TestReplayStopPreservesScrolledPosition(t *testing.T) {
	m := newReplayTestModel(t)
	m.width = 72
	m.height = 12
	fillTranscriptMessages(m, 60)
	m.ensureTranscriptViewport(m.width, m.height)

	startMsg, _ := m.startReplay(makeTrace(m.cfg.WorkingDir, []agent.ReplayEvent{makeEvent(agent.EventTextDelta, 0, agent.TextDeltaData{Text: "tail"})}))
	m.appendMessage(startMsg, true)
	scrollReplayUp(m, 2)
	before := m.scroll
	if before == 0 {
		t.Fatal("expected replay transcript to be scrolled off bottom before stopping")
	}

	m.stopReplay()
	if m.scroll == 0 {
		t.Fatal("replay stop unexpectedly snapped transcript to bottom")
	}
	if m.scroll < before-2 {
		t.Fatalf("replay stop moved scroll too far: got %d want >= %d", m.scroll, before-2)
	}
}

func TestReplayStreamingRespectsUserScrollPosition(t *testing.T) {
	m := newReplayTestModel(t)
	m.width = 72
	m.height = 12
	for i := 0; i < 50; i++ {
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindAssistant, Content: fmt.Sprintf("history %02d", i+1)})
	}
	m.ensureTranscriptViewport(m.width, m.height)
	scrollReplayUp(m, 1)
	scrolled := m.scroll
	if scrolled == 0 {
		t.Fatal("expected transcript to be scrolled off bottom before replay tick")
	}

	trace := makeTrace(m.cfg.WorkingDir, []agent.ReplayEvent{makeEvent(agent.EventTextDelta, 0, agent.TextDeltaData{Text: "new streamed reply"})})
	_, _ = m.startReplay(trace)
	scrollReplayUp(m, 1)
	scrolled = m.scroll
	next, _ := m.handleReplayTick()
	m = next.(*Model)
	if m.scroll == 0 {
		t.Fatal("expected replay append to preserve scrolled position when user is not at bottom")
	}
	if m.scroll < scrolled-2 {
		t.Fatalf("scroll moved too far during replay append: got %d want >= %d", m.scroll, scrolled-2)
	}
}
