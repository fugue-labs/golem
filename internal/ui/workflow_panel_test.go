package ui

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/ui/chat"
	uiinvariants "github.com/fugue-labs/golem/internal/ui/invariants"
	"github.com/fugue-labs/golem/internal/ui/plan"
	"github.com/fugue-labs/golem/internal/ui/styles"
	"github.com/fugue-labs/gollem/core"
)

func TestCancelActiveRunClearsRunStateAndBumpsRunID(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 4
	m.hookRID.Store(4)
	m.busy = true
	m.runCtx = context.Background()
	m.cancel = func() {}
	m.agent = &core.Agent[string]{}

	m.cancelActiveRun(false)

	if m.busy {
		t.Fatal("expected busy=false after cancellation")
	}
	if m.cancel != nil {
		t.Fatal("expected cancel func cleared")
	}
	if m.runCtx != nil {
		t.Fatal("expected run context cleared")
	}
	if m.agent != nil {
		t.Fatal("expected agent cleared")
	}
	if m.runID != 5 {
		t.Fatalf("runID=%d, want 5", m.runID)
	}
	if got := m.hookRID.Load(); got != 5 {
		t.Fatalf("hookRID=%d, want 5", got)
	}
}

func TestCancelActiveRunAsyncAppendsCancelMessageAndClearsPending(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 1
	m.hookRID.Store(1)
	m.busy = true
	m.runCtx = context.Background()
	m.cancel = func() {}
	m.pendingMsgs = []string{"follow-up one", "follow-up two"}

	m.cancelActiveRun(true)

	if got := len(m.pendingMsgs); got != 0 {
		t.Fatalf("pending messages=%d, want 0", got)
	}
	if len(m.messages) == 0 {
		t.Fatal("expected cancellation message to be appended")
	}
	last := m.messages[len(m.messages)-1]
	if last.Kind != chat.KindAssistant || !strings.Contains(last.Content, "Run canceled") {
		t.Fatalf("unexpected cancel message: %+v", last)
	}
	if !strings.Contains(last.Content, "Discarded 2 queued follow-up(s).") {
		t.Fatalf("expected dropped-queue note, got %q", last.Content)
	}
}

func TestClearSessionStateResetsWorkflowAndPendingState(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 2
	m.hookRID.Store(2)
	m.busy = true
	m.runCtx = context.Background()
	m.cancel = func() {}
	m.agent = &core.Agent[string]{}
	m.messages = []*chat.Message{{Kind: chat.KindAssistant, Content: "hi"}}
	m.history = []core.ModelMessage{core.ModelRequest{}}
	m.pendingMsgs = []string{"follow-up"}
	m.planState = plan.State{Tasks: []plan.Task{{ID: "T1", Description: "ship", Status: "in_progress"}}}
	m.invariantState = uiinvariants.State{Extracted: true, Items: []uiinvariants.Item{{ID: "I1", Description: "tests pass", Kind: "hard", Status: "pass"}}}

	m.clearSessionState()

	if m.busy {
		t.Fatal("expected busy=false")
	}
	if len(m.messages) != 0 || len(m.history) != 0 {
		t.Fatal("expected transcript/history cleared")
	}
	if len(m.pendingMsgs) != 0 {
		t.Fatal("expected pending messages cleared")
	}
	if m.planState.HasTasks() {
		t.Fatal("expected plan state reset")
	}
	if m.invariantState.HasItems() {
		t.Fatal("expected invariant state reset")
	}
	if m.agent != nil {
		t.Fatal("expected agent cleared")
	}
	if m.runtime.CodeModeStatus == "" {
		t.Fatal("expected runtime reset to initial state")
	}
}

func TestWorkflowPanelRendersPlanAndInvariantSections(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.planState = plan.State{Tasks: []plan.Task{
		{ID: "T1", Description: "inspect repo", Status: "completed"},
		{ID: "T2", Description: "verify tests", Status: "in_progress"},
	}}
	m.invariantState = uiinvariants.State{Extracted: true, Items: []uiinvariants.Item{
		{ID: "I1", Description: "tests pass", Kind: "hard", Status: "pass"},
		{ID: "I2", Description: "no TODOs", Kind: "hard", Status: "unknown"},
	}}

	rendered := stripANSI(m.renderWorkflowPanel(10, 40))

	for _, want := range []string{"Workflow", "Plan 1/2 completed", "Inv 1✓ 0✗ 1?", "inspect repo", "tests pass"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("workflow panel missing %q\n%s", want, rendered)
		}
	}
}

func TestHasWorkflowPanelIncludesInvariantOnlyState(t *testing.T) {
	m := New(&config.Config{})
	m.invariantState = uiinvariants.State{Extracted: true}
	if !m.hasWorkflowPanel() {
		t.Fatal("expected workflow panel when only invariants exist")
	}
}

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}
