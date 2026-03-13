package mission

import (
	"context"
	"strings"
	"testing"
	"time"
)

// controllerMockStore extends recoveryMockStore for Controller tests.
// It adds CreateMission (which panics in the base mockStore).
type controllerMockStore struct {
	recoveryMockStore
}

func newControllerMockStore() *controllerMockStore {
	return &controllerMockStore{
		recoveryMockStore: *newRecoveryMockStore(),
	}
}

func (s *controllerMockStore) CreateMission(_ context.Context, m *Mission) error {
	s.missions[m.ID] = m
	return nil
}

// --- Tests ---

func TestCreateMission(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)

	m, err := ctrl.CreateMission(context.Background(), CreateMissionRequest{
		Title:      "Test Mission",
		Goal:       "Test the system",
		RepoRoot:   "/tmp/repo",
		BaseCommit: "abc123",
		BaseBranch: "main",
		Budget:     Budget{MaxConcurrentWorkers: 3},
	})
	if err != nil {
		t.Fatal(err)
	}

	if m.Status != MissionDraft {
		t.Errorf("expected draft status, got %s", m.Status)
	}
	if m.Title != "Test Mission" {
		t.Errorf("expected title 'Test Mission', got %s", m.Title)
	}
	if m.Goal != "Test the system" {
		t.Errorf("expected goal, got %s", m.Goal)
	}
	if m.ID == "" {
		t.Error("expected non-empty ID")
	}
	if m.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
	if m.Budget.MaxConcurrentWorkers != 3 {
		t.Errorf("expected MaxConcurrentWorkers=3, got %d", m.Budget.MaxConcurrentWorkers)
	}
}

func TestApplyPlan(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)

	m, err := ctrl.CreateMission(context.Background(), CreateMissionRequest{
		Title:    "Plan Test",
		Goal:     "Test plan application",
		RepoRoot: "/tmp/repo",
	})
	if err != nil {
		t.Fatal(err)
	}

	plan := &PlanResult{
		Summary:         "Implementation plan",
		SuccessCriteria: []string{"Tests pass", "No regressions"},
		Tasks: []PlanTask{
			{ID: "t1", Title: "Task 1", Kind: TaskKindCode, Objective: "Do thing 1", Priority: 1, RiskLevel: RiskLow},
			{ID: "t2", Title: "Task 2", Kind: TaskKindTest, Objective: "Test thing", Priority: 2, RiskLevel: RiskLow},
		},
		Dependencies: []TaskDependency{
			{TaskID: "t2", DependsOnID: "t1"},
		},
	}

	err = ctrl.ApplyPlan(context.Background(), m.ID, plan)
	if err != nil {
		t.Fatal(err)
	}

	// Mission should now be awaiting_approval.
	got, _ := ctrl.GetMission(context.Background(), m.ID)
	if got.Status != MissionAwaitingApproval {
		t.Errorf("expected awaiting_approval, got %s", got.Status)
	}
	if len(got.SuccessCriteria) != 2 {
		t.Errorf("expected 2 success criteria, got %d", len(got.SuccessCriteria))
	}

	// t1 should be ready (no deps), t2 should be pending (dep on t1).
	t1 := store.tasks["t1"]
	if t1 == nil {
		t.Fatal("task t1 not found")
	}
	if t1.Status != TaskReady {
		t.Errorf("expected t1 ready (no deps), got %s", t1.Status)
	}

	t2 := store.tasks["t2"]
	if t2 == nil {
		t.Fatal("task t2 not found")
	}
	if t2.Status != TaskPending {
		t.Errorf("expected t2 pending (dep on t1), got %s", t2.Status)
	}
}

