package ui

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/mission"
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
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{{
		ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh",
	}}}

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
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{{
		ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh",
	}}}
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

func TestWorkflowPanelRendersPlanAndInvariantSections(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.planState = plan.State{Tasks: []plan.Task{{ID: "T1", Description: "inspect repo", Status: "completed"}, {ID: "T2", Description: "verify tests", Status: "in_progress"}}}
	m.invariantState = uiinvariants.State{Extracted: true, Items: []uiinvariants.Item{{ID: "I1", Description: "tests pass", Kind: "hard", Status: "pass"}, {ID: "I2", Description: "no TODOs", Kind: "hard", Status: "unknown"}}}

	rendered := stripANSI(m.renderWorkflowPanel(10, 40))

	for _, want := range []string{"Workflow", "Plan", "1/2 done", "Invariants", "hard 1✓ 0✗ 1?", "inspect repo", "tests pass"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("workflow panel missing %q\n%s", want, rendered)
		}
	}
}

func TestWorkflowPanelRendersSpecSectionWithSeparatorAndCompactFileLabel(t *testing.T) {
	store := mission.NewInMemoryStore()
	ctrl := mission.NewController(store)
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.missionCtrl = ctrl

	created, err := ctrl.CreateMission(context.Background(), mission.CreateMissionRequest{
		Title:    "Test mission",
		Goal:     "Implement feature",
		RepoRoot: "/tmp/repo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ctrl.ApplyPlan(context.Background(), created.ID, &mission.PlanResult{
		Summary: "test plan",
		Tasks: []mission.PlanTask{{
			ID:        "t_1",
			Title:     "Implement feature",
			Kind:      mission.TaskKindCode,
			Objective: "Implement feature",
			Priority:  1,
			RiskLevel: mission.RiskLow,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ctrl.StartMission(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	m.activeMissionID = created.ID
	m.specState = uispec.New("requirements/auth/spec.md")

	rendered := stripANSI(m.renderWorkflowPanel(12, 34))
	lines := strings.Split(rendered, "\n")
	sepLine := strings.Repeat(styles.Separator, 32)
	missionLine, specLine := -1, -1
	for i, line := range lines {
		if strings.Contains(line, "Mission") {
			missionLine = i
		}
		if strings.Contains(line, "Spec") {
			specLine = i
		}
	}
	if missionLine == -1 || specLine == -1 {
		t.Fatalf("expected mission and spec sections\n%s", rendered)
	}
	foundSep := false
	for _, line := range lines[missionLine+1 : specLine] {
		if strings.Contains(line, sepLine) {
			foundSep = true
			break
		}
	}
	if !foundSep {
		t.Fatalf("expected separator between mission and spec sections\n%s", rendered)
	}
	if !strings.Contains(rendered, "file spec.md") {
		t.Fatalf("expected compact spec file label\n%s", rendered)
	}
	if strings.Contains(rendered, "requirements/auth/spec.md") {
		t.Fatalf("expected full spec path to be compacted at narrow width\n%s", rendered)
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

	for _, want := range []string{"Workflow", "Verification", "1✓ 1✗", "go test ./...", "go build ./..."} {
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

	for _, want := range []string{"Workflow", "Plan", "1/1 done", "Invariants", "hard 1✓ 0✗ 0?", "Verification", "1✓ 0✗ 0◐ 0*", "implement feature", "tests pass", "go test ./..."} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("three-section panel missing %q\n%s", want, rendered)
		}
	}
	if !strings.Contains(rendered, "plan 1/1") {
		t.Fatalf("summary missing plan part\n%s", rendered)
	}
}

func TestWorkflowPanelRendersInProgressVerification(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{{ID: "V1", Command: "go test ./...", Status: "in_progress", Freshness: "fresh"}}}

	rendered := stripANSI(m.renderWorkflowPanel(10, 38))

	for _, want := range []string{"Workflow", "1◐", "go test ./..."} {
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

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}
