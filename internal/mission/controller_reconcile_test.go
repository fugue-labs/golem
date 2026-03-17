package mission

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestControllerReconcileMissionHealth_RepairsOrphanedRunningTask(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	ctrl := NewController(store)
	now := time.Now().UTC()

	mission := &Mission{ID: "m-reconcile", Title: "Repair stale lane", Goal: "repair", Status: MissionRunning, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateMission(ctx, mission); err != nil {
		t.Fatalf("CreateMission: %v", err)
	}
	task := &Task{ID: "t-orphan", MissionID: mission.ID, Title: "Reconnect worker lane", Kind: TaskKindCode, Status: TaskRunning, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	run := &Run{ID: "r-done", MissionID: mission.ID, TaskID: task.ID, Mode: RunModeWorker, Status: RunSucceeded}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	summary, err := ctrl.ReconcileMissionHealth(ctx, mission.ID)
	if err != nil {
		t.Fatalf("ReconcileMissionHealth: %v", err)
	}
	updatedTask, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if updatedTask.Status != TaskReady {
		t.Fatalf("task status after reconcile = %s, want %s", updatedTask.Status, TaskReady)
	}
	if summary.HealthStatus != MissionHealthRepairNeeded {
		t.Fatalf("health status = %q, want %q", summary.HealthStatus, MissionHealthRepairNeeded)
	}
	if summary.Recovery == nil || !summary.Recovery.RepairNeeded || !summary.Recovery.OrphanedRunning {
		t.Fatalf("unexpected recovery state: %+v", summary.Recovery)
	}
	if summary.NextAction != "Next ready task: Reconnect worker lane" {
		t.Fatalf("next action = %q", summary.NextAction)
	}
}

func TestControllerGetMissionSummary_DoesNotAutoReconcile(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	ctrl := NewController(store)
	now := time.Now().UTC()

	mission := &Mission{ID: "m-summary", Title: "Inspect stale lane", Goal: "inspect", Status: MissionRunning, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateMission(ctx, mission); err != nil {
		t.Fatalf("CreateMission: %v", err)
	}
	task := &Task{ID: "t-summary", MissionID: mission.ID, Title: "Reconnect worker lane", Kind: TaskKindCode, Status: TaskRunning, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	run := &Run{ID: "r-summary", MissionID: mission.ID, TaskID: task.ID, Mode: RunModeWorker, Status: RunSucceeded}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	summary, err := ctrl.GetMissionSummary(ctx, mission.ID)
	if err != nil {
		t.Fatalf("GetMissionSummary: %v", err)
	}
	if summary.HealthStatus != MissionHealthRepairNeeded {
		t.Fatalf("health status = %q, want %q", summary.HealthStatus, MissionHealthRepairNeeded)
	}
	if summary.Recovery != nil {
		t.Fatalf("unexpected recovery state without explicit reconciliation: %+v", summary.Recovery)
	}
	updatedTask, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if updatedTask.Status != TaskRunning {
		t.Fatalf("task status after GetMissionSummary = %s, want %s", updatedTask.Status, TaskRunning)
	}
	events, err := store.ListEvents(ctx, mission.ID, 0)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if got := countRecoveryCompletedEvents(events); got != 0 {
		t.Fatalf("recovery.completed count after GetMissionSummary = %d, want 0", got)
	}
}

func TestControllerReconcileMissionHealth_IdempotentRecoveryEvent(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	ctrl := NewController(store)
	now := time.Now().UTC()

	mission := &Mission{ID: "m-idem", Title: "Repair stale lane", Goal: "repair", Status: MissionRunning, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateMission(ctx, mission); err != nil {
		t.Fatalf("CreateMission: %v", err)
	}
	task := &Task{ID: "t-idem", MissionID: mission.ID, Title: "Reconnect worker lane", Kind: TaskKindCode, Status: TaskRunning, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	run := &Run{ID: "r-idem", MissionID: mission.ID, TaskID: task.ID, Mode: RunModeWorker, Status: RunSucceeded}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	if _, err := ctrl.ReconcileMissionHealth(ctx, mission.ID); err != nil {
		t.Fatalf("first ReconcileMissionHealth: %v", err)
	}
	firstEvents, err := store.ListEvents(ctx, mission.ID, 0)
	if err != nil {
		t.Fatalf("ListEvents after first reconcile: %v", err)
	}
	firstCount := countRecoveryCompletedEvents(firstEvents)
	if firstCount != 1 {
		t.Fatalf("recovery.completed count after first reconcile = %d, want 1", firstCount)
	}
	if _, err := ctrl.ReconcileMissionHealth(ctx, mission.ID); err != nil {
		t.Fatalf("second ReconcileMissionHealth: %v", err)
	}
	secondEvents, err := store.ListEvents(ctx, mission.ID, 0)
	if err != nil {
		t.Fatalf("ListEvents after second reconcile: %v", err)
	}
	if got := countRecoveryCompletedEvents(secondEvents); got != 1 {
		t.Fatalf("recovery.completed count after second reconcile = %d, want 1", got)
	}
}

func TestControllerReconcileMissionHealth_PropagatesPartialRepairFailure(t *testing.T) {
	ctx := context.Background()
	store := newRecoveryMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}
	store.tasks["t1"] = &Task{ID: "t1", MissionID: "m1", Status: TaskRunning}
	store.runsList = []*Run{{ID: "r1", MissionID: "m1", TaskID: "t1", Status: RunSucceeded}}
	store.updateTaskErr = context.DeadlineExceeded
	ctrl := NewController(store)

	_, err := ctrl.ReconcileMissionHealth(ctx, "m1")
	if err == nil {
		t.Fatal("expected reconcile error")
	}
	if !strings.Contains(err.Error(), "recover: stuck tasks") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestControllerReconcileMissionHealth_IgnoresOnlyTerminalMissionRecoveryError(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	ctrl := NewController(store)
	now := time.Now().UTC()
	mission := &Mission{ID: "m-terminal", Title: "Done", Goal: "done", Status: MissionCompleted, CreatedAt: now, UpdatedAt: now, EndedAt: &now}
	if err := store.CreateMission(ctx, mission); err != nil {
		t.Fatalf("CreateMission: %v", err)
	}

	summary, err := ctrl.ReconcileMissionHealth(ctx, mission.ID)
	if err != nil {
		t.Fatalf("ReconcileMissionHealth on terminal mission: %v", err)
	}
	if summary.Mission == nil || summary.Mission.Status != MissionCompleted {
		t.Fatalf("summary mission status = %#v", summary.Mission)
	}
	if summary.HealthStatus != MissionHealthHealthy {
		t.Fatalf("health status = %q, want %q", summary.HealthStatus, MissionHealthHealthy)
	}
	if summary.Recovery != nil {
		t.Fatalf("unexpected recovery state on terminal mission: %+v", summary.Recovery)
	}
}

func TestControllerReconcileMissionHealth_ReportsPausedSummaryState(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	ctrl := NewController(store)
	now := time.Now().UTC()
	mission := &Mission{ID: "m-paused", Title: "Paused", Goal: "pause", Status: MissionPaused, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateMission(ctx, mission); err != nil {
		t.Fatalf("CreateMission: %v", err)
	}
	task := &Task{ID: "t-paused", MissionID: mission.ID, Title: "Resume lane", Kind: TaskKindCode, Status: TaskReady, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	summary, err := ctrl.ReconcileMissionHealth(ctx, mission.ID)
	if err != nil {
		t.Fatalf("ReconcileMissionHealth: %v", err)
	}
	if summary.HealthStatus != MissionHealthPaused {
		t.Fatalf("health status = %q, want %q", summary.HealthStatus, MissionHealthPaused)
	}
	if summary.Recovery != nil {
		if !summary.Recovery.Paused || summary.Recovery.Blocked || summary.Recovery.RepairNeededFlow || summary.Recovery.HealthyRunning {
			t.Fatalf("unexpected paused recovery flow state: %+v", summary.Recovery)
		}
	}
}

func countRecoveryCompletedEvents(events []*Event) int {
	count := 0
	for _, event := range events {
		if event != nil && event.Type == "recovery.completed" {
			count++
		}
	}
	return count
}
