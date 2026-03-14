package mission

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// workerMockStore extends mockStore with the methods WorkerLauncher needs.
type workerMockStore struct {
	mockStore
	tasks        map[string]*Task
	runs         map[string]*Run
	events       []*Event
	runsList     []*Run // separate from runs map for ListRuns ordering
	createRunErr error  // if set, CreateRun returns this error
}

func newWorkerMockStore() *workerMockStore {
	return &workerMockStore{
		mockStore: mockStore{
			missions: make(map[string]*Mission),
		},
		tasks: make(map[string]*Task),
		runs:  make(map[string]*Run),
	}
}

func (s *workerMockStore) CreateRun(_ context.Context, r *Run) error {
	if s.createRunErr != nil {
		return s.createRunErr
	}
	s.runs[r.ID] = r
	s.runsList = append(s.runsList, r)
	return nil
}

func (s *workerMockStore) GetRun(_ context.Context, id string) (*Run, error) {
	if r, ok := s.runs[id]; ok {
		return r, nil
	}
	return nil, nil
}

func (s *workerMockStore) UpdateRun(_ context.Context, r *Run) error {
	s.runs[r.ID] = r
	return nil
}

func (s *workerMockStore) ListRuns(_ context.Context, _ string) ([]*Run, error) {
	return s.runsList, nil
}

func (s *workerMockStore) GetTask(_ context.Context, id string) (*Task, error) {
	if t, ok := s.tasks[id]; ok {
		return t, nil
	}
	return nil, nil
}

func (s *workerMockStore) UpdateTask(_ context.Context, t *Task) error {
	s.tasks[t.ID] = t
	return nil
}

func (s *workerMockStore) AppendEvent(_ context.Context, e *Event) error {
	s.events = append(s.events, e)
	return nil
}

func (s *workerMockStore) GetReadyTasks(_ context.Context, _ string) ([]*Task, error) {
	return s.ready, nil
}

func (s *workerMockStore) GetMission(_ context.Context, id string) (*Mission, error) {
	return s.missions[id], nil
}

// stubWorktreeManager satisfies the WorktreeManager interface for testing
// without actual git operations.
type stubWorktreeManager struct {
	created  map[string]string // taskID → path
	released []string
}

func newStubWorktreeManager() *stubWorktreeManager {
	return &stubWorktreeManager{created: make(map[string]string)}
}

func (m *stubWorktreeManager) create(taskID string) (string, error) {
	path := "/worktrees/worker-" + taskID
	m.created[taskID] = path
	return path, nil
}

func (m *stubWorktreeManager) release(taskID string) {
	m.released = append(m.released, taskID)
	delete(m.created, taskID)
}

// --- Tests ---

func TestDispatchReadyTasks_Basic(t *testing.T) {
	store := newWorkerMockStore()
	store.missions["m1"] = &Mission{
		ID:         "m1",
		Status:     MissionRunning,
		BaseBranch: "main",
		Budget:     Budget{MaxConcurrentWorkers: 3},
	}
	store.ready = []*Task{
		{ID: "t1", MissionID: "m1", Title: "Task 1", Kind: TaskKindCode, Priority: 1,
			Objective: "Do thing 1", Scope: TaskScope{WritePaths: []string{"pkg/a"}}},
		{ID: "t2", MissionID: "m1", Title: "Task 2", Kind: TaskKindTest, Priority: 2,
			Objective: "Do thing 2", Scope: TaskScope{WritePaths: []string{"pkg/b"}}},
	}
	store.tasks["t1"] = store.ready[0]
	store.tasks["t2"] = store.ready[1]

	// Use a real WorktreeManager pointed at a temp dir — but since we can't
	// run real git in unit tests, we test the launcher logic with a mock approach.
	// We'll override the worktree manager's behavior by testing BuildWorkerPrompt
	// separately, and test the full flow via the store state transitions.
	sched := NewScheduler(store)

	// For unit testing, we need a WorktreeManager that doesn't call git.
	// We'll test the prompt builder and state transitions separately.
	t.Run("prompt contains task details", func(t *testing.T) {
		prompt := BuildWorkerPrompt(store.ready[0], "/worktrees/worker-t1")
		if !strings.Contains(prompt, "Task 1") {
			t.Error("prompt missing task title")
		}
		if !strings.Contains(prompt, "Do thing 1") {
			t.Error("prompt missing objective")
		}
		if !strings.Contains(prompt, "/worktrees/worker-t1") {
			t.Error("prompt missing worktree path")
		}
		if !strings.Contains(prompt, "pkg/a") {
			t.Error("prompt missing write scope")
		}
	})

	t.Run("scheduler selects tasks", func(t *testing.T) {
		tasks, err := sched.SelectTasks(context.Background(), "m1")
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) != 2 {
			t.Fatalf("expected 2 tasks, got %d", len(tasks))
		}
	})
}

