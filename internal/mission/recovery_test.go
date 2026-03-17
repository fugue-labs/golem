package mission

import (
	"context"
	"strings"
	"testing"
	"time"
)

// recoveryMockStore extends integratorMockStore with the additional methods
// needed by MissionRecoveryManager (event listing for replan counting).
type recoveryMockStore struct {
	integratorMockStore
	createdTasks []*Task // tracks order of task creation
}

func newRecoveryMockStore() *recoveryMockStore {
	return &recoveryMockStore{
		integratorMockStore: integratorMockStore{
			reviewMockStore: reviewMockStore{
				workerMockStore: workerMockStore{
					mockStore: mockStore{
						missions: make(map[string]*Mission),
					},
					tasks: make(map[string]*Task),
					runs:  make(map[string]*Run),
				},
				tasksByStatus: make(map[TaskStatus][]*Task),
				runsByTask:    make(map[string][]*Run),
			},
		},
	}
}

func (s *recoveryMockStore) ListEvents(_ context.Context, _ string, _ int) ([]*Event, error) {
	return s.events, nil
}

func (s *recoveryMockStore) CreateTask(_ context.Context, t *Task) error {
	s.tasks[t.ID] = t
	s.createdTasks = append(s.createdTasks, t)
	return nil
}

func (s *recoveryMockStore) AddDependency(_ context.Context, dep TaskDependency) error {
	s.deps = append(s.deps, dep)
	return nil
}

// --- Tests ---

