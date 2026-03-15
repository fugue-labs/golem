package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/ui/chat"
	uiinvariants "github.com/fugue-labs/golem/internal/ui/invariants"
	"github.com/fugue-labs/golem/internal/ui/plan"
	"github.com/fugue-labs/golem/internal/ui/styles"
	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/codetool"
)

func fillTranscriptMessages(m *Model, count int) {
	for i := 0; i < count; i++ {
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindAssistant, Content: fmt.Sprintf("message %03d %s", i+1, strings.Repeat("detail ", 4))})
	}
}

func testTempConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", WorkingDir: t.TempDir()}
}

func newAppTestModel(t *testing.T) *Model {
	t.Helper()
	m := New(testTempConfig(t))
	m.sty = styles.New(nil)
	return m
}

func TestHandleRuntimePreparedReusesSessionAndPreparesFreshAgent(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", TeamMode: "auto"})
	oldSession := &codetool.Session{}
	m.runtime.Session = oldSession

	msg := runtimePreparedMsg{
		runID:  1,
		prompt: "inspect repo",
		runtime: agent.RuntimeState{
			Session:           nil,
			EffectiveTeamMode: false,
			TeamModeReason:    "auto router model=test-model complexity=simple confidence=high",
			CodeModeStatus:    "pending",
			AskUserStatus:     "off",
		},
	}
	m.runID = 1
	m.runCtx = context.Background()

	updated, cmd := m.handleRuntimePrepared(msg)
	model, ok := updated.(*Model)
	if !ok {
		t.Fatalf("handleRuntimePrepared returned %T", updated)
	}
	if model.runtime.Session != oldSession {
		t.Fatal("expected session reuse across runtime refresh")
	}
	if model.runtime.AskUserStatus != "off" {
		t.Fatalf("AskUserStatus = %q, want off", model.runtime.AskUserStatus)
	}
	if model.runtime.AskUserFunc != nil {
		t.Fatal("expected ask_user callback to remain disabled when team mode is off")
	}
	if model.agent == nil {
		t.Fatal("expected fresh agent to be constructed")
	}
	if cmd == nil {
		t.Fatal("expected run command after runtime preparation")
	}
}

func TestAgentDoneMsgClearsAgentForNextPromptRouting(t *testing.T) {
	m := New(&config.Config{})
	m.runID = 7
	m.agent = &core.Agent[string]{}
	m.busy = true
	m.runCtx = context.Background()
	m.cancel = func() {}

	updated, _ := m.Update(agentDoneMsg{runID: 7})
	model := updated.(*Model)
	if model.agent != nil {
		t.Fatal("expected agent to be cleared after run completion")
	}
	if model.runCtx != nil {
		t.Fatal("expected run context to be cleared after run completion")
	}
}

func TestStaleStreamingEventsAreIgnored(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", APIKey: "test-key"})
	m.runID = 2
	m.runtime = agent.RuntimeState{CodeModeStatus: "on", OpenImageStatus: "off"}
	m.messages = []*chat.Message{{Kind: chat.KindAssistant, Content: "current"}}
	m.busy = true
	m.agent = &core.Agent[string]{}
	m.runCtx = context.Background()
	m.cancel = func() {}

	staleEvents := []struct {
		name string
		msg  tea.Msg
	}{
		{name: "text", msg: textDeltaMsg{runID: 1, text: "stale text"}},
		{name: "thinking", msg: thinkingDeltaMsg{runID: 1, text: "stale thinking"}},
		{name: "tool call", msg: toolCallMsg{runID: 1, name: "grep", args: "{}", rawArgs: "{}"}},
		{name: "runtime prepared", msg: runtimePreparedMsg{runID: 1, prompt: "old", runtime: agent.RuntimeState{CodeModeStatus: "off", OpenImageStatus: "on"}}},
		{name: "done", msg: agentDoneMsg{runID: 1, err: context.Canceled}},
	}

	for _, tt := range staleEvents {
		t.Run(tt.name, func(t *testing.T) {
			updated, _ := m.Update(tt.msg)
			model := updated.(*Model)
			if len(model.messages) != 1 || model.messages[0].Content != "current" {
				t.Fatalf("messages mutated by stale %s event: %+v", tt.name, model.messages)
			}
			if !model.busy {
				t.Fatalf("busy cleared by stale %s event", tt.name)
			}
			if model.agent == nil {
				t.Fatalf("agent cleared by stale %s event", tt.name)
			}
		})
	}
}

