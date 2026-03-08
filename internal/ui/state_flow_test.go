package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/golem/internal/config"
	uiinvariants "github.com/fugue-labs/golem/internal/ui/invariants"
	"github.com/fugue-labs/golem/internal/ui/plan"
	"github.com/fugue-labs/golem/internal/ui/styles"
	"github.com/fugue-labs/gollem/core"
)

func TestActiveToolResultMsgUpdatesWorkflowState(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 3

	toolState := map[string]any{
		"planning": map[string]any{
			"tasks": []map[string]any{{
				"id":          "T1",
				"description": "verify implementation",
				"status":      "completed",
			}},
		},
		"invariants": map[string]any{
			"extracted": true,
			"items": []map[string]any{{
				"id":          "I1",
				"description": "tests pass",
				"kind":        "hard",
				"status":      "pass",
			}},
		},
	}

	updated, _ := m.Update(toolResultMsg{runID: 3, name: "planning", toolState: toolState})
	model := updated.(*Model)

	if got := len(model.planState.Tasks); got != 1 {
		t.Fatalf("plan tasks=%d, want 1", got)
	}
	if got := model.planState.Tasks[0].ID; got != "T1" {
		t.Fatalf("plan task id=%q", got)
	}
	if got := len(model.invariantState.Items); got != 1 {
		t.Fatalf("invariant items=%d, want 1", got)
	}
	if got := model.invariantState.Items[0].ID; got != "I1" {
		t.Fatalf("invariant id=%q", got)
	}
}

func TestAgentDoneMsgUpdatesWorkflowStateFromToolState(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 4
	m.busy = true

	toolState := map[string]any{
		"planning": map[string]any{
			"tasks": []map[string]any{{
				"id":          "T2",
				"description": "ship panel",
				"status":      "in_progress",
			}},
		},
		"invariants": map[string]any{
			"extracted": true,
			"items": []map[string]any{{
				"id":          "I2",
				"description": "no TODOs",
				"kind":        "hard",
				"status":      "unknown",
			}},
		},
	}

	updated, _ := m.Update(agentDoneMsg{runID: 4, toolState: toolState, messages: []core.ModelMessage{}})
	model := updated.(*Model)

	if model.busy {
		t.Fatal("expected busy=false")
	}
	if got := len(model.planState.Tasks); got != 1 {
		t.Fatalf("plan tasks=%d, want 1", got)
	}
	if got := model.planState.Tasks[0].ID; got != "T2" {
		t.Fatalf("plan task id=%q", got)
	}
	if got := len(model.invariantState.Items); got != 1 {
		t.Fatalf("invariant items=%d, want 1", got)
	}
	if got := model.invariantState.Items[0].Description; got != "no TODOs" {
		t.Fatalf("invariant description=%q", got)
	}
}

func TestViewShowsWorkflowPanelForInvariantOnlyState(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.width = 140
	m.height = 20
	m.invariantState.Extracted = true

	rendered := stripANSI(m.View().Content)
	for _, want := range []string{"Workflow", "Inv 0✓ 0✗ 0?"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("view missing %q\n%s", want, rendered)
		}
	}
}

func TestRenderRuntimeSummaryMessageListsToolSurfaces(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", TeamMode: "auto", RouterModel: "router-mini"})
	m.runtime.CodeModeStatus = "on"
	m.runtime.OpenImageStatus = "off"
	m.runtime.RouterModelName = "router-resolved"

	msg := m.renderRuntimeSummaryMessage()
	for _, want := range []string{"Effective router model:", "**Tool surfaces**", "Guaranteed repo tools:", "Guaranteed workflow tools:", "Execute code: `on`", "Open image: `off`"} {
		if !strings.Contains(msg.Content, want) {
			t.Fatalf("runtime summary missing %q\n%s", want, msg.Content)
		}
	}
	if got := strings.Count(msg.Content, "Delegate:"); got != 1 {
		t.Fatalf("delegate count=%d, want 1\n%s", got, msg.Content)
	}
	if got := strings.Count(msg.Content, "Execute code:"); got != 1 {
		t.Fatalf("execute code count=%d, want 1\n%s", got, msg.Content)
	}
	if got := strings.Count(msg.Content, "Open image:"); got != 1 {
		t.Fatalf("open image count=%d, want 1\n%s", got, msg.Content)
	}
}

func TestSteeringMiddlewareInjectsQueuedMessagesInOrder(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.pendingMsgs = []string{"first follow-up", "second follow-up"}

	var captured []core.ModelMessage
	next := func(_ context.Context, messages []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		captured = append([]core.ModelMessage(nil), messages...)
		return &core.ModelResponse{}, nil
	}

	_, err := m.steeringMiddleware()(context.Background(), nil, nil, nil, next)
	if err != nil {
		t.Fatalf("middleware error: %v", err)
	}
	if got := len(captured); got != 2 {
		t.Fatalf("captured messages=%d, want 2", got)
	}
	for i, want := range []string{"first follow-up", "second follow-up"} {
		req, ok := captured[i].(core.ModelRequest)
		if !ok || len(req.Parts) != 1 {
			t.Fatalf("captured[%d]=%T, want ModelRequest with one part", i, captured[i])
		}
		part, ok := req.Parts[0].(core.UserPromptPart)
		if !ok {
			t.Fatalf("captured[%d] part=%T, want UserPromptPart", i, req.Parts[0])
		}
		if part.Content != want {
			t.Fatalf("captured[%d] content=%q, want %q", i, part.Content, want)
		}
	}
	if got := m.pendingCount(); got != 0 {
		t.Fatalf("pending count after drain=%d, want 0", got)
	}
}

func TestRenderInputShowsQueuedCountWhileBusy(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.busy = true
	m.startTime = time.Now().Add(-2 * time.Second)
	m.pendingMsgs = []string{"a", "b"}

	rendered := stripANSI(m.renderInput())
	if !strings.Contains(rendered, "2 queued") {
		t.Fatalf("expected queued count in input status, got %q", rendered)
	}
}

func TestRenderStatusBarShowsQueuedCount(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.width = 140
	m.pendingMsgs = []string{"queued follow-up"}

	rendered := stripANSI(m.renderStatusBar())
	if !strings.Contains(rendered, "queued 1") {
		t.Fatalf("expected queued count in status bar, got %q", rendered)
	}
}

func TestStaleToolResultDoesNotMutateCurrentWorkflowState(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 2
	m.planState = plan.State{Tasks: []plan.Task{{ID: "T1", Description: "current", Status: "completed"}}}
	m.invariantState = uiinvariants.State{Extracted: true, Items: []uiinvariants.Item{{ID: "I1", Description: "current invariant", Kind: "hard", Status: "pass"}}}

	state := map[string]any{
		"planning": map[string]any{"tasks": []map[string]any{{"id": "OLD", "description": "stale", "status": "pending"}}},
		"invariants": map[string]any{"extracted": true, "items": []map[string]any{{"id": "OLD", "description": "stale invariant", "kind": "hard", "status": "fail"}}},
	}

	updated, _ := m.Update(toolResultMsg{runID: 1, name: "planning", toolState: state})
	model := updated.(*Model)

	if got := model.planState.Tasks[0].ID; got != "T1" {
		t.Fatalf("plan state mutated by stale event: %q", got)
	}
	if got := model.invariantState.Items[0].ID; got != "I1" {
		t.Fatalf("invariant state mutated by stale event: %q", got)
	}
}
