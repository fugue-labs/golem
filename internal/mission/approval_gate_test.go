package mission

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStartMissionRejectsPendingMissionPlanApproval(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	ctrl := NewController(store)

	created, err := ctrl.CreateMission(ctx, CreateMissionRequest{
		Title:    "Approval gate",
		Goal:     "Require explicit approval before start",
		RepoRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("CreateMission: %v", err)
	}

	plan := &PlanResult{
		Summary: "single-task plan",
		Tasks: []PlanTask{{
			ID:        "t_gate",
			Title:     "Implement approval gate",
			Kind:      TaskKindCode,
			Objective: "Keep start blocked until approval resolves",
			Priority:  1,
			RiskLevel: RiskLow,
		}},
	}
	if err := ctrl.ApplyPlan(ctx, created.ID, plan); err != nil {
		t.Fatalf("ApplyPlan: %v", err)
	}

	before, err := ctrl.GetMission(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetMission before start: %v", err)
	}
	if before.Status != MissionAwaitingApproval {
		t.Fatalf("mission status before start = %s, want %s", before.Status, MissionAwaitingApproval)
	}

	approvals, err := ctrl.Store().ListApprovals(ctx, created.ID)
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	if len(approvals) != 1 {
		t.Fatalf("approvals = %d, want 1", len(approvals))
	}
	if approvals[0].Status != ApprovalPending {
		t.Fatalf("approval status before start = %s, want %s", approvals[0].Status, ApprovalPending)
	}

	startErr := ctrl.StartMission(ctx, created.ID)
	if startErr == nil {
		t.Fatal("StartMission unexpectedly succeeded with pending approval")
	}
	if !strings.Contains(startErr.Error(), "mission plan approval is pending") {
		t.Fatalf("StartMission error = %v, want pending approval message", startErr)
	}

	after, err := ctrl.GetMission(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetMission after start: %v", err)
	}
	if after.Status != MissionAwaitingApproval {
		t.Fatalf("mission status after failed start = %s, want %s", after.Status, MissionAwaitingApproval)
	}
	if after.StartedAt != nil {
		t.Fatalf("mission started_at set after failed start: %v", after.StartedAt)
	}

	approvals, err = ctrl.Store().ListApprovals(ctx, created.ID)
	if err != nil {
		t.Fatalf("ListApprovals after start: %v", err)
	}
	if len(approvals) != 1 {
		t.Fatalf("approvals after start = %d, want 1", len(approvals))
	}
	if approvals[0].Status != ApprovalPending {
		t.Fatalf("approval status after failed start = %s, want %s", approvals[0].Status, ApprovalPending)
	}
	if approvals[0].ResolvedAt != nil {
		t.Fatalf("approval resolved_at set after failed start: %v", approvals[0].ResolvedAt)
	}

	events, err := ctrl.Store().ListEvents(ctx, created.ID, 20)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	for _, event := range events {
		if event == nil {
			continue
		}
		if event.Type == "mission.started" || event.Type == "mission.approved" {
			t.Fatalf("unexpected lifecycle event after failed start: %s", event.Type)
		}
	}
}

func TestBuildMissionSummaryAwaitingApprovalShowsApprovalGateAndReadyWork(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	now := time.Now().UTC()

	ms := &Mission{
		ID:        "m_summary_gate",
		Title:     "Awaiting approval mission",
		Goal:      "Verify summary fields",
		Status:    MissionAwaitingApproval,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateMission(ctx, ms); err != nil {
		t.Fatalf("CreateMission: %v", err)
	}
	if err := store.CreateTask(ctx, &Task{ID: "t_ready", MissionID: ms.ID, Title: "Ready task", Kind: TaskKindCode, Status: TaskReady, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateTask ready: %v", err)
	}
	if err := store.CreateTask(ctx, &Task{ID: "t_wait", MissionID: ms.ID, Title: "Blocked by dependency", Kind: TaskKindTest, Status: TaskPending, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateTask pending: %v", err)
	}
	if err := store.AddDependency(ctx, TaskDependency{TaskID: "t_wait", DependsOnID: "t_ready"}); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	if err := store.CreateApproval(ctx, &Approval{ID: "ap_plan", MissionID: ms.ID, Kind: missionPlanApprovalKind, Status: ApprovalPending, CreatedAt: now}); err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}

	summary, err := BuildMissionSummary(ctx, store, ms.ID)
	if err != nil {
		t.Fatalf("BuildMissionSummary: %v", err)
	}
	if !summary.HasApprovalGate() {
		t.Fatal("expected summary to report mission approval gate")
	}
	if summary.PendingApprovals != 1 {
		t.Fatalf("pending approvals = %d, want 1", summary.PendingApprovals)
	}
	if summary.TaskCounts.Ready != 1 || summary.TaskCounts.Pending != 1 {
		t.Fatalf("unexpected task counts: ready=%d pending=%d", summary.TaskCounts.Ready, summary.TaskCounts.Pending)
	}
	if summary.DependencyEdges != 1 {
		t.Fatalf("dependency edges = %d, want 1", summary.DependencyEdges)
	}
	if summary.PhaseLabel != "Awaiting approval" {
		t.Fatalf("phase label = %q, want %q", summary.PhaseLabel, "Awaiting approval")
	}
	if summary.NextAction != "Review the proposed mission plan and approve start with /mission approve" {
		t.Fatalf("next action = %q", summary.NextAction)
	}
	if summary.Attention != "Mission plan is waiting for approval" {
		t.Fatalf("attention = %q", summary.Attention)
	}
	if summary.FocusTask == nil || summary.FocusTask.Title != "Ready task" {
		t.Fatalf("focus task = %#v, want ready task", summary.FocusTask)
	}
	if summary.NextTask == nil || summary.NextTask.Title != "Blocked by dependency" {
		t.Fatalf("next task = %#v, want pending dependent task", summary.NextTask)
	}
	if len(summary.PendingApprovalItems) != 1 || summary.PendingApprovalItems[0].Title != "Mission plan approval" {
		t.Fatalf("pending approval items = %#v", summary.PendingApprovalItems)
	}
}