func TestDispatchReadyTasks_NotRunning(t *testing.T) {
	store := newWorkerMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionDraft}

	sched := NewScheduler(store)
	wt := NewWorktreeManager("/tmp/test-repo")
	launcher := NewWorkerLauncher(sched, wt, store)

	_, err := launcher.DispatchReadyTasks(context.Background(), "m1")
	if err == nil {
		t.Fatal("expected error for non-running mission")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompleteWorker(t *testing.T) {
	store := newWorkerMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	task := &Task{ID: "t1", MissionID: "m1", Status: TaskRunning, AttemptCount: 1}
	store.tasks["t1"] = task

	now := time.Now().UTC()
	run := &Run{ID: "r1", MissionID: "m1", TaskID: "t1", Status: RunRunning, StartedAt: &now}
	store.runs["r1"] = run

	wt := NewWorktreeManager("/tmp/test-repo")
	launcher := NewWorkerLauncher(nil, wt, store)

	spec := &WorkerSpec{Run: run, Task: task, WorktreePath: "/worktrees/worker-t1"}
	err := launcher.CompleteWorker(context.Background(), spec, "Added the feature")
	if err != nil {
		t.Fatal(err)
	}

	if spec.Run.Status != RunSucceeded {
		t.Errorf("expected run status succeeded, got %s", spec.Run.Status)
	}
	if spec.Run.Summary != "Added the feature" {
		t.Errorf("expected summary 'Added the feature', got %s", spec.Run.Summary)
	}
	if spec.Task.Status != TaskAwaitingReview {
		t.Errorf("expected task status awaiting_review, got %s", spec.Task.Status)
	}

	// Verify event was logged.
	found := false
	for _, e := range store.events {
		if e.Type == "worker.completed" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected worker.completed event")
	}
}

func TestFailWorker_Retry(t *testing.T) {
	store := newWorkerMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	task := &Task{ID: "t1", MissionID: "m1", Status: TaskRunning, AttemptCount: 1}
	store.tasks["t1"] = task

	now := time.Now().UTC()
	run := &Run{ID: "r1", MissionID: "m1", TaskID: "t1", Status: RunRunning, StartedAt: &now}
	store.runs["r1"] = run

	wt := NewWorktreeManager("/tmp/test-repo")
	launcher := NewWorkerLauncher(nil, wt, store)

	spec := &WorkerSpec{Run: run, Task: task}
	err := launcher.FailWorker(context.Background(), spec, "compilation error", 3)
	if err != nil {
		t.Fatal(err)
	}

	if spec.Run.Status != RunFailed {
		t.Errorf("expected run status failed, got %s", spec.Run.Status)
	}
	// Should be back to ready for retry (attempt 1 of max 3).
	if spec.Task.Status != TaskReady {
		t.Errorf("expected task status ready (retry), got %s", spec.Task.Status)
	}
}

func TestFailWorker_MaxAttempts(t *testing.T) {
	store := newWorkerMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	task := &Task{ID: "t1", MissionID: "m1", Status: TaskRunning, AttemptCount: 3}
	store.tasks["t1"] = task

	now := time.Now().UTC()
	run := &Run{ID: "r1", MissionID: "m1", TaskID: "t1", Status: RunRunning, StartedAt: &now}
	store.runs["r1"] = run

	wt := NewWorktreeManager("/tmp/test-repo")
	launcher := NewWorkerLauncher(nil, wt, store)

	spec := &WorkerSpec{Run: run, Task: task}
	err := launcher.FailWorker(context.Background(), spec, "persistent error", 3)
	if err != nil {
		t.Fatal(err)
	}

	// Should be permanently failed (attempt 3 of max 3).
	if spec.Task.Status != TaskFailed {
		t.Errorf("expected task status failed, got %s", spec.Task.Status)
	}
	if !strings.Contains(spec.Task.BlockingReason, "exceeded max attempts") {
		t.Errorf("expected blocking reason to mention max attempts, got %s", spec.Task.BlockingReason)
	}
}

func TestCancelWorker(t *testing.T) {
	store := newWorkerMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	task := &Task{ID: "t1", MissionID: "m1", Status: TaskRunning, AttemptCount: 1}
	store.tasks["t1"] = task

	now := time.Now().UTC()
	run := &Run{ID: "r1", MissionID: "m1", TaskID: "t1", Status: RunRunning, StartedAt: &now}
	store.runs["r1"] = run

	wt := NewWorktreeManager("/tmp/test-repo")
	launcher := NewWorkerLauncher(nil, wt, store)

	spec := &WorkerSpec{Run: run, Task: task}
	err := launcher.CancelWorker(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}

	if spec.Run.Status != RunCancelled {
		t.Errorf("expected run status cancelled, got %s", spec.Run.Status)
	}
	if spec.Task.Status != TaskReady {
		t.Errorf("expected task status ready after cancel, got %s", spec.Task.Status)
	}

	found := false
	for _, e := range store.events {
		if e.Type == "worker.cancelled" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected worker.cancelled event")
	}
}

func TestRecoverStaleWorkers(t *testing.T) {
	store := newWorkerMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	past := time.Now().UTC().Add(-1 * time.Hour)
	future := time.Now().UTC().Add(1 * time.Hour)

	// Stale run — lease expired.
	task1 := &Task{ID: "t1", MissionID: "m1", Status: TaskRunning}
	store.tasks["t1"] = task1
	staleRun := &Run{
		ID: "r1", MissionID: "m1", TaskID: "t1",
		Status: RunRunning, LeaseExpires: &past,
	}
	store.runs["r1"] = staleRun

	// Active run — lease still valid.
	task2 := &Task{ID: "t2", MissionID: "m1", Status: TaskRunning}
	store.tasks["t2"] = task2
	activeRun := &Run{
		ID: "r2", MissionID: "m1", TaskID: "t2",
		Status: RunRunning, LeaseExpires: &future,
	}
	store.runs["r2"] = activeRun

	store.runsList = []*Run{staleRun, activeRun}

	wt := NewWorktreeManager("/tmp/test-repo")
	launcher := NewWorkerLauncher(nil, wt, store)

	recovered, err := launcher.RecoverStaleWorkers(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	if recovered != 1 {
		t.Fatalf("expected 1 recovered, got %d", recovered)
	}

	if staleRun.Status != RunLeaseLost {
		t.Errorf("expected stale run status lease_lost, got %s", staleRun.Status)
	}
	if task1.Status != TaskReady {
		t.Errorf("expected stale task back to ready, got %s", task1.Status)
	}

	// Active run should be untouched.
	if activeRun.Status != RunRunning {
		t.Errorf("expected active run still running, got %s", activeRun.Status)
	}
}

func TestHeartbeatWorker(t *testing.T) {
	store := newWorkerMockStore()

	now := time.Now().UTC()
	run := &Run{ID: "r1", MissionID: "m1", Status: RunRunning, HeartbeatAt: &now}
	store.runs["r1"] = run

	launcher := NewWorkerLauncher(nil, nil, store)
	spec := &WorkerSpec{Run: run}

	time.Sleep(10 * time.Millisecond) // ensure time advances
	err := launcher.HeartbeatWorker(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}

	if spec.Run.HeartbeatAt.Before(now) || spec.Run.HeartbeatAt.Equal(now) {
		t.Error("expected heartbeat to advance")
	}
	if spec.Run.LeaseExpires == nil {
		t.Fatal("expected lease expiry to be set")
	}
	if spec.Run.LeaseExpires.Before(time.Now().UTC()) {
		t.Error("expected lease expiry to be in the future")
	}
}

func TestBuildWorkerPrompt(t *testing.T) {
	task := &Task{
		ID:        "t1",
		Title:     "Add user authentication",
		Kind:      TaskKindCode,
		Objective: "Implement JWT-based auth middleware for the API layer.",
		Scope: TaskScope{
			WritePaths: []string{"pkg/auth", "pkg/api/middleware.go"},
			ReadPaths:  []string{"pkg/config", "docs/auth-spec.md"},
		},
		AcceptanceCriteria: []string{
			"JWT tokens are validated on protected routes",
			"Unauthorized requests return 401",
		},
	}

	prompt := BuildWorkerPrompt(task, "/worktrees/worker-t1")

	checks := []struct {
		name     string
		contains string
	}{
		{"task ID", "t1"},
		{"title", "Add user authentication"},
		{"kind", "code"},
		{"worktree", "/worktrees/worker-t1"},
		{"objective", "JWT-based auth middleware"},
		{"write scope", "pkg/auth"},
		{"write scope file", "pkg/api/middleware.go"},
		{"read scope", "pkg/config"},
		{"acceptance criteria", "JWT tokens are validated"},
		{"acceptance criteria 2", "Unauthorized requests return 401"},
		{"rule about worktree", "Work ONLY within your worktree"},
		{"rule about commits", "Commit your changes"},
		{"rule about push", "Push your branch to the remote"},
	}

	for _, c := range checks {
		if !strings.Contains(prompt, c.contains) {
			t.Errorf("prompt missing %s (%q)", c.name, c.contains)
		}
	}
}

func TestProvisionWorkerReusesExistingWorktree(t *testing.T) {
	store := newWorkerMockStore()
	store.missions["m1"] = &Mission{
		ID:         "m1",
		Status:     MissionRunning,
		BaseBranch: "main",
		Budget:     Budget{MaxConcurrentWorkers: 3},
	}
	task := &Task{
		ID:        "t1",
		MissionID: "m1",
		Title:     "Task with existing worktree",
		Kind:      TaskKindCode,
		Objective: "Iterate on review feedback",
		Status:    TaskReady,
	}
	store.ready = []*Task{task}
	store.tasks["t1"] = task

	// Create a WorktreeManager with a pre-existing worktree for t1.
	// This simulates the request_changes case where the worktree is preserved.
	wm := NewWorktreeManager("/tmp/test-repo")
	wm.active["t1"] = "/existing/worktree/worker-t1"

	sched := NewScheduler(store)
	launcher := NewWorkerLauncher(sched, wm, store)

	specs, err := launcher.DispatchReadyTasks(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}

	// The worktree path should be the existing one, not a newly created one.
	if specs[0].WorktreePath != "/existing/worktree/worker-t1" {
		t.Fatalf("expected existing worktree path, got %s", specs[0].WorktreePath)
	}

	// The active map should still have the original entry (not replaced).
	path, ok := wm.Get("t1")
	if !ok {
		t.Fatal("worktree should still be tracked")
	}
	if path != "/existing/worktree/worker-t1" {
		t.Fatalf("worktree path changed unexpectedly to %s", path)
	}
}

func TestProvisionWorker_CreateRunFails_AttemptCounted(t *testing.T) {
	store := newWorkerMockStore()
	store.missions["m1"] = &Mission{
		ID:         "m1",
		Status:     MissionRunning,
		BaseBranch: "main",
		Budget:     Budget{MaxConcurrentWorkers: 3},
	}
	task := &Task{
		ID:           "t1",
		MissionID:    "m1",
		Title:        "Failing task",
		Kind:         TaskKindCode,
		Objective:    "Will fail on CreateRun",
		Status:       TaskReady,
		AttemptCount: 0,
	}
	store.ready = []*Task{task}
	store.tasks["t1"] = task

	// Inject CreateRun failure.
	store.createRunErr = fmt.Errorf("injected CreateRun failure")

	wm := NewWorktreeManager("/tmp/test-repo")
	wm.active["t1"] = "/existing/worktree/worker-t1" // pre-provision worktree

	sched := NewScheduler(store)
	launcher := NewWorkerLauncher(sched, wm, store)

	specs, err := launcher.DispatchReadyTasks(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	// Task should have failed provisioning — no specs returned.
	if len(specs) != 0 {
		t.Fatalf("expected 0 specs, got %d", len(specs))
	}

	// AttemptCount must be incremented even though CreateRun failed.
	if task.AttemptCount != 1 {
		t.Errorf("expected AttemptCount=1 after failed CreateRun, got %d", task.AttemptCount)
	}
	// Task should be reverted to TaskReady for retry.
	if task.Status != TaskReady {
		t.Errorf("expected task status ready after failed CreateRun, got %s", task.Status)
	}
}

func TestProvisionWorker_CreateRunFails_MaxAttemptsExceeded(t *testing.T) {
	store := newWorkerMockStore()
	store.missions["m1"] = &Mission{
		ID:         "m1",
		Status:     MissionRunning,
		BaseBranch: "main",
		Budget:     Budget{MaxConcurrentWorkers: 3},
	}
	task := &Task{
		ID:           "t1",
		MissionID:    "m1",
		Title:        "Repeatedly failing task",
		Kind:         TaskKindCode,
		Objective:    "Will fail on CreateRun",
		Status:       TaskReady,
		AttemptCount: 2, // Already 2 attempts
	}
	store.ready = []*Task{task}
	store.tasks["t1"] = task

	// Inject CreateRun failure.
	store.createRunErr = fmt.Errorf("injected CreateRun failure")

	wm := NewWorktreeManager("/tmp/test-repo")
	wm.active["t1"] = "/existing/worktree/worker-t1"

	sched := NewScheduler(store)
	launcher := NewWorkerLauncher(sched, wm, store)

	specs, _ := launcher.DispatchReadyTasks(context.Background(), "m1")
	if len(specs) != 0 {
		t.Fatalf("expected 0 specs, got %d", len(specs))
	}

	// AttemptCount should be 3 now — visible for max-attempt checks.
	if task.AttemptCount != 3 {
		t.Errorf("expected AttemptCount=3, got %d", task.AttemptCount)
	}
}

func TestBuildWorkerPrompt_MinimalTask(t *testing.T) {
	task := &Task{
		ID:        "t2",
		Title:     "Quick fix",
		Kind:      TaskKindCode,
		Objective: "Fix the typo.",
	}

	prompt := BuildWorkerPrompt(task, "/wt/t2")

	// Should still have the basics.
	if !strings.Contains(prompt, "Quick fix") {
		t.Error("prompt missing title")
	}
	if !strings.Contains(prompt, "Fix the typo") {
		t.Error("prompt missing objective")
	}
	// Should NOT have scope sections when empty.
	if strings.Contains(prompt, "Writable Scope") {
		t.Error("prompt should not have writable scope section for empty scope")
	}
	if strings.Contains(prompt, "Read Scope") {
		t.Error("prompt should not have read scope section for empty scope")
	}
}