func TestHandleRuntimePreparedErrorReturnsAgentDoneError(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 3

	updated, cmd := m.handleRuntimePrepared(runtimePreparedMsg{runID: 3, err: context.DeadlineExceeded})
	if updated != m {
		t.Fatal("expected original model to be returned")
	}
	if cmd == nil {
		t.Fatal("expected follow-up command")
	}
	msg := cmd()
	done, ok := msg.(agentDoneMsg)
	if !ok {
		t.Fatalf("cmd() returned %T", msg)
	}
	if done.runID != 3 || done.err == nil || !strings.Contains(done.err.Error(), "deadline") {
		t.Fatalf("unexpected agentDoneMsg: %+v", done)
	}
}

func TestHandleKeyQuitShutsDownAskLoop(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})

	updated, cmd := m.handleKey(tea.KeyPressMsg{Text: "ctrl+c", Code: tea.KeyEscape, Mod: tea.ModCtrl})
	model := updated.(*Model)
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("cmd() did not return tea.QuitMsg")
	}
	select {
	case <-model.askDone:
	default:
		t.Fatal("expected ask loop shutdown channel to be closed")
	}
}

func TestAgentDoneCapturesRunSummary(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", APIKey: "test-key"})
	m.runID = 9
	m.lastPrompt = "inspect repo"
	m.messages = []*chat.Message{{Kind: chat.KindUser, Content: "earlier run"}, {Kind: chat.KindUser, Content: "inspect repo"}}
	m.currentRunMessages = []*chat.Message{{Kind: chat.KindUser, Content: "inspect repo"}}
	m.busy = true

	updated, _ := m.Update(agentDoneMsg{runID: 9, usage: core.RunUsage{Requests: 1}, messages: []core.ModelMessage{}})
	model := updated.(*Model)
	if model.lastRunSummary == nil {
		t.Fatal("expected run summary to be captured")
	}
	if model.lastRunSummary.Prompt != "inspect repo" {
		t.Fatalf("prompt = %q", model.lastRunSummary.Prompt)
	}
	if model.lastRunSummary.FinalStatus != "success" {
		t.Fatalf("final status = %q", model.lastRunSummary.FinalStatus)
	}
	if got := len(model.lastRunSummary.Transcript); got != 1 {
		t.Fatalf("transcript entries=%d, want 1", got)
	}
}

func TestIsContextCanceled(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"direct context.Canceled", context.Canceled, true},
		{"wrapped with %w", fmt.Errorf("outer: %w", context.Canceled), true},
		{"double wrapped with %w", fmt.Errorf("model request failed: %w", fmt.Errorf("openai: SSE read error: %w", context.Canceled)), true},
		{"wrapped with %v (broken chain)", fmt.Errorf("openai: SSE read error: %v", context.Canceled), true},
		{"SSE error chain with %v", fmt.Errorf("model request failed: %w", fmt.Errorf("openai: SSE read error: %v", context.Canceled)), true},
		{"unrelated error", errors.New("connection refused"), false},
		{"deadline exceeded", context.DeadlineExceeded, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isContextCanceled(tt.err); got != tt.want {
				t.Fatalf("isContextCanceled(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestAgentDoneWrappedCancelNotShownAsError(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", APIKey: "test-key"})
	m.runID = 10
	m.busy = true

	// Simulate an error where context.Canceled is wrapped with %v,
	// breaking errors.Is but preserving the "context canceled" string.
	wrappedErr := fmt.Errorf("model request failed: openai: SSE read error: %v", context.Canceled)
	updated, _ := m.Update(agentDoneMsg{runID: 10, err: wrappedErr})
	model := updated.(*Model)

	for _, msg := range model.messages {
		if msg.Kind == chat.KindError {
			t.Fatalf("context cancel with broken wrapping shown as error: %q", msg.Content)
		}
	}
}

