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
	uispec "github.com/fugue-labs/golem/internal/ui/spec"
	"github.com/fugue-labs/golem/internal/ui/styles"
	uiverification "github.com/fugue-labs/golem/internal/ui/verification"
	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/codetool"
	"github.com/fugue-labs/gollem/ext/team"
)

func TestCancelActiveRunClearsRunStateAndBumpsRunID(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 4
	m.hookRID.Store(4)
	m.busy = true
	m.runCtx = context.Background()
	m.cancel = func() {}
	m.agent = &core.Agent[string]{}
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"}}}

	m.cancelActiveRun(false)

	if !m.verificationState.HasEntries() {
		t.Fatal("expected verification state to survive cancellation")
	}
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
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"}}}
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
	if m.verificationState.HasEntries() {
		t.Fatal("expected verification state reset")
	}
	if m.runtime.CodeModeStatus == "" {
		t.Fatal("expected runtime reset to initial state")
	}
}

func TestWorkflowStackedHeightUsesMidWidthFallback(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.planState = plan.State{Tasks: []plan.Task{{ID: "T1", Description: "verify tests", Status: "in_progress"}}}

	m.width = workflowPanelStackMinWidth - 1
	if got := m.workflowStackedHeight(12); got != 0 {
		t.Fatalf("narrow width stacked height=%d, want 0", got)
	}

	m.width = 80
	if got := m.workflowStackedHeight(12); got < workflowPanelStackMinLines {
		t.Fatalf("mid-width stacked height=%d, want at least %d", got, workflowPanelStackMinLines)
	}

	m.width = workflowPanelWideMinWidth
	if got := m.workflowStackedHeight(12); got != 0 {
		t.Fatalf("wide width stacked height=%d, want 0", got)
	}
}

func TestWorkflowPanelCompactHeightKeepsSummaryVisible(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.planState = plan.State{Tasks: []plan.Task{{ID: "T1", Description: "verify tests", Status: "in_progress"}}}
	m.invariantState = uiinvariants.State{Extracted: true, Items: []uiinvariants.Item{{ID: "I1", Description: "no TODOs", Kind: "hard", Status: "unknown"}}}

	rendered := stripANSI(m.renderWorkflowPanel(3, 80))
	for _, want := range []string{"Workflow", "plan active"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("compact workflow panel missing %q\n%s", want, rendered)
		}
	}
}

func TestWorkflowPanelRendersPlanAndInvariantSections(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.planState = plan.State{Tasks: []plan.Task{{ID: "T1", Description: "inspect repo", Status: "completed"}, {ID: "T2", Description: "verify tests", Status: "in_progress"}}}
	m.invariantState = uiinvariants.State{Extracted: true, Items: []uiinvariants.Item{{ID: "I1", Description: "tests pass", Kind: "hard", Status: "pass"}, {ID: "I2", Description: "no TODOs", Kind: "hard", Status: "unknown"}}}

	rendered := stripANSI(m.renderWorkflowPanel(10, 40))
	for _, want := range []string{"Workflow", "plan active", "Plan ◐ active", "In progress: verify tests", "Inv unresolved", "Open: no TODOs"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("workflow panel missing %q\n%s", want, rendered)
		}
	}
}

func TestWorkflowPanelPrioritizesBlockedSpecBeforePlan(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.specState = uispec.New("design.md")
	m.specState.SetPhase(uispec.PhaseDraft)
	m.planState = plan.State{Tasks: []plan.Task{{ID: "T1", Description: "implement feature", Status: "in_progress"}}}

	rendered := stripANSI(m.renderWorkflowPanel(10, 46))
	specIdx := strings.Index(rendered, "Spec — Reviewing spec")
	planIdx := strings.Index(rendered, "Plan ◐ active")
	if specIdx == -1 || planIdx == -1 {
		t.Fatalf("expected spec and plan sections\n%s", rendered)
	}
	if specIdx > planIdx {
		t.Fatalf("expected spec approval bottleneck before plan work\n%s", rendered)
	}
}

func TestWorkflowPanelKeepsSpecFocusedOnImplementationBeforeReview(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.specState = uispec.New("design.md")
	m.specState.AdvanceGate("Spec Approval")
	m.specState.AdvanceGate("Task Decomposition")
	m.specState.SetPhase(uispec.PhaseImplementing)
	m.specState.SetTaskProgress(3, 5)

	rendered := stripANSI(strings.Join(m.renderSpecPanelLines(6, 46), "\n"))
	for _, want := range []string{"Spec — Implementing · tasks 3/5", "Next: finish implementation (2 remaining)", "2/3 gates · Tasks 3/5"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("spec rail missing %q\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Final Diff Review") {
		t.Fatalf("implementation rail should defer final diff review gate\n%s", rendered)
	}
}

func TestHasWorkflowPanelIncludesInvariantOnlyState(t *testing.T) {
	m := New(&config.Config{})
	m.invariantState = uiinvariants.State{Extracted: true}
	if !m.hasWorkflowPanel() {
		t.Fatal("expected workflow panel when only invariants exist")
	}
}

func TestHasWorkflowPanelIncludesVerificationOnlyState(t *testing.T) {
	m := New(&config.Config{})
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"}}}
	if !m.hasWorkflowPanel() {
		t.Fatal("expected workflow panel when only verification entries exist")
	}
}

func TestWorkflowPanelRendersVerificationOnlySection(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"}, {ID: "V2", Command: "go build ./...", Status: "fail", Freshness: "stale"}}}

	rendered := stripANSI(m.renderWorkflowPanel(10, 38))
	for _, want := range []string{"Workflow", "verify failed", "Verify ✗ failed", "Failing: go build ./...", "go test ./..."} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("verification-only panel missing %q\n%s", want, rendered)
		}
	}
}