func TestApplyPlan_NoDeps(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)

	m, err := ctrl.CreateMission(context.Background(), CreateMissionRequest{
		Title: "No deps", Goal: "All tasks independent", RepoRoot: "/tmp",
	})
	if err != nil {
		t.Fatal(err)
	}

	plan := &PlanResult{
		Tasks: []PlanTask{
			{ID: "t1", Title: "Task A", Kind: TaskKindCode, Objective: "A", Priority: 1, RiskLevel: RiskLow},
			{ID: "t2", Title: "Task B", Kind: TaskKindCode, Objective: "B", Priority: 2, RiskLevel: RiskLow},
		},
	}

	if err := ctrl.ApplyPlan(context.Background(), m.ID, plan); err != nil {
		t.Fatal(err)
	}

	// Both tasks should be ready (no dependencies).
	if store.tasks["t1"].Status != TaskReady {
		t.Errorf("expected t1 ready, got %s", store.tasks["t1"].Status)
	}
	if store.tasks["t2"].Status != TaskReady {
		t.Errorf("expected t2 ready, got %s", store.tasks["t2"].Status)
	}
}

func TestApplyPlan_WrongState(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)

	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	err := ctrl.ApplyPlan(context.Background(), "m1", &PlanResult{})
	if err == nil {
		t.Fatal("expected error applying plan to running mission")
	}
	if !strings.Contains(err.Error(), "running") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyPlan_PlanningStateAllowed(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)

	store.missions["m1"] = &Mission{ID: "m1", Status: MissionPlanning}

	plan := &PlanResult{
		Tasks: []PlanTask{
			{ID: "t1", Title: "Task 1", Kind: TaskKindCode, Objective: "Do it", Priority: 1, RiskLevel: RiskLow},
		},
	}

	err := ctrl.ApplyPlan(context.Background(), "m1", plan)
	if err != nil {
		t.Fatalf("should allow applying plan to planning mission: %v", err)
	}
	if store.missions["m1"].Status != MissionAwaitingApproval {
		t.Errorf("expected awaiting_approval, got %s", store.missions["m1"].Status)
	}
}

func TestStartMission(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)

	store.missions["m1"] = &Mission{ID: "m1", Status: MissionAwaitingApproval}

	err := ctrl.StartMission(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	if store.missions["m1"].Status != MissionRunning {
		t.Errorf("expected running, got %s", store.missions["m1"].Status)
	}
	if store.missions["m1"].StartedAt == nil {
		t.Error("expected StartedAt to be set")
	}
}

func TestStartMission_FromPaused(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)

	originalStart := time.Now().UTC().Add(-1 * time.Hour)
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionPaused, StartedAt: &originalStart}

	err := ctrl.StartMission(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	if store.missions["m1"].Status != MissionRunning {
		t.Errorf("expected running, got %s", store.missions["m1"].Status)
	}
	// StartedAt should be preserved from original start, not overwritten.
	if store.missions["m1"].StartedAt == nil {
		t.Fatal("StartedAt should not be nil")
	}
}

func TestStartMission_WrongState(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)

	store.missions["m1"] = &Mission{ID: "m1", Status: MissionDraft}

	err := ctrl.StartMission(context.Background(), "m1")
	if err == nil {
		t.Fatal("expected error starting draft mission")
	}
	if !strings.Contains(err.Error(), "awaiting_approval or paused") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPauseMission(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)

	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	err := ctrl.PauseMission(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	if store.missions["m1"].Status != MissionPaused {
		t.Errorf("expected paused, got %s", store.missions["m1"].Status)
	}
}

func TestPauseMission_WrongState(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)

	store.missions["m1"] = &Mission{ID: "m1", Status: MissionDraft}

	err := ctrl.PauseMission(context.Background(), "m1")
	if err == nil {
		t.Fatal("expected error pausing non-running mission")
	}
}

func TestCancelMission_Running(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)

	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	err := ctrl.CancelMission(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	if store.missions["m1"].Status != MissionCancelled {
		t.Errorf("expected cancelled, got %s", store.missions["m1"].Status)
	}
	if store.missions["m1"].EndedAt == nil {
		t.Error("expected EndedAt to be set")
	}
}