func TestRecoverMission_StaleWorkers(t *testing.T) {
	store := newRecoveryMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	past := time.Now().UTC().Add(-1 * time.Hour)

	// Stale run — lease expired.
	task1 := &Task{ID: "t1", MissionID: "m1", Status: TaskRunning}
	store.tasks["t1"] = task1
	staleRun := &Run{
		ID: "r1", MissionID: "m1", TaskID: "t1",
		Status: RunRunning, LeaseExpires: &past,
	}
	store.runs["r1"] = staleRun
	store.runsList = []*Run{staleRun}

	wt := NewWorktreeManager("/tmp/test-repo")
	sched := NewScheduler(store)
	workers := NewWorkerLauncher(sched, wt, store)
	rm := NewMissionRecoveryManager(store, wt, workers)

	report, err := rm.RecoverMission(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	if report.StaleRecovered != 1 {
		t.Errorf("expected 1 stale recovered, got %d", report.StaleRecovered)
	}
	if staleRun.Status != RunLeaseLost {
		t.Errorf("expected stale run status lease_lost, got %s", staleRun.Status)
	}
	if task1.Status != TaskReady {
		t.Errorf("expected stale task back to ready, got %s", task1.Status)
	}
}

func TestRecoverMission_StuckTasks(t *testing.T) {
	store := newRecoveryMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	// Task in running state but no active run exists for it.
	stuckTask := &Task{ID: "t1", MissionID: "m1", Status: TaskRunning}
	store.tasks["t1"] = stuckTask

	// A run exists for this task, but it's already completed.
	completedRun := &Run{
		ID: "r1", MissionID: "m1", TaskID: "t1",
		Status: RunSucceeded,
	}
	store.runs["r1"] = completedRun
	store.runsList = []*Run{completedRun}

	wt := NewWorktreeManager("/tmp/test-repo")
	sched := NewScheduler(store)
	workers := NewWorkerLauncher(sched, wt, store)
	rm := NewMissionRecoveryManager(store, wt, workers)

	report, err := rm.RecoverMission(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	if report.StuckReset != 1 {
		t.Errorf("expected 1 stuck reset, got %d", report.StuckReset)
	}
	if stuckTask.Status != TaskReady {
		t.Errorf("expected stuck task back to ready, got %s", stuckTask.Status)
	}

	// Verify event was logged.
	found := false
	for _, e := range store.events {
		if e.Type == "recovery.stuck_task_reset" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected recovery.stuck_task_reset event")
	}
}

func TestRecoverMission_NewlyReady(t *testing.T) {
	store := newRecoveryMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	// t1 is done (completed while offline).
	doneTask := &Task{ID: "t1", MissionID: "m1", Status: TaskDone}
	store.tasks["t1"] = doneTask

	// t2 is pending and depends on t1 — should be promoted to ready.
	pendingTask := &Task{ID: "t2", MissionID: "m1", Status: TaskPending}
	store.tasks["t2"] = pendingTask

	store.deps = []TaskDependency{
		{TaskID: "t2", DependsOnID: "t1"},
	}

	// No runs.
	store.runsList = nil

	wt := NewWorktreeManager("/tmp/test-repo")
	sched := NewScheduler(store)
	workers := NewWorkerLauncher(sched, wt, store)
	rm := NewMissionRecoveryManager(store, wt, workers)

	report, err := rm.RecoverMission(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	if report.NewlyReady != 1 {
		t.Errorf("expected 1 newly ready, got %d", report.NewlyReady)
	}
	if pendingTask.Status != TaskReady {
		t.Errorf("expected pending task promoted to ready, got %s", pendingTask.Status)
	}
}

func TestRecoverMission_TerminalMission(t *testing.T) {
	store := newRecoveryMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionCompleted}

	wt := NewWorktreeManager("/tmp/test-repo")
	sched := NewScheduler(store)
	workers := NewWorkerLauncher(sched, wt, store)
	rm := NewMissionRecoveryManager(store, wt, workers)

	_, err := rm.RecoverMission(context.Background(), "m1")
	if err == nil {
		t.Fatal("expected error for terminal mission")
	}
	if !strings.Contains(err.Error(), "terminal state") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestReplan_BudgetExceeded(t *testing.T) {
	store := newRecoveryMockStore()
	store.missions["m1"] = &Mission{
		ID:     "m1",
		Status: MissionRunning,
		Budget: Budget{MaxReplans: 2},
	}

	// Simulate 2 existing replans via events.
	store.events = []*Event{
		{MissionID: "m1", Type: "replan.applied"},
		{MissionID: "m1", Type: "replan.applied"},
	}

	rm := NewMissionRecoveryManager(store, nil, nil)

	_, err := rm.RequestReplan(context.Background(), "m1", []string{"t1"}, "task keeps failing")
	if err == nil {
		t.Fatal("expected error when replan budget exceeded")
	}
	if !strings.Contains(err.Error(), "budget exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestReplan_Success(t *testing.T) {
	store := newRecoveryMockStore()
	store.missions["m1"] = &Mission{
		ID:     "m1",
		Status: MissionRunning,
		Budget: Budget{MaxReplans: 3},
	}

	// 1 existing replan.
	store.events = []*Event{
		{MissionID: "m1", Type: "replan.applied"},
	}

	rm := NewMissionRecoveryManager(store, nil, nil)

	req, err := rm.RequestReplan(context.Background(), "m1", []string{"t1", "t2"}, "persistent failures")
	if err != nil {
		t.Fatal(err)
	}

	if req.MissionID != "m1" {
		t.Errorf("expected mission ID m1, got %s", req.MissionID)
	}
	if len(req.AffectedTaskIDs) != 2 {
		t.Errorf("expected 2 affected tasks, got %d", len(req.AffectedTaskIDs))
	}
	if req.Reason != "persistent failures" {
		t.Errorf("unexpected reason: %s", req.Reason)
	}
	if req.ID == "" {
		t.Error("expected non-empty request ID")
	}

	// Verify replan.requested event was logged.
	found := false
	for _, e := range store.events {
		if e.Type == "replan.requested" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected replan.requested event")
	}
}

func TestRequestReplan_UnlimitedBudget(t *testing.T) {
	store := newRecoveryMockStore()
	store.missions["m1"] = &Mission{
		ID:     "m1",
		Status: MissionRunning,
		Budget: Budget{MaxReplans: 0}, // 0 means unlimited
	}

	// Many replans already done.
	store.events = []*Event{
		{MissionID: "m1", Type: "replan.applied"},
		{MissionID: "m1", Type: "replan.applied"},
		{MissionID: "m1", Type: "replan.applied"},
		{MissionID: "m1", Type: "replan.applied"},
		{MissionID: "m1", Type: "replan.applied"},
	}

	rm := NewMissionRecoveryManager(store, nil, nil)

	req, err := rm.RequestReplan(context.Background(), "m1", []string{"t1"}, "try again")
	if err != nil {
		t.Fatalf("expected no error with unlimited budget, got: %v", err)
	}
	if req == nil {
		t.Fatal("expected non-nil request")
	}
}

func TestResolveNewlyReady_AcceptedDependencyDoesNotPromotePendingTask(t *testing.T) {
	store := newRecoveryMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}
	store.tasks["t1"] = &Task{ID: "t1", MissionID: "m1", Status: TaskAccepted}
	store.tasks["t2"] = &Task{ID: "t2", MissionID: "m1", Status: TaskPending}
	store.deps = []TaskDependency{{TaskID: "t2", DependsOnID: "t1"}}

	rm := NewMissionRecoveryManager(store, nil, nil)
	promoted, err := rm.resolveNewlyReady(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if promoted != 0 {
		t.Fatalf("expected accepted dependency to keep task pending, promoted=%d", promoted)
	}
	if got := store.tasks["t2"].Status; got != TaskPending {
		t.Fatalf("task status = %s, want %s", got, TaskPending)
	}
}

func TestApplyReplan_AddsTasks(t *testing.T) {
	store := newRecoveryMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	// Existing done task.
	store.tasks["t1"] = &Task{ID: "t1", MissionID: "m1", Status: TaskDone}

	plan := &PlanResult{
		Summary: "Revised approach",
		Tasks: []PlanTask{
			{
				ID:        "t3",
				Title:     "New replacement task",
				Kind:      TaskKindCode,
				Objective: "Implement the feature differently",
				Priority:  1,
				RiskLevel: RiskLow,
			},
			{
				ID:        "t4",
				Title:     "Test the new approach",
				Kind:      TaskKindTest,
				Objective: "Add integration tests",
				Priority:  2,
				RiskLevel: RiskLow,
			},
		},
		Dependencies: []TaskDependency{
			{TaskID: "t4", DependsOnID: "t3"},
		},
	}

	rm := NewMissionRecoveryManager(store, nil, nil)

	err := rm.ApplyReplan(context.Background(), "m1", plan)
	if err != nil {
		t.Fatal(err)
	}

	// Verify tasks were created.
	if _, ok := store.tasks["t3"]; !ok {
		t.Error("expected task t3 to be created")
	}
	if _, ok := store.tasks["t4"]; !ok {
		t.Error("expected task t4 to be created")
	}

	// Verify t3 got promoted to ready (no unmet deps).
	if store.tasks["t3"].Status != TaskReady {
		t.Errorf("expected t3 to be ready (no deps), got %s", store.tasks["t3"].Status)
	}
	// t4 depends on t3 which is not done, so should stay pending.
	if store.tasks["t4"].Status != TaskPending {
		t.Errorf("expected t4 to be pending (dep on t3), got %s", store.tasks["t4"].Status)
	}

	// Verify dependencies were added.
	if len(store.deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(store.deps))
	}
	if store.deps[0].TaskID != "t4" || store.deps[0].DependsOnID != "t3" {
		t.Errorf("unexpected dependency: %+v", store.deps[0])
	}

	// Verify mission's LastReplanAt was set.
	if store.missions["m1"].LastReplanAt == nil {
		t.Error("expected LastReplanAt to be set")
	}

	// Verify replan.applied event was logged.
	found := false
	for _, e := range store.events {
		if e.Type == "replan.applied" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected replan.applied event")
	}
}

func TestResolveBlockedTask(t *testing.T) {
	store := newRecoveryMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	blockedTask := &Task{
		ID:             "t1",
		MissionID:      "m1",
		Status:         TaskBlocked,
		BlockingReason: "merge conflict in pkg/api/handler.go",
	}
	store.tasks["t1"] = blockedTask

	rm := NewMissionRecoveryManager(store, nil, nil)

	err := rm.ResolveBlockedTask(context.Background(), "m1", "t1", "conflict resolved manually")
	if err != nil {
		t.Fatal(err)
	}

	if blockedTask.Status != TaskReady {
		t.Errorf("expected task status ready, got %s", blockedTask.Status)
	}
	if blockedTask.BlockingReason != "" {
		t.Errorf("expected empty blocking reason, got %s", blockedTask.BlockingReason)
	}

	// Verify event was logged.
	found := false
	for _, e := range store.events {
		if e.Type == "task.unblocked" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected task.unblocked event")
	}
}

func TestResolveBlockedTask_NotBlocked(t *testing.T) {
	store := newRecoveryMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	runningTask := &Task{
		ID:        "t1",
		MissionID: "m1",
		Status:    TaskRunning,
	}
	store.tasks["t1"] = runningTask

	rm := NewMissionRecoveryManager(store, nil, nil)

	err := rm.ResolveBlockedTask(context.Background(), "m1", "t1", "tried to unblock")
	if err == nil {
		t.Fatal("expected error for non-blocked task")
	}
	if !strings.Contains(err.Error(), "not blocked") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveBlockedTask_NotFound(t *testing.T) {
	store := newRecoveryMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	rm := NewMissionRecoveryManager(store, nil, nil)

	err := rm.ResolveBlockedTask(context.Background(), "m1", "nonexistent", "resolve")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecoverMission_StaleRunningMissionOffersReadyQueue(t *testing.T) {
	store := newRecoveryMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning, Goal: "Repair stale worker execution"}

	stuckTask := &Task{
		ID:        "t_repair",
		MissionID: "m1",
		Title:     "Repair stale worker lease",
		Status:    TaskRunning,
	}
	store.tasks[stuckTask.ID] = stuckTask

	completedRun := &Run{
		ID:        "r_done",
		MissionID: "m1",
		TaskID:    stuckTask.ID,
		Status:    RunSucceeded,
	}
	store.runs[completedRun.ID] = completedRun
	store.runsList = []*Run{completedRun}

	wt := NewWorktreeManager("/tmp/test-repo")
	sched := NewScheduler(store)
	workers := NewWorkerLauncher(sched, wt, store)
	rm := NewMissionRecoveryManager(store, wt, workers)

	report, err := rm.RecoverMission(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if report.StuckReset != 1 {
		t.Fatalf("expected 1 stuck task reset, got %d", report.StuckReset)
	}

	summary, err := BuildMissionSummary(context.Background(), store, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if summary.ActiveRuns != 0 {
		t.Fatalf("active runs = %d, want 0", summary.ActiveRuns)
	}
	if summary.PhaseLabel != "Running · ready queue" {
		t.Fatalf("phase label = %q", summary.PhaseLabel)
	}
	if summary.Attention != "1 ready task(s)" {
		t.Fatalf("attention = %q", summary.Attention)
	}
	if summary.FocusTask == nil {
		t.Fatal("expected focus task after recovery")
	}
	if summary.FocusTask.Title != "Repair stale worker lease" {
		t.Fatalf("focus task = %q", summary.FocusTask.Title)
	}
	if summary.FocusTask.Status != TaskReady {
		t.Fatalf("focus task status = %s, want %s", summary.FocusTask.Status, TaskReady)
	}
	if !strings.Contains(summary.NextAction, "Next ready task: Repair stale worker lease") {
		t.Fatalf("next action = %q", summary.NextAction)
	}

	events, err := store.ListEvents(context.Background(), "m1", 10)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected recovery events to be recorded")
	}
	foundReset := false
	foundCompleted := false
	for _, event := range events {
		switch event.Type {
		case "recovery.stuck_task_reset":
			if event.TaskID == stuckTask.ID {
				foundReset = true
			}
		case "recovery.completed":
			foundCompleted = true
		}
	}
	if !foundReset {
		t.Fatal("expected stuck-task recovery event for repairable running mission")
	}
	if !foundCompleted {
		t.Fatal("expected recovery.completed event for repairable running mission")
	}
}

func TestBuildReplanPrompt(t *testing.T) {
	store := newRecoveryMockStore()
	store.missions["m1"] = &Mission{
		ID:   "m1",
		Goal: "Implement user authentication system",
	}

	store.tasks["t1"] = &Task{
		ID: "t1", MissionID: "m1", Title: "Add JWT middleware",
		Kind: TaskKindCode, Status: TaskDone, Objective: "JWT auth",
	}
	store.tasks["t2"] = &Task{
		ID: "t2", MissionID: "m1", Title: "Add OAuth provider",
		Kind: TaskKindCode, Status: TaskFailed, Objective: "OAuth integration",
		AttemptCount: 3, BlockingReason: "API rate limited",
	}
	store.tasks["t3"] = &Task{
		ID: "t3", MissionID: "m1", Title: "Write auth tests",
		Kind: TaskKindTest, Status: TaskPending, Objective: "Test auth",
	}

	store.deps = []TaskDependency{
		{TaskID: "t3", DependsOnID: "t1"},
		{TaskID: "t3", DependsOnID: "t2"},
	}

	failedTasks := []*Task{store.tasks["t2"]}

	rm := NewMissionRecoveryManager(store, nil, nil)

	prompt, err := rm.BuildReplanPrompt(context.Background(), "m1", failedTasks, "OAuth provider consistently rate limited")
	if err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		name     string
		contains string
	}{
		{"mission goal", "user authentication system"},
		{"partial replan header", "Partial Replan"},
		{"done task", "t1"},
		{"failed task ID", "t2"},
		{"failed task title", "Add OAuth provider"},
		{"blocking reason", "API rate limited"},
		{"attempt count", "3"},
		{"pending task", "t3"},
		{"dependency info", "depends on"},
		{"replan reason", "OAuth provider consistently rate limited"},
		{"instruction about partial", "partial plan update"},
		{"instruction about completed", "leave completed tasks untouched"},
	}

	for _, c := range checks {
		if !strings.Contains(prompt, c.contains) {
			t.Errorf("prompt missing %s (%q)", c.name, c.contains)
		}
	}
}
