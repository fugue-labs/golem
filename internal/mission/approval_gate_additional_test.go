package mission

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBuildMissionSummaryAwaitingApprovalApprovedPlanShowsReadyToStart(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	now := time.Now().UTC()

	ms := &Mission{
		ID:        "m_summary_ready",
		Title:     "Approved plan mission",
		Goal:      "Verify approved-but-not-started summary",
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
	if err := store.CreateApproval(ctx, &Approval{ID: "ap_plan_ready", MissionID: ms.ID, Kind: missionPlanApprovalKind, Status: ApprovalApproved, CreatedAt: now, ResolvedAt: &now}); err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}

	summary, err := BuildMissionSummary(ctx, store, ms.ID)
	if err != nil {
		t.Fatalf("BuildMissionSummary: %v", err)
	}
	if summary.HasApprovalGate() {
		t.Fatal("expected approved plan to clear approval gate")
	}
	if summary.PendingApprovals != 0 {
		t.Fatalf("pending approvals = %d, want 0", summary.PendingApprovals)
	}
	if summary.PlanApprovalStatus != ApprovalApproved {
		t.Fatalf("plan approval status = %s, want %s", summary.PlanApprovalStatus, ApprovalApproved)
	}
	if summary.PhaseLabel != "Ready to start" {
		t.Fatalf("phase label = %q, want %q", summary.PhaseLabel, "Ready to start")
	}
	if summary.NextAction != "Plan approved. Start mission execution with /mission start" {
		t.Fatalf("next action = %q", summary.NextAction)
	}
	if summary.Attention != "Plan approved and ready to start" {
		t.Fatalf("attention = %q", summary.Attention)
	}
}

func TestStartMissionAllowsApprovedAwaitingApprovalRetry(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	ctrl := NewController(store)

	created, err := ctrl.CreateMission(ctx, CreateMissionRequest{
		Title:    "Retry start",
		Goal:     "Allow retry after approval",
		RepoRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("CreateMission: %v", err)
	}
	plan := &PlanResult{
		Summary: "single-task plan",
		Tasks: []PlanTask{{
			ID:        "t_retry",
			Title:     "Retry task",
			Kind:      TaskKindCode,
			Objective: "Retry after approval",
			Priority:  1,
			RiskLevel: RiskLow,
		}},
	}
	if err := ctrl.ApplyPlan(ctx, created.ID, plan); err != nil {
		t.Fatalf("ApplyPlan: %v", err)
	}
	if err := ctrl.ApproveMission(ctx, created.ID); err != nil {
		t.Fatalf("ApproveMission: %v", err)
	}

	if err := ctrl.StartMission(ctx, created.ID); err != nil {
		t.Fatalf("StartMission: %v", err)
	}
	started, err := ctrl.GetMission(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if started.Status != MissionRunning {
		t.Fatalf("mission status = %s, want %s", started.Status, MissionRunning)
	}
}

func TestStartMissionRejectsAwaitingApprovalWithoutDurableGate(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	ctrl := NewController(store)
	now := time.Now().UTC()

	ms := &Mission{
		ID:        "m_missing_gate",
		Title:     "Missing gate",
		Goal:      "Require durable gate",
		Status:    MissionAwaitingApproval,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateMission(ctx, ms); err != nil {
		t.Fatalf("CreateMission: %v", err)
	}

	err := ctrl.StartMission(ctx, ms.ID)
	if err == nil {
		t.Fatal("StartMission unexpectedly succeeded without durable gate")
	}
	if !strings.Contains(err.Error(), "durable mission-plan gate") {
		t.Fatalf("StartMission error = %v, want durable gate guidance", err)
	}

	after, getErr := ctrl.GetMission(ctx, ms.ID)
	if getErr != nil {
		t.Fatalf("GetMission: %v", getErr)
	}
	if after.Status != MissionAwaitingApproval {
		t.Fatalf("mission status after failed start = %s, want %s", after.Status, MissionAwaitingApproval)
	}
}
