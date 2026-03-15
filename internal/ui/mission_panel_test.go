package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/mission"
	"github.com/fugue-labs/golem/internal/ui/chat"
	"github.com/fugue-labs/golem/internal/ui/styles"
)

// testMissionModel creates a Model wired to an in-memory mission store.
func testMissionModel(t *testing.T) (*Model, *mission.Controller) {
	t.Helper()
	store := mission.NewInMemoryStore()
	ctrl := mission.NewController(store)
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.missionCtrl = ctrl
	t.Cleanup(func() {
		m.stopOrchestrator()
		m.appCancel()
	})
	return m, ctrl
}

// seedMission creates a mission with tasks at various statuses and returns the mission ID.
func seedMission(t *testing.T, ctrl *mission.Controller, status mission.MissionStatus, tasks []mission.Task) string {
	t.Helper()
	ctx := context.Background()

	created, err := ctrl.CreateMission(ctx, mission.CreateMissionRequest{
		Title:    "Test mission",
		Goal:     "Verify the mission panel renders correctly",
		RepoRoot: "/tmp/repo",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Transition to desired status through valid state machine path.
	if status != mission.MissionDraft {
		// Need to apply a plan first to get to awaiting_approval.
		planTasks := make([]mission.PlanTask, len(tasks))
		for i, task := range tasks {
			planTasks[i] = mission.PlanTask{
				ID:        task.ID,
				Title:     task.Title,
				Kind:      task.Kind,
				Objective: task.Objective,
				Priority:  task.Priority,
				RiskLevel: task.RiskLevel,
			}
		}
		if err := ctrl.ApplyPlan(ctx, created.ID, &mission.PlanResult{
			Summary: "test plan",
			Tasks:   planTasks,
		}); err != nil {
			t.Fatal(err)
		}

		// Now set each task to its desired status.
		for _, task := range tasks {
			if task.Status == mission.TaskPending || task.Status == mission.TaskReady {
				// ApplyPlan already set tasks to ready/pending based on deps.
				// Override to desired status.
				stored, err := ctrl.Store().GetTask(ctx, task.ID)
				if err != nil {
					t.Fatal(err)
				}
				stored.Status = task.Status
				if err := ctrl.Store().UpdateTask(ctx, stored); err != nil {
					t.Fatal(err)
				}
				continue
			}
			stored, err := ctrl.Store().GetTask(ctx, task.ID)
			if err != nil {
				t.Fatal(err)
			}
			stored.Status = task.Status
			if err := ctrl.Store().UpdateTask(ctx, stored); err != nil {
				t.Fatal(err)
			}
		}

		if status == mission.MissionRunning || status == mission.MissionPaused || status == mission.MissionCancelled {
			if err := ctrl.StartMission(ctx, created.ID); err != nil {
				t.Fatal(err)
			}
		}
		if status == mission.MissionPaused {
			if err := ctrl.PauseMission(ctx, created.ID); err != nil {
				t.Fatal(err)
			}
		}
		if status == mission.MissionCancelled {
			if err := ctrl.CancelMission(ctx, created.ID); err != nil {
				t.Fatal(err)
			}
		}
	}

	return created.ID
}

func TestMissionPanelRendersTaskGraph(t *testing.T) {
	m, ctrl := testMissionModel(t)

	tasks := []mission.Task{
		{ID: "t_impl", Title: "Implement feature", Kind: mission.TaskKindCode, Status: mission.TaskDone, Priority: 3, RiskLevel: mission.RiskLow},
		{ID: "t_test", Title: "Write tests", Kind: mission.TaskKindTest, Status: mission.TaskRunning, Priority: 2, RiskLevel: mission.RiskLow},
		{ID: "t_docs", Title: "Update docs", Kind: mission.TaskKindDocs, Status: mission.TaskReady, Priority: 1, RiskLevel: mission.RiskLow},
	}
	missionID := seedMission(t, ctrl, mission.MissionRunning, tasks)
	m.activeMissionID = missionID

	lines := m.renderMissionPanelLines(15, 40)
	rendered := stripANSI(strings.Join(lines, "\n"))

	// Header must show running status.
	if !strings.Contains(rendered, "Mission") {
		t.Fatalf("missing mission header\n%s", rendered)
	}
	if !strings.Contains(rendered, string(mission.MissionRunning)) {
		t.Fatalf("missing running status\n%s", rendered)
	}

	for _, want := range []string{"In progress: Write tests", "Next: Update docs", "Tasks"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing active mission focus %q\n%s", want, rendered)
		}
	}

	// Individual task lines.
	if !strings.Contains(rendered, "Implement feature") {
		t.Fatalf("missing task: Implement feature\n%s", rendered)
	}
	if !strings.Contains(rendered, "Write tests") {
		t.Fatalf("missing task: Write tests\n%s", rendered)
	}
	if !strings.Contains(rendered, "Update docs") {
		t.Fatalf("missing task: Update docs\n%s", rendered)
	}
}

func TestMissionPanelSurfacesBlockedTaskBeforeMissionMetadata(t *testing.T) {
	m, ctrl := testMissionModel(t)

	tasks := []mission.Task{
		{ID: "t_blocked", Title: "Unblock dependency", Kind: mission.TaskKindCode, Status: mission.TaskBlocked, Priority: 2, RiskLevel: mission.RiskLow, BlockingReason: "waiting on API schema"},
		{ID: "t_ready", Title: "Implement endpoint", Kind: mission.TaskKindCode, Status: mission.TaskReady, Priority: 1, RiskLevel: mission.RiskLow},
	}
	missionID := seedMission(t, ctrl, mission.MissionRunning, tasks)
	m.activeMissionID = missionID

	rendered := stripANSI(strings.Join(m.renderMissionPanelLines(6, 60), "\n"))
	blockedIdx := strings.Index(rendered, "Blocked: Unblock dependency")
	progressIdx := strings.Index(rendered, "Tasks")
	if blockedIdx == -1 || progressIdx == -1 {
		t.Fatalf("expected blocked focus and task counts\n%s", rendered)
	}
	if blockedIdx > progressIdx {
		t.Fatalf("expected blocked focus before secondary mission metadata\n%s", rendered)
	}
}

func TestMissionPanelTaskCountIcons(t *testing.T) {
	m, ctrl := testMissionModel(t)

	tasks := []mission.Task{
		{ID: "t_done", Title: "Done task", Kind: mission.TaskKindCode, Status: mission.TaskDone, Priority: 5, RiskLevel: mission.RiskLow},
		{ID: "t_run", Title: "Running task", Kind: mission.TaskKindCode, Status: mission.TaskRunning, Priority: 4, RiskLevel: mission.RiskLow},
		{ID: "t_ready", Title: "Ready task", Kind: mission.TaskKindCode, Status: mission.TaskReady, Priority: 3, RiskLevel: mission.RiskLow},
		{ID: "t_blocked", Title: "Blocked task", Kind: mission.TaskKindCode, Status: mission.TaskBlocked, Priority: 2, RiskLevel: mission.RiskLow},
		{ID: "t_pending", Title: "Pending task", Kind: mission.TaskKindCode, Status: mission.TaskPending, Priority: 1, RiskLevel: mission.RiskLow},
	}
	missionID := seedMission(t, ctrl, mission.MissionRunning, tasks)
	m.activeMissionID = missionID

	lines := m.renderMissionPanelLines(15, 50)
	rendered := stripANSI(strings.Join(lines, "\n"))

	// Task count line should contain status indicators.
	if !strings.Contains(rendered, "1✓") {
		t.Fatalf("missing done count (1✓)\n%s", rendered)
	}
	if !strings.Contains(rendered, "1◐") {
		t.Fatalf("missing running count (1◐)\n%s", rendered)
	}
	if !strings.Contains(rendered, "1●") {
		t.Fatalf("missing ready count (1●)\n%s", rendered)
	}
	if !strings.Contains(rendered, "1✗") {
		t.Fatalf("missing blocked count (1✗)\n%s", rendered)
	}
	if !strings.Contains(rendered, "1○") {
		t.Fatalf("missing pending count (1○)\n%s", rendered)
	}
}

func TestMissionPanelLimitTruncatesTaskList(t *testing.T) {
	m, ctrl := testMissionModel(t)

	tasks := make([]mission.Task, 10)
	for i := range tasks {
		tasks[i] = mission.Task{
			ID:        "t_" + string(rune('a'+i)),
			Title:     "Task " + string(rune('A'+i)),
			Kind:      mission.TaskKindCode,
			Status:    mission.TaskReady,
			Priority:  10 - i,
			RiskLevel: mission.RiskLow,
		}
	}
	missionID := seedMission(t, ctrl, mission.MissionRunning, tasks)
	m.activeMissionID = missionID

	// Only allow 6 lines total (header + title + counts + a few tasks).
	lines := m.renderMissionPanelLines(6, 40)
	if len(lines) > 6 {
		t.Fatalf("got %d lines, want at most 6", len(lines))
	}

	rendered := stripANSI(strings.Join(lines, "\n"))
	// Should show overflow indicator or at least preserve active-work focus in the final visible line.
	if !strings.Contains(rendered, "…") && !strings.Contains(rendered, "+") && !strings.Contains(rendered, "Task C") {
		t.Fatalf("expected truncation or focused overflow behavior\n%s", rendered)
	}
}

func TestMissionPanelPrioritizesRunningTaskForNextAction(t *testing.T) {
	m, ctrl := testMissionModel(t)

	tasks := []mission.Task{
		{ID: "t_run", Title: "Implement worker lane", Kind: mission.TaskKindCode, Status: mission.TaskRunning, Priority: 3, RiskLevel: mission.RiskLow},
		{ID: "t_ready", Title: "Polish dashboard", Kind: mission.TaskKindDocs, Status: mission.TaskReady, Priority: 1, RiskLevel: mission.RiskLow},
	}
	missionID := seedMission(t, ctrl, mission.MissionRunning, tasks)
	m.activeMissionID = missionID

	rendered := stripANSI(strings.Join(m.renderMissionPanelLines(8, 70), "\n"))
	if !strings.Contains(rendered, "In progress: Implement worker lane") {
		t.Fatalf("expected running task spotlight\n%s", rendered)
	}
	if !strings.Contains(rendered, "Next: Polish dashboard") {
		t.Fatalf("expected ready task queued next line\n%s", rendered)
	}

	msg, _ := m.handleMissionCommand("/mission status")
	if !strings.Contains(msg.Content, "Monitor active work on Implement worker lane") {
		t.Fatalf("expected running-task next action in status message\n%s", msg.Content)
	}
}

func TestMissionStatusKeepsRunningLifecycleWhileSurfacingBlockedTasks(t *testing.T) {
	m, ctrl := testMissionModel(t)

	tasks := []mission.Task{
		{ID: "t_blocked", Title: "Unblock schema sync", Kind: mission.TaskKindCode, Status: mission.TaskBlocked, Priority: 2, RiskLevel: mission.RiskLow},
		{ID: "t_ready", Title: "Apply follow-up patch", Kind: mission.TaskKindCode, Status: mission.TaskReady, Priority: 1, RiskLevel: mission.RiskLow},
	}
	missionID := seedMission(t, ctrl, mission.MissionRunning, tasks)
	blockedTask, err := ctrl.Store().GetTask(context.Background(), "t_blocked")
	if err != nil {
		t.Fatal(err)
	}
	blockedTask.BlockingReason = "waiting on schema approval"
	if err := ctrl.Store().UpdateTask(context.Background(), blockedTask); err != nil {
		t.Fatal(err)
	}
	m.activeMissionID = missionID

	msg, _ := m.handleMissionCommand("/mission status")
	for _, want := range []string{
		"**Status**: running",
		"**Phase**: Running · ready queue",
		"**Attention**: 1 blocked task(s)",
		"**Next action**: Unblock Unblock schema sync: waiting on schema approval",
		"- Blocked: Unblock schema sync — waiting on schema approval",
	} {
		if !strings.Contains(msg.Content, want) {
			t.Fatalf("status missing %q\n%s", want, msg.Content)
		}
	}
}

func TestMissionPanelSummaryTreatsTaskBlockersAsAttentionNotBlockedLifecycle(t *testing.T) {
	m, ctrl := testMissionModel(t)

	tasks := []mission.Task{
		{ID: "t_blocked", Title: "Resolve migration conflict", Kind: mission.TaskKindCode, Status: mission.TaskBlocked, Priority: 2, RiskLevel: mission.RiskLow},
		{ID: "t_done", Title: "Ship base migration", Kind: mission.TaskKindCode, Status: mission.TaskDone, Priority: 1, RiskLevel: mission.RiskLow},
	}
	missionID := seedMission(t, ctrl, mission.MissionRunning, tasks)
	m.activeMissionID = missionID

	summary := m.missionPanelSummary()
	if strings.Contains(summary, "mission blocked") {
		t.Fatalf("task blockers should not rewrite lifecycle summary: %q", summary)
	}
	if !strings.Contains(summary, "mission attention") || !strings.Contains(summary, "blocked") {
		t.Fatalf("expected attention summary for blocked tasks, got %q", summary)
	}
}

func TestMissionApprovalGateWinsOverPendingApprovalAttention(t *testing.T) {
	m, ctrl := testMissionModel(t)
	ctx := context.Background()
	now := time.Now().UTC()

	created, err := ctrl.CreateMission(ctx, mission.CreateMissionRequest{Title: "Approval gate", Goal: "Need approval", RepoRoot: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ctrl.ApplyPlan(ctx, created.ID, &mission.PlanResult{
		Summary: "plan",
		Tasks:   []mission.PlanTask{{ID: "t_a", Title: "First task", Kind: mission.TaskKindCode, Priority: 1, RiskLevel: mission.RiskLow}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ctrl.Store().CreateApproval(ctx, &mission.Approval{
		ID:        "ap_1",
		MissionID: created.ID,
		Kind:      "plan",
		Status:    mission.ApprovalPending,
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	m.activeMissionID = created.ID

	rendered := stripANSI(strings.Join(m.renderMissionPanelLines(6, 120), "\n"))
	if !strings.Contains(rendered, "Approval: Review the proposed mission plan and approve start with /mission approve") {
		t.Fatalf("expected approval gate spotlight\n%s", rendered)
	}
	if !strings.Contains(m.missionPanelSummary(), "mission awaiting approval") {
		t.Fatalf("expected awaiting approval summary, got %q", m.missionPanelSummary())
	}
}

func TestMissionPanelSummaryEmptyWhenNoMission(t *testing.T) {
	m, _ := testMissionModel(t)
	if got := m.missionPanelSummary(); got != "" {
		t.Fatalf("expected empty summary, got %q", got)
	}
}

func TestMissionPanelReturnsNilWhenNoMission(t *testing.T) {
	m, _ := testMissionModel(t)
	if lines := m.renderMissionPanelLines(10, 40); lines != nil {
		t.Fatalf("expected nil lines for no mission, got %d lines", len(lines))
	}
}

func TestMissionPanelReturnsNilForZeroLimit(t *testing.T) {
	m, ctrl := testMissionModel(t)
	missionID := seedMission(t, ctrl, mission.MissionRunning, []mission.Task{
		{ID: "t_1", Title: "A", Kind: mission.TaskKindCode, Status: mission.TaskReady, Priority: 1, RiskLevel: mission.RiskLow},
	})
	m.activeMissionID = missionID

	if lines := m.renderMissionPanelLines(0, 40); lines != nil {
		t.Fatalf("expected nil for limit=0, got %d lines", len(lines))
	}
}

func TestMissionNewCommandCreatesMission(t *testing.T) {
	m, _ := testMissionModel(t)
	m.cfg.WorkingDir = "/tmp/test-repo"

	msg, _ := m.handleMissionCommand("/mission new Build a REST API server")

	if msg.Kind != chat.KindAssistant {
		t.Fatalf("expected assistant message, got %v", msg.Kind)
	}
	if !strings.Contains(msg.Content, "Mission created") {
		t.Fatalf("expected 'Mission created' in response\n%s", msg.Content)
	}
	if !strings.Contains(msg.Content, "Build a REST API server") {
		t.Fatalf("expected goal in response\n%s", msg.Content)
	}
	if m.activeMissionID == "" {
		t.Fatal("expected activeMissionID to be set")
	}
}

func TestMissionNewCommandTruncatesLongTitle(t *testing.T) {
	m, _ := testMissionModel(t)
	m.cfg.WorkingDir = "/tmp/repo"

	longGoal := strings.Repeat("x", 100)
	msg, _ := m.handleMissionCommand("/mission new " + longGoal)

	if !strings.Contains(msg.Content, "Mission created") {
		t.Fatalf("expected mission created\n%s", msg.Content)
	}
	// The title should be truncated at 80 chars (77 + "...").
	if !strings.Contains(msg.Content, "...") {
		t.Fatalf("expected truncated title\n%s", msg.Content)
	}
}

func TestMissionNewCommandEmptyGoal(t *testing.T) {
	m, _ := testMissionModel(t)
	// "/mission new " with only a trailing space trims to "new" which doesn't
	// match "new " prefix, so falls through to unknown-command error.
	msg, _ := m.handleMissionCommand("/mission new ")

	if msg.Kind != chat.KindError {
		t.Fatalf("expected error for empty goal, got kind=%v", msg.Kind)
	}
	if !strings.Contains(msg.Content, "Unknown mission command") {
		t.Fatalf("expected unknown command error\n%s", msg.Content)
	}
}

func TestMissionStatusDisplayNoMission(t *testing.T) {
	m, _ := testMissionModel(t)
	msg, _ := m.handleMissionCommand("/mission status")

	if msg.Kind != chat.KindAssistant {
		t.Fatalf("expected assistant message, got %v", msg.Kind)
	}
	if !strings.Contains(msg.Content, "No active mission") {
		t.Fatalf("expected 'No active mission'\n%s", msg.Content)
	}
}

func TestMissionStatusDisplayWithTasks(t *testing.T) {
	m, ctrl := testMissionModel(t)

	tasks := []mission.Task{
		{ID: "t_1", Title: "Implement", Kind: mission.TaskKindCode, Status: mission.TaskDone, Priority: 3, RiskLevel: mission.RiskLow},
		{ID: "t_2", Title: "Test", Kind: mission.TaskKindTest, Status: mission.TaskRunning, Priority: 2, RiskLevel: mission.RiskLow},
		{ID: "t_3", Title: "Blocked", Kind: mission.TaskKindCode, Status: mission.TaskBlocked, Priority: 1, RiskLevel: mission.RiskMedium},
	}
	missionID := seedMission(t, ctrl, mission.MissionRunning, tasks)
	m.activeMissionID = missionID

	msg, _ := m.handleMissionCommand("/mission status")

	if msg.Kind != chat.KindAssistant {
		t.Fatalf("expected assistant message, got %v", msg.Kind)
	}
	for _, want := range []string{"Test mission", "running", "Phase", "Next action", "Task DAG", "Done", "Running", "Blocked"} {
		if !strings.Contains(msg.Content, want) {
			t.Fatalf("status missing %q\n%s", want, msg.Content)
		}
	}
}

func TestMissionTasksDisplayWithDependencies(t *testing.T) {
	m, ctrl := testMissionModel(t)
	ctx := context.Background()

	created, err := ctrl.CreateMission(ctx, mission.CreateMissionRequest{
		Title: "Dep test", Goal: "Test deps", RepoRoot: "/tmp",
	})
	if err != nil {
		t.Fatal(err)
	}

	plan := &mission.PlanResult{
		Summary: "dep plan",
		Tasks: []mission.PlanTask{
			{ID: "t_a", Title: "Base work", Kind: mission.TaskKindCode, Priority: 2, RiskLevel: mission.RiskLow},
			{ID: "t_b", Title: "Follow-up", Kind: mission.TaskKindTest, Priority: 1, RiskLevel: mission.RiskLow},
		},
		Dependencies: []mission.TaskDependency{
			{TaskID: "t_b", DependsOnID: "t_a"},
		},
	}
	if err := ctrl.ApplyPlan(ctx, created.ID, plan); err != nil {
		t.Fatal(err)
	}

	m.activeMissionID = created.ID
	msg, _ := m.handleMissionCommand("/mission tasks")

	if msg.Kind != chat.KindAssistant {
		t.Fatalf("expected assistant, got %v", msg.Kind)
	}
	if !strings.Contains(msg.Content, "depends on: t_a") {
		t.Fatalf("expected dependency info\n%s", msg.Content)
	}
	if !strings.Contains(msg.Content, "Base work") {
		t.Fatalf("expected task title\n%s", msg.Content)
	}
}

func TestMissionHelpCommand(t *testing.T) {
	m, _ := testMissionModel(t)
	msg, _ := m.handleMissionCommand("/mission help")

	if msg.Kind != chat.KindAssistant {
		t.Fatalf("expected assistant, got %v", msg.Kind)
	}
	for _, want := range []string{"/mission new", "/mission status", "/mission tasks", "/mission plan", "/mission approve"} {
		if !strings.Contains(msg.Content, want) {
			t.Fatalf("help missing %q\n%s", want, msg.Content)
		}
	}
}

func TestMissionUnknownSubcommand(t *testing.T) {
	m, _ := testMissionModel(t)
	msg, _ := m.handleMissionCommand("/mission foobar")

	if msg.Kind != chat.KindError {
		t.Fatalf("expected error, got %v", msg.Kind)
	}
	if !strings.Contains(msg.Content, "Unknown mission command") {
		t.Fatalf("expected unknown command error\n%s", msg.Content)
	}
}

func TestTaskStateTransitionIcons(t *testing.T) {
	tests := []struct {
		status mission.TaskStatus
		want   string // expected icon string (from styles)
	}{
		{mission.TaskDone, styles.CheckIcon},
		{mission.TaskIntegrated, styles.CheckIcon},
		{mission.TaskAccepted, styles.CheckIcon},
		{mission.TaskRunning, styles.InProgressIcon},
		{mission.TaskLeased, styles.InProgressIcon},
		{mission.TaskBlocked, styles.BlockedIcon},
		{mission.TaskFailed, styles.BlockedIcon},
		{mission.TaskRejected, styles.BlockedIcon},
		{mission.TaskReady, styles.PendingIcon},
		{mission.TaskAwaitingReview, "◎"},
		{mission.TaskPending, styles.HollowIcon},
	}

	m, _ := testMissionModel(t)
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			icon := stripANSI(m.taskIcon(tt.status))
			if !strings.Contains(icon, tt.want) {
				t.Fatalf("taskIcon(%s)=%q, want to contain %q", tt.status, icon, tt.want)
			}
		})
	}
}

func TestMissionStatusIcons(t *testing.T) {
	tests := []struct {
		status mission.MissionStatus
		want   string
	}{
		{mission.MissionCompleted, styles.CheckIcon},
		{mission.MissionRunning, styles.InProgressIcon},
		{mission.MissionPaused, "⏸"},
		{mission.MissionBlocked, styles.BlockedIcon},
		{mission.MissionFailed, styles.ErrorIcon},
		{mission.MissionCancelled, styles.ErrorIcon},
		{mission.MissionAwaitingApproval, "⏳"},
		{mission.MissionPlanning, styles.SpinnerIcon},
		{mission.MissionDraft, styles.HollowIcon},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := missionStatusIcon(tt.status)
			if got != tt.want {
				t.Fatalf("missionStatusIcon(%s)=%q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestMissionPhaseHeaderDisplayAllStatuses(t *testing.T) {
	m, ctrl := testMissionModel(t)

	// Test that the panel header line includes the correct icon for each status.
	statuses := []mission.MissionStatus{
		mission.MissionRunning,
		mission.MissionPaused,
	}

	for i, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			taskID := "t_ph_" + string(rune('a'+i))
			tasks := []mission.Task{
				{ID: taskID, Title: "Phase task", Kind: mission.TaskKindCode, Status: mission.TaskReady, Priority: 1, RiskLevel: mission.RiskLow},
			}
			missionID := seedMission(t, ctrl, status, tasks)
			m.activeMissionID = missionID

			lines := m.renderMissionPanelLines(5, 40)
			header := stripANSI(lines[0])

			expectedIcon := missionStatusIcon(status)
			if !strings.Contains(header, expectedIcon) {
				t.Fatalf("header for %s missing icon %q: %q", status, expectedIcon, header)
			}
			if !strings.Contains(header, strings.ToLower(string(status))) {
				t.Fatalf("header for %s missing status text: %q", status, header)
			}
		})
	}
}

func TestMissionPanelSingleLineLimit(t *testing.T) {
	m, ctrl := testMissionModel(t)
	tasks := []mission.Task{
		{ID: "t_1", Title: "A task", Kind: mission.TaskKindCode, Status: mission.TaskReady, Priority: 1, RiskLevel: mission.RiskLow},
	}
	missionID := seedMission(t, ctrl, mission.MissionRunning, tasks)
	m.activeMissionID = missionID

	lines := m.renderMissionPanelLines(1, 40)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	rendered := stripANSI(lines[0])
	if !strings.Contains(rendered, "Mission") {
		t.Fatalf("single line should contain mission header: %q", rendered)
	}
}

func TestMissionCancelCommand(t *testing.T) {
	m, ctrl := testMissionModel(t)
	ctx := context.Background()

	created, err := ctrl.CreateMission(ctx, mission.CreateMissionRequest{
		Title: "Cancel test", Goal: "Test cancel", RepoRoot: "/tmp",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Apply a plan and start so we can cancel.
	if err := ctrl.ApplyPlan(ctx, created.ID, &mission.PlanResult{
		Summary: "plan",
		Tasks:   []mission.PlanTask{{ID: "t_c", Title: "Task", Kind: mission.TaskKindCode, Priority: 1, RiskLevel: mission.RiskLow}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ctrl.StartMission(ctx, created.ID); err != nil {
		t.Fatal(err)
	}

	m.activeMissionID = created.ID
	msg, _ := m.handleMissionCommand("/mission cancel")

	if msg.Kind != chat.KindAssistant {
		t.Fatalf("expected assistant, got %v", msg.Kind)
	}
	if !strings.Contains(msg.Content, "cancelled") {
		t.Fatalf("expected cancelled confirmation\n%s", msg.Content)
	}
	if m.activeMissionID != "" {
		t.Fatal("expected activeMissionID cleared after cancel")
	}
}

func TestMissionPauseCommand(t *testing.T) {
	m, ctrl := testMissionModel(t)
	ctx := context.Background()

	created, err := ctrl.CreateMission(ctx, mission.CreateMissionRequest{
		Title: "Pause test", Goal: "Test pause", RepoRoot: "/tmp",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := ctrl.ApplyPlan(ctx, created.ID, &mission.PlanResult{
		Summary: "plan",
		Tasks:   []mission.PlanTask{{ID: "t_p", Title: "Task", Kind: mission.TaskKindCode, Priority: 1, RiskLevel: mission.RiskLow}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ctrl.StartMission(ctx, created.ID); err != nil {
		t.Fatal(err)
	}

	m.activeMissionID = created.ID
	msg, _ := m.handleMissionCommand("/mission pause")

	if msg.Kind != chat.KindAssistant {
		t.Fatalf("expected assistant, got %v", msg.Kind)
	}
	if !strings.Contains(msg.Content, "paused") {
		t.Fatalf("expected paused confirmation\n%s", msg.Content)
	}
}

func TestMissionListCommand(t *testing.T) {
	m, ctrl := testMissionModel(t)
	ctx := context.Background()

	m1, err := ctrl.CreateMission(ctx, mission.CreateMissionRequest{Title: "First mission", Goal: "G1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.CreateMission(ctx, mission.CreateMissionRequest{Title: "Second mission", Goal: "G2"}); err != nil {
		t.Fatal(err)
	}

	m.activeMissionID = m1.ID
	msg, _ := m.handleMissionCommand("/mission list")

	if msg.Kind != chat.KindAssistant {
		t.Fatalf("expected assistant, got %v", msg.Kind)
	}
	if !strings.Contains(msg.Content, "First mission") {
		t.Fatalf("list missing first mission\n%s", msg.Content)
	}
	if !strings.Contains(msg.Content, "Second mission") {
		t.Fatalf("list missing second mission\n%s", msg.Content)
	}
	if !strings.Contains(msg.Content, "← active") {
		t.Fatalf("list missing active marker\n%s", msg.Content)
	}
}

func TestHasMissionState(t *testing.T) {
	m, _ := testMissionModel(t)

	if m.hasMissionState() {
		t.Fatal("expected no mission state initially")
	}

	m.activeMissionID = "m_test"
	if !m.hasMissionState() {
		t.Fatal("expected mission state when ID is set")
	}
}

func TestMissionPanelWidthTruncation(t *testing.T) {
	m, ctrl := testMissionModel(t)

	longTitle := strings.Repeat("W", 80)
	tasks := []mission.Task{
		{ID: "t_w", Title: longTitle, Kind: mission.TaskKindCode, Status: mission.TaskReady, Priority: 1, RiskLevel: mission.RiskLow},
	}
	missionID := seedMission(t, ctrl, mission.MissionRunning, tasks)
	m.activeMissionID = missionID

	width := 30
	lines := m.renderMissionPanelLines(10, width)
	for i, line := range lines {
		plain := stripANSI(line)
		if len([]rune(plain)) > width+4 { // allow small margin for icons
			t.Fatalf("line %d exceeds width %d: len=%d %q", i, width, len([]rune(plain)), plain)
		}
	}
}

func TestTaskStatusIconTextMapping(t *testing.T) {
	// Verify the non-styled taskStatusIcon function (used in /mission tasks).
	tests := []struct {
		status mission.TaskStatus
		want   string
	}{
		{mission.TaskDone, "✓"},
		{mission.TaskIntegrated, "✓"},
		{mission.TaskAccepted, "✓"},
		{mission.TaskRunning, "◐"},
		{mission.TaskLeased, "◐"},
		{mission.TaskBlocked, "✗"},
		{mission.TaskFailed, "✗"},
		{mission.TaskRejected, "✗"},
		{mission.TaskReady, "●"},
		{mission.TaskAwaitingReview, "◎"},
		{mission.TaskPending, "○"},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := taskStatusIcon(tt.status)
			if got != tt.want {
				t.Fatalf("taskStatusIcon(%s)=%q, want %q", tt.status, got, tt.want)
			}
		})
	}
}
