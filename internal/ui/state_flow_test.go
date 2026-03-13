package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fugue-labs/golem/internal/config"
	uiinvariants "github.com/fugue-labs/golem/internal/ui/invariants"
	"github.com/fugue-labs/golem/internal/ui/plan"
	"github.com/fugue-labs/golem/internal/ui/styles"
	uiverification "github.com/fugue-labs/golem/internal/ui/verification"
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
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", APIKey: "test-key", Timeout: time.Minute, TeamMode: "auto", RouterModel: "router-mini"})
	m.runtime.CodeModeStatus = "on"
	m.runtime.OpenImageStatus = "off"
	m.runtime.RouterModelName = "router-resolved"

	msg := m.renderRuntimeSummaryMessage()
	for _, want := range []string{"Effective router model:", "**Tool surfaces**", "Guaranteed repo tools:", "Guaranteed workflow tools:", "Delegate: `off`", "Execute code: `on`", "Open image: `off`", "Fetch URL: `on`", "Ask user: `pending`", "Team mode: `auto` (effective: `off`)"} {
		if !strings.Contains(msg.Content, want) {
			t.Fatalf("runtime summary missing %q\n%s", want, msg.Content)
		}
	}
	for _, label := range []string{"Delegate:", "Execute code:", "Open image:"} {
		if got := strings.Count(msg.Content, label); got != 1 {
			t.Fatalf("%s count=%d, want 1\n%s", label, got, msg.Content)
		}
	}
}