func TestViewConfiguresBubbleTeaMetadata(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", WorkingDir: "/tmp/project"})
	m.sty = styles.New(nil)
	m.width = 100
	m.height = 22

	v := m.View()
	if !v.AltScreen {
		t.Fatal("expected alt screen metadata")
	}
	if v.WindowTitle != "GOLEM — project — ready" {
		t.Fatalf("window title = %q", v.WindowTitle)
	}
	if !v.ReportFocus {
		t.Fatal("expected focus reporting metadata")
	}
	if v.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("mouse mode = %v", v.MouseMode)
	}
}

func TestViewFocusAndMouseWheelUpdateMetadataAndScroll(t *testing.T) {
	m := newAppTestModel(t)
	m.width = 100
	m.height = 16
	fillTranscriptMessages(m, 24)
	m.ensureTranscriptViewport(m.width, m.height)

	updated, _ := m.Update(tea.BlurMsg{})
	m = updated.(*Model)
	if m.terminalFocused {
		t.Fatal("expected blur to clear focus state")
	}
	if got := m.renderStatusMeta(); got != "Terminal unfocused" {
		t.Fatalf("status meta = %q", got)
	}
	if got := m.renderInputMeta(); got != "Input paused · refocus terminal to type" {
		t.Fatalf("input meta = %q", got)
	}
	if title := m.View().WindowTitle; title != m.windowTitle() {
		t.Fatalf("window title changed unexpectedly on blur: %q", title)
	}

	updated, _ = m.Update(tea.FocusMsg{})
	m = updated.(*Model)
	if !m.terminalFocused {
		t.Fatal("expected focus msg to restore focus state")
	}

	updated, _ = m.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
	m = updated.(*Model)
	if m.scroll != 1 {
		t.Fatalf("scroll after wheel up = %d", m.scroll)
	}
	updated, _ = m.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
	m = updated.(*Model)
	if m.scroll != 0 {
		t.Fatalf("scroll after wheel down = %d", m.scroll)
	}
}

func TestViewMetadataStillAllowsSlashCommandCompletion(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", WorkingDir: "/tmp/project"})
	m.sty = styles.New(nil)
	m.width = 100
	m.height = 16
	m.input.SetValue("/se")

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(*Model)
	if got := m.input.Value(); got != "/search" {
		t.Fatalf("tab completion = %q", got)
	}

	v := m.View()
	if !v.AltScreen || !v.ReportFocus {
		t.Fatalf("unexpected view metadata: alt=%v focus=%v", v.AltScreen, v.ReportFocus)
	}
	if v.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("mouse mode = %v", v.MouseMode)
	}
}

func TestViewRendersDistinctShellRegions(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.width = 100
	m.height = 22
	m.input.SetValue("draft task")

	rendered := stripANSI(m.View().Content)
	for _, want := range []string{"GOLEM", "Transcript", "Input", "Status", "Context ·", "Activity ·", "draft task", " Context "} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("view missing %q\n%s", want, rendered)
		}
	}
}

func TestViewWorkflowPanelGatesByWidth(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.height = 20
	m.messages = []*chat.Message{{Kind: chat.KindAssistant, Content: "brief response"}}
	m.planState = plan.State{Tasks: []plan.Task{{ID: "T1", Description: "verify tests", Status: "in_progress"}}}
	m.invariantState = uiinvariants.State{Extracted: true, Items: []uiinvariants.Item{{ID: "I1", Description: "no TODOs", Kind: "hard", Status: "unknown"}}}

	m.width = workflowPanelStackMinWidth - 1
	narrow := stripANSI(m.View().Content)
	if strings.Contains(narrow, "Workflow") {
		t.Fatalf("workflow panel should stay hidden below stacked width\n%s", narrow)
	}

	m.width = 80
	stacked := stripANSI(m.View().Content)
	if !strings.Contains(stacked, "Workflow") {
		t.Fatalf("workflow panel should remain visible on mid-width terminals\n%s", stacked)
	}

	m.width = workflowPanelWideMinWidth
	wide := stripANSI(m.View().Content)
	if !strings.Contains(wide, "Workflow") {
		t.Fatalf("workflow panel should appear at wide layout width\n%s", wide)
	}
	sharedHeader := false
	for _, line := range strings.Split(wide, "\n") {
		if strings.Contains(line, "Transcript") && strings.Contains(line, "Workflow") {
			sharedHeader = true
			break
		}
	}
	if !sharedHeader {
		t.Fatalf("workflow panel should share the transcript row in wide layout\n%s", wide)
	}
}