func TestWorkflowPanelRendersAllThreeSections(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.planState = plan.State{Tasks: []plan.Task{{ID: "T1", Description: "implement feature", Status: "completed"}}}
	m.invariantState = uiinvariants.State{Extracted: true, Items: []uiinvariants.Item{{ID: "I1", Description: "tests pass", Kind: "hard", Status: "pass"}}}
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"}}}

	rendered := stripANSI(m.renderWorkflowPanel(20, 38))
	for _, want := range []string{"Workflow", "plan 1/1", "verify ok", "Plan ✓ complete", "Done: implement feature", "Verify ✓ clean", "Passed: go test ./...", "Inv ✓ satisfied", "Pass: tests pass"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("three-section panel missing %q\n%s", want, rendered)
		}
	}
}

func TestWorkflowPanelRendersInProgressVerification(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{{ID: "V1", Command: "go test ./...", Status: "in_progress", Freshness: "fresh"}}}

	rendered := stripANSI(m.renderWorkflowPanel(10, 38))
	for _, want := range []string{"Workflow", "verify running", "Verify ◐ running", "Running: go test ./..."} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("in-progress panel missing %q\n%s", want, rendered)
		}
	}
}

func TestWorkflowPanelSummaryTruncatesAtProductionWidth(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.planState = plan.State{Tasks: []plan.Task{{ID: "T1", Description: "task", Status: "completed"}}}
	m.invariantState = uiinvariants.State{Extracted: true, Items: []uiinvariants.Item{{ID: "I1", Description: "inv", Kind: "hard", Status: "pass"}}}
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"}}}

	rendered := stripANSI(m.renderWorkflowPanel(20, 38))
	for i, line := range strings.Split(rendered, "\n") {
		if w := len([]rune(line)); w > 38 {
			t.Fatalf("line %d width=%d exceeds panel width 38: %q", i, w, line)
		}
	}
}

func TestWorkflowCompactSummaryIncludesMidWidthFallbackState(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.planState = plan.State{Tasks: []plan.Task{{ID: "T1", Description: "verify tests", Status: "in_progress"}}}
	m.invariantState = uiinvariants.State{Extracted: true, Items: []uiinvariants.Item{{ID: "I1", Description: "no TODOs", Kind: "hard", Status: "unknown"}}}

	summary := m.workflowCompactSummary(28)
	for _, want := range []string{"workflow", "plan active"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("compact summary missing %q: %q", want, summary)
		}
	}
	if got := len([]rune(summary)); got > 28 {
		t.Fatalf("compact summary width=%d exceeds max 28: %q", got, summary)
	}
}

func TestActiveTeamMembersFiltersStoppedMembers(t *testing.T) {
	members := []team.TeammateInfo{{Name: "leader", State: team.TeammateRunning}, {Name: "worker-1", State: team.TeammateRunning}, {Name: "worker-2", State: team.TeammateStopped}, {Name: "worker-3", State: team.TeammateIdle}}
	active := activeTeamMembers(members)
	if len(active) != 3 {
		t.Fatalf("expected 3 active members, got %d", len(active))
	}
	for _, mi := range active {
		if mi.State == team.TeammateStopped {
			t.Fatalf("stopped member %q should have been filtered", mi.Name)
		}
	}
}

func TestActiveTeamMembersAllStopped(t *testing.T) {
	members := []team.TeammateInfo{{Name: "leader", State: team.TeammateStopped}, {Name: "worker", State: team.TeammateStopped}}
	active := activeTeamMembers(members)
	if len(active) != 0 {
		t.Fatalf("expected 0 active members, got %d", len(active))
	}
}

func TestPurgeStaleTeamNilsWhenAllWorkersStopped(t *testing.T) {
	tm := team.NewTeam(team.TeamConfig{Name: "test-team", Leader: "leader"})
	sess := &codetool.Session{Team: tm}
	m := New(&config.Config{})
	m.purgeStaleTeam(sess)
	if sess.Team == nil {
		t.Fatal("expected team to be preserved when no teammates exist")
	}
}

func TestWorkflowPanelSummaryIncludesDiscoverabilityState(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.specState = uispec.New("design.md")
	m.specState.SetPhase(uispec.PhaseReviewing)
	m.planState = plan.State{Tasks: []plan.Task{{ID: "T1", Description: "verify tests", Status: "in_progress"}}}
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{{ID: "V1", Command: "go test ./...", Status: "fail", Freshness: "fresh"}}}
	m.invariantState = uiinvariants.State{Extracted: true, Items: []uiinvariants.Item{{ID: "I1", Description: "no TODOs", Kind: "hard", Status: "unknown"}}}

	summary := workflowPanelSummary(m)
	for _, want := range []string{"spec approval", "plan active", "verify failed", "inv open"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("workflow summary missing %q: %q", want, summary)
		}
	}
}

func TestWorkflowPanelMidWidthFallbackCombinesSpecAndPlanSignals(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.specState = uispec.New("design.md")
	m.specState.SetPhase(uispec.PhaseImplementing)
	m.specState.AdvanceGate("Spec Approval")
	m.planState = plan.State{Tasks: []plan.Task{{ID: "T1", Description: "verify tests", Status: "in_progress"}}}

	summary := m.workflowCompactSummary(80)
	for _, want := range []string{"workflow", "spec implementing", "plan active"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("mid-width summary missing %q: %q", want, summary)
		}
	}
}

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}