func TestRenderRuntimeSummaryMessageIncludesValidationWarnings(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", APIKey: "test-key", Timeout: time.Minute, TeamMode: "on", DisableDelegate: true})
	m.runtime.CodeModeStatus = "off"
	m.runtime.OpenImageStatus = "off"

	msg := m.renderRuntimeSummaryMessage()
	if !strings.Contains(msg.Content, "**Validation warnings**") {
		t.Fatalf("expected validation warnings section\n%s", msg.Content)
	}
	if !strings.Contains(msg.Content, "delegate is disabled") {
		t.Fatalf("expected delegate warning\n%s", msg.Content)
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

	mw := m.steeringMiddleware()
	_, err := mw.Request(context.Background(), nil, nil, nil, next)
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

func TestToolResultMsgUpdatesVerificationState(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 5

	toolState := map[string]any{
		"verification": map[string]any{
			"entries": []map[string]any{{
				"id":        "V1",
				"command":   "go test ./...",
				"status":    "pass",
				"freshness": "fresh",
				"summary":   "ok all packages",
			}},
		},
	}

	updated, _ := m.Update(toolResultMsg{runID: 5, name: "verification", toolState: toolState})
	model := updated.(*Model)

	if !model.verificationState.HasEntries() {
		t.Fatal("expected verification state to be updated")
	}
	if got := len(model.verificationState.Entries); got != 1 {
		t.Fatalf("entries=%d, want 1", got)
	}
	entry := model.verificationState.Entries[0]
	if entry.ID != "V1" || entry.Command != "go test ./..." || entry.Status != "pass" {
		t.Fatalf("entry=%+v", entry)
	}
}

func TestAgentDoneMsgUpdatesVerificationState(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 6
	m.busy = true

	toolState := map[string]any{
		"verification": map[string]any{
			"entries": []map[string]any{
				{"id": "V1", "command": "go test ./...", "status": "pass", "freshness": "stale", "stale_by": "edit main.go"},
				{"id": "V2", "command": "go build ./...", "status": "fail", "freshness": "fresh"},
			},
		},
	}

	updated, _ := m.Update(agentDoneMsg{runID: 6, toolState: toolState, messages: []core.ModelMessage{}})
	model := updated.(*Model)

	if model.busy {
		t.Fatal("expected busy=false")
	}
	if got := len(model.verificationState.Entries); got != 2 {
		t.Fatalf("entries=%d, want 2", got)
	}
	if model.verificationState.Entries[0].Freshness != "stale" {
		t.Fatalf("V1 freshness=%q, want stale", model.verificationState.Entries[0].Freshness)
	}
	if model.verificationState.Entries[1].Status != "fail" {
		t.Fatalf("V2 status=%q, want fail", model.verificationState.Entries[1].Status)
	}
}

func TestVerificationStatePersistsAcrossPrompts(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.verificationState = uiverification.State{
		Entries: []uiverification.Entry{{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"}},
	}
	m.input.SetValue("follow-up question")

	updated, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	model := updated.(*Model)

	if !model.verificationState.HasEntries() {
		t.Fatal("expected verification state to persist across new prompt")
	}
	if got := model.verificationState.Entries[0].Command; got != "go test ./..." {
		t.Fatalf("verification command=%q", got)
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
		"planning":   map[string]any{"tasks": []map[string]any{{"id": "OLD", "description": "stale", "status": "pending"}}},
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

func TestStaleToolResultDoesNotMutateVerificationState(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 2
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{
		{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"},
	}}

	staleState := map[string]any{
		"verification": map[string]any{
			"entries": []map[string]any{{
				"id": "OLD", "command": "old cmd", "status": "fail", "freshness": "stale",
			}},
		},
	}

	updated, _ := m.Update(toolResultMsg{runID: 1, name: "verification", toolState: staleState})
	model := updated.(*Model)

	if got := model.verificationState.Entries[0].ID; got != "V1" {
		t.Fatalf("verification state mutated by stale event: %q", got)
	}
}

func TestMutatingToolAutoMarksVerificationStale(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 7
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{
		{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"},
		{ID: "V2", Command: "go build ./...", Status: "pass", Freshness: "fresh"},
	}}

	// A successful edit should auto-mark all fresh entries stale.
	updated, _ := m.Update(toolResultMsg{runID: 7, name: "edit", toolState: map[string]any{}})
	model := updated.(*Model)

	for _, e := range model.verificationState.Entries {
		if e.Freshness != "stale" {
			t.Fatalf("entry %s freshness=%q after edit, want stale", e.ID, e.Freshness)
		}
		if e.StaleBy != "edit" {
			t.Fatalf("entry %s StaleBy=%q, want edit", e.ID, e.StaleBy)
		}
	}
}

func TestFailedMutatingToolDoesNotMarkStale(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 8
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{
		{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"},
	}}

	// A failed edit should NOT mark entries stale.
	updated, _ := m.Update(toolResultMsg{runID: 8, name: "edit", errText: "file not found", toolState: map[string]any{}})
	model := updated.(*Model)

	if model.verificationState.Entries[0].Freshness != "fresh" {
		t.Fatal("expected freshness to remain fresh after failed edit")
	}
}

func TestNonMutatingToolDoesNotMarkStale(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 9
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{
		{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"},
	}}

	// A non-mutating tool (e.g. grep) should NOT mark entries stale.
	updated, _ := m.Update(toolResultMsg{runID: 9, name: "grep", toolState: map[string]any{}})
	model := updated.(*Model)

	if model.verificationState.Entries[0].Freshness != "fresh" {
		t.Fatal("expected freshness to remain fresh after non-mutating tool")
	}
}

func TestVerifyCommandRendersVerificationSummary(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{
		{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"},
	}}
	m.input.SetValue("/verify")

	updated, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	model := updated.(*Model)

	if len(model.messages) == 0 {
		t.Fatal("expected message to be appended")
	}
	last := model.messages[len(model.messages)-1]
	if !strings.Contains(last.Content, "Verification summary") {
		t.Fatalf("expected verification summary, got %q", last.Content)
	}
	if model.input.Value() != "" {
		t.Fatal("expected input to be reset")
	}
}

// ---------- usageUpdateMsg tests ----------

func TestUsageUpdateMsgSetsEstimatedTokens(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 10

	updated, _ := m.Update(usageUpdateMsg{runID: 10, inputTokens: 42000})
	model := updated.(*Model)

	if model.estimatedTokens != 42000 {
		t.Fatalf("estimatedTokens=%d, want 42000", model.estimatedTokens)
	}
}

func TestUsageUpdateMsgIgnoresStaleRunID(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 10
	m.estimatedTokens = 5000

	updated, _ := m.Update(usageUpdateMsg{runID: 9, inputTokens: 99000})
	model := updated.(*Model)

	if model.estimatedTokens != 5000 {
		t.Fatalf("estimatedTokens=%d, want 5000 (stale runID should be ignored)", model.estimatedTokens)
	}
}

func TestUsageUpdateMsgIgnoresZeroTokens(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 10
	m.estimatedTokens = 5000

	updated, _ := m.Update(usageUpdateMsg{runID: 10, inputTokens: 0})
	model := updated.(*Model)

	if model.estimatedTokens != 5000 {
		t.Fatalf("estimatedTokens=%d, want 5000 (zero tokens should be ignored)", model.estimatedTokens)
	}
}

// ---------- applyWorkflowToolState tests ----------

func TestApplyWorkflowToolStatePreservesStateWhenKeysAbsent(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.planState = plan.State{Tasks: []plan.Task{
		{ID: "T1", Description: "existing task", Status: "completed"},
	}}
	m.invariantState = uiinvariants.State{
		Extracted: true,
		Items:     []uiinvariants.Item{{ID: "I1", Description: "existing", Kind: "hard", Status: "pass"}},
	}
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{
		{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"},
	}}

	// Apply empty tool state — no planning/invariants/verification keys.
	m.applyWorkflowToolState(map[string]any{})

	if len(m.planState.Tasks) != 1 || m.planState.Tasks[0].ID != "T1" {
		t.Fatalf("plan state was mutated by empty tool state")
	}
	if len(m.invariantState.Items) != 1 || m.invariantState.Items[0].ID != "I1" {
		t.Fatalf("invariant state was mutated by empty tool state")
	}
	if len(m.verificationState.Entries) != 1 || m.verificationState.Entries[0].ID != "V1" {
		t.Fatalf("verification state was mutated by empty tool state")
	}
}

func TestApplyWorkflowToolStateUpdatesWhenKeysPresent(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.planState = plan.State{Tasks: []plan.Task{
		{ID: "T-OLD", Description: "old task", Status: "completed"},
	}}

	toolState := map[string]any{
		"planning": map[string]any{
			"tasks": []map[string]any{{
				"id":          "T-NEW",
				"description": "new task",
				"status":      "in_progress",
			}},
		},
		"invariants": map[string]any{
			"extracted": true,
			"items": []map[string]any{{
				"id":          "I-NEW",
				"description": "new invariant",
				"kind":        "hard",
				"status":      "pass",
			}},
		},
		"verification": map[string]any{
			"entries": []map[string]any{{
				"id":        "V-NEW",
				"command":   "go build ./...",
				"status":    "pass",
				"freshness": "fresh",
			}},
		},
	}

	m.applyWorkflowToolState(toolState)

	if len(m.planState.Tasks) != 1 || m.planState.Tasks[0].Description != "new task" {
		t.Fatalf("plan state not updated: %+v", m.planState)
	}
	if len(m.invariantState.Items) != 1 || m.invariantState.Items[0].Description != "new invariant" {
		t.Fatalf("invariant state not updated: %+v", m.invariantState)
	}
	if len(m.verificationState.Entries) != 1 || m.verificationState.Entries[0].Command != "go build ./..." {
		t.Fatalf("verification state not updated: %+v", m.verificationState)
	}
}

func TestApplyWorkflowToolStateNilToolState(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.planState = plan.State{Tasks: []plan.Task{
		{ID: "T1", Description: "existing", Status: "completed"},
	}}

	// nil tool state should not panic or mutate existing state.
	m.applyWorkflowToolState(nil)

	if len(m.planState.Tasks) != 1 || m.planState.Tasks[0].ID != "T1" {
		t.Fatalf("plan state mutated by nil tool state")
	}
}

// ---------- contextCompactedMsg tests ----------

func TestContextCompactedMsgUpdatesEstimatedTokens(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.estimatedTokens = 100000

	updated, _ := m.Update(contextCompactedMsg{
		strategy:     "auto_summary",
		msgsBefore:   50,
		msgsAfter:    20,
		tokensBefore: 100000,
		tokensAfter:  30000,
	})
	model := updated.(*Model)

	if model.estimatedTokens != 30000 {
		t.Fatalf("estimatedTokens=%d, want 30000", model.estimatedTokens)
	}
}

func TestContextCompactedMsgSuppressesHistoryProcessorMessages(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	msgsBefore := len(m.messages)

	updated, _ := m.Update(contextCompactedMsg{
		strategy:     "history_processor",
		msgsBefore:   50,
		msgsAfter:    48,
		tokensBefore: 100000,
		tokensAfter:  95000,
	})
	model := updated.(*Model)

	if len(model.messages) != msgsBefore {
		t.Fatalf("history_processor should not append a message, got %d messages (was %d)", len(model.messages), msgsBefore)
	}
	// But tokens should still update.
	if model.estimatedTokens != 95000 {
		t.Fatalf("estimatedTokens=%d, want 95000", model.estimatedTokens)
	}
}

func TestContextCompactedMsgShowsMessageForAutoSummary(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	msgsBefore := len(m.messages)

	updated, _ := m.Update(contextCompactedMsg{
		strategy:     "auto_summary",
		msgsBefore:   50,
		msgsAfter:    20,
		tokensBefore: 100000,
		tokensAfter:  30000,
	})
	model := updated.(*Model)

	if len(model.messages) != msgsBefore+1 {
		t.Fatalf("auto_summary should append a message, got %d messages (was %d)", len(model.messages), msgsBefore)
	}
	content := model.messages[len(model.messages)-1].Content
	if !strings.Contains(content, "Auto-compact") {
		t.Fatalf("expected Auto-compact label, got %q", content)
	}
}

func TestContextCompactedMsgShowsEmergencyLabel(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})

	updated, _ := m.Update(contextCompactedMsg{
		strategy:     "emergency_truncation",
		msgsBefore:   50,
		msgsAfter:    10,
		tokensBefore: 200000,
		tokensAfter:  50000,
	})
	model := updated.(*Model)

	if len(model.messages) == 0 {
		t.Fatal("expected a message to be appended")
	}
	content := model.messages[len(model.messages)-1].Content
	if !strings.Contains(content, "Emergency truncation") {
		t.Fatalf("expected Emergency truncation label, got %q", content)
	}
}