func TestViewShowsResizeHelpWhenBelowMinimumSize(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)

	tests := []struct {
		name   string
		width  int
		height int
		want   string
	}{
		{name: "narrow", width: minShellWidth - 1, height: 18, want: "Resize wider"},
		{name: "short", width: 80, height: minShellHeight - 1, want: "Resize taller"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.width = tt.width
			m.height = tt.height
			rendered := stripANSI(m.View().Content)
			for _, want := range []string{"GOLEM", "Terminal too small", tt.want, "need at least 56x6"} {
				if !strings.Contains(rendered, want) {
					t.Fatalf("minimum-size view missing %q\n%s", want, rendered)
				}
			}
		})
	}
}

func TestViewShowsResizeHelpWithinVeryNarrowWidths(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.width = 12
	m.height = 5

	rendered := stripANSI(m.View().Content)
	if got := lipgloss.Height(rendered); got > m.height {
		t.Fatalf("minimum-size view height=%d produced %d lines\n%s", m.height, got, rendered)
	}
	for i, line := range strings.Split(rendered, "\n") {
		if w := lipgloss.Width(line); w > m.width {
			t.Fatalf("line %d width=%d exceeds shell width %d: %q", i, w, m.width, line)
		}
	}
	for _, want := range []string{"GOLEM", "Resize", "/help"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("very narrow minimum-size view missing %q\n%s", want, rendered)
		}
	}
}

func TestViewAdaptsWorkflowPanelAcrossWidths(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.messages = []*chat.Message{{Kind: chat.KindAssistant, Content: "brief response"}}
	m.planState = plan.State{Tasks: []plan.Task{{ID: "T1", Description: "verify tests", Status: "in_progress"}}}
	m.invariantState = uiinvariants.State{Extracted: true, Items: []uiinvariants.Item{{ID: "I1", Description: "no TODOs", Kind: "hard", Status: "unknown"}}}

	cases := []struct {
		name               string
		width              int
		height             int
		wantWorkflow       bool
		wantSeparateHeader bool
		wantSharedHeader   bool
		wantStatusSummary  bool
	}{
		{name: "mid-width stacked", width: 80, height: 20, wantWorkflow: true, wantSeparateHeader: true},
		{name: "short mid-width status summary 6 rows", width: 80, height: 6, wantWorkflow: false, wantStatusSummary: true},
		{name: "short mid-width status summary 7 rows", width: 80, height: 7, wantWorkflow: false, wantStatusSummary: true},
		{name: "wide side-by-side", width: 120, height: 20, wantWorkflow: true, wantSharedHeader: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			m.width = tt.width
			m.height = tt.height
			rendered := stripANSI(m.View().Content)
			if strings.Contains(rendered, "Terminal too small") {
				t.Fatalf("layout should stay interactive at %dx%d\n%s", tt.width, tt.height, rendered)
			}
			if got := lipgloss.Height(rendered); got > tt.height {
				t.Fatalf("view height=%d produced %d lines\n%s", tt.height, got, rendered)
			}
			if tt.wantWorkflow && !strings.Contains(rendered, "Workflow") {
				t.Fatalf("expected workflow state to stay visible at %dx%d\n%s", tt.width, tt.height, rendered)
			}
			if !tt.wantWorkflow && strings.Contains(rendered, "Workflow") {
				t.Fatalf("did not expect stacked workflow panel chrome at %dx%d\n%s", tt.width, tt.height, rendered)
			}
			if tt.wantStatusSummary {
				for _, want := range []string{"workflow plan active", "GOLEM", "brief response", "❯"} {
					if !strings.Contains(rendered, want) {
						t.Fatalf("expected compact workflow summary %q at %dx%d\n%s", want, tt.width, tt.height, rendered)
					}
				}
			}
			if tt.wantSeparateHeader {
				found := false
				for _, line := range strings.Split(rendered, "\n") {
					if strings.Contains(line, "Workflow") && !strings.Contains(line, "Transcript") {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("expected stacked workflow header on mid-width layout\n%s", rendered)
				}
			}
			if tt.wantSharedHeader {
				found := false
				for _, line := range strings.Split(rendered, "\n") {
					if strings.Contains(line, "Transcript") && strings.Contains(line, "Workflow") {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("expected side-by-side workflow panel on wide layout\n%s", rendered)
				}
			}
		})
	}
}

func TestRenderChatRespectsVerySmallHeights(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.messages = []*chat.Message{{Kind: chat.KindAssistant, Content: "one\ntwo\nthree"}}

	for _, height := range []int{1, 2} {
		t.Run(fmt.Sprintf("height-%d", height), func(t *testing.T) {
			rendered := m.renderChat(height, 40)
			if got := lipgloss.Height(rendered); got > height {
				t.Fatalf("renderChat height=%d produced %d lines\n%s", height, got, stripANSI(rendered))
			}
			if strings.Contains(stripANSI(rendered), "Transcript") {
				t.Fatalf("renderChat should skip transcript chrome at height=%d\n%s", height, stripANSI(rendered))
			}
		})
	}
}

func TestViewSwitchesToCompactLayoutWhenFullShellLeavesNoTranscriptSpace(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.width = 80
	m.height = lipgloss.Height(m.renderHeader()) + lipgloss.Height(m.renderInput()) + lipgloss.Height(m.renderStatusBar())
	m.messages = []*chat.Message{{Kind: chat.KindAssistant, Content: "compact fallback transcript"}}

	rendered := stripANSI(m.View().Content)
	if strings.Contains(rendered, "Input") || strings.Contains(rendered, "Status") {
		t.Fatalf("expected compact layout to collapse full-shell input/status chrome\n%s", rendered)
	}
	for _, want := range []string{"GOLEM", "Transcript", "compact fallback transcript", "❯"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("compact layout missing %q\n%s", want, rendered)
		}
	}
}