func TestCancelMission_Draft(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)

	store.missions["m1"] = &Mission{ID: "m1", Status: MissionDraft}

	err := ctrl.CancelMission(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	if store.missions["m1"].Status != MissionCancelled {
		t.Errorf("expected cancelled, got %s", store.missions["m1"].Status)
	}
}

func TestCancelMission_AlreadyTerminal(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)

	store.missions["m1"] = &Mission{ID: "m1", Status: MissionCompleted}

	err := ctrl.CancelMission(context.Background(), "m1")
	if err == nil {
		t.Fatal("expected error cancelling completed mission")
	}
	if !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCannotStartDraftMission(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)
	ctx := context.Background()

	m, err := ctrl.CreateMission(ctx, CreateMissionRequest{
		Title: "Draft test", Goal: "Test", RepoRoot: "/tmp",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Try to start without applying a plan — should fail because status is draft.
	err = ctrl.StartMission(ctx, m.ID)
	if err == nil {
		t.Fatal("expected error starting draft mission without plan")
	}
}

func TestFullMissionLifecycle(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)
	ctx := context.Background()

	// 1. Create
	m, err := ctrl.CreateMission(ctx, CreateMissionRequest{
		Title: "Lifecycle Test", Goal: "Full lifecycle", RepoRoot: "/tmp/repo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != MissionDraft {
		t.Fatalf("expected draft, got %s", m.Status)
	}

	// 2. Apply plan
	plan := &PlanResult{
		Tasks: []PlanTask{
			{ID: "t1", Title: "Work", Kind: TaskKindCode, Objective: "Do it", Priority: 1, RiskLevel: RiskLow},
		},
	}
	if err := ctrl.ApplyPlan(ctx, m.ID, plan); err != nil {
		t.Fatal(err)
	}
	m, _ = ctrl.GetMission(ctx, m.ID)
	if m.Status != MissionAwaitingApproval {
		t.Fatalf("expected awaiting_approval, got %s", m.Status)
	}

	// 3. Start (approve)
	if err := ctrl.StartMission(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	m, _ = ctrl.GetMission(ctx, m.ID)
	if m.Status != MissionRunning {
		t.Fatalf("expected running, got %s", m.Status)
	}
	if m.StartedAt == nil {
		t.Fatal("expected StartedAt set on first start")
	}

	// 4. Pause
	if err := ctrl.PauseMission(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	m, _ = ctrl.GetMission(ctx, m.ID)
	if m.Status != MissionPaused {
		t.Fatalf("expected paused, got %s", m.Status)
	}

	// 5. Resume (start from paused)
	if err := ctrl.StartMission(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	m, _ = ctrl.GetMission(ctx, m.ID)
	if m.Status != MissionRunning {
		t.Fatalf("expected running after resume, got %s", m.Status)
	}

	// 6. Cancel
	if err := ctrl.CancelMission(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	m, _ = ctrl.GetMission(ctx, m.ID)
	if m.Status != MissionCancelled {
		t.Fatalf("expected cancelled, got %s", m.Status)
	}
	if m.EndedAt == nil {
		t.Fatal("expected EndedAt set on cancellation")
	}
}

func TestCancelMission_Paused(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)

	store.missions["m1"] = &Mission{ID: "m1", Status: MissionPaused}

	if err := ctrl.CancelMission(context.Background(), "m1"); err != nil {
		t.Fatal(err)
	}
	if store.missions["m1"].Status != MissionCancelled {
		t.Errorf("expected cancelled, got %s", store.missions["m1"].Status)
	}
}

func TestCancelMission_Failed(t *testing.T) {
	store := newControllerMockStore()
	ctrl := NewController(store)

	store.missions["m1"] = &Mission{ID: "m1", Status: MissionFailed}

	err := ctrl.CancelMission(context.Background(), "m1")
	if err == nil {
		t.Fatal("expected error cancelling failed (terminal) mission")
	}
}