func TestVisibleChatLinesKeepsBottomWindowWithoutRenderingEntireTranscriptSlice(t *testing.T) {
	m := newAppTestModel(t)
	fillTranscriptMessages(m, 40)

	visible := stripANSI(strings.Join(m.visibleChatLines(4, 60), "\n"))
	if strings.Contains(visible, "message 001") {
		t.Fatalf("expected bottom-aligned transcript window, got\n%s", visible)
	}
	for _, want := range []string{"message 039", "message 040"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("visible window missing %q\n%s", want, visible)
		}
	}

	m.ensureTranscriptViewport(60, 4)
	m.transcriptViewport.SetYOffset(max(0, m.transcriptViewport.YOffset()-2))
	m.scroll = transcriptMaxScroll(m.transcriptViewport) - m.transcriptViewport.YOffset()
	scrolled := stripANSI(strings.Join(m.visibleChatLines(4, 60), "\n"))
	if !strings.Contains(scrolled, "message 038") {
		t.Fatalf("expected scrolled transcript to reveal earlier content\n%s", scrolled)
	}
}

func TestTranscriptViewportTracksResizeWithoutResettingScroll(t *testing.T) {
	m := newAppTestModel(t)
	m.width = 96
	m.height = 18
	fillTranscriptMessages(m, 80)
	m.renderChat(10, 96)

	m.handleTranscriptViewportMsg(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
	if m.scroll == 0 {
		t.Fatal("expected upward transcript scroll to move away from bottom")
	}
	before := m.scroll

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 72, Height: 12})
	m = updated.(*Model)
	if m.scroll == 0 {
		t.Fatal("resize unexpectedly snapped transcript to bottom")
	}
	if m.scroll < before-6 {
		t.Fatalf("resize should preserve most transcript scroll, got %d want >= %d", m.scroll, before-6)
	}
	view := stripANSI(m.renderChat(6, 72))
	if !strings.Contains(view, "Transcript") || !strings.Contains(view, "message") {
		t.Fatalf("resized transcript missing expected content\n%s", view)
	}
}

func TestViewShortWindowDoesNotExceedTerminalHeight(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.width = 72
	m.height = 6
	m.invariantState.Extracted = true
	m.messages = []*chat.Message{{Kind: chat.KindAssistant, Content: "brief response"}}

	rendered := stripANSI(m.View().Content)
	if got := lipgloss.Height(rendered); got > m.height {
		t.Fatalf("view height=%d produced %d lines\n%s", m.height, got, rendered)
	}
	for _, want := range []string{"GOLEM", "brief response", "❯", "Ready"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("short view missing %q\n%s", want, rendered)
		}
	}
}
