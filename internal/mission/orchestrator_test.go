package mission

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test mocks
// ---------------------------------------------------------------------------

// mockHandle implements AgentHandle with controllable completion.
// It respects context cancellation so orchestrator shutdown is clean.
type mockHandle struct {
	ctx     context.Context
	ch      chan struct{} // closed when Wait should return
	summary string
	err     error
}

func newMockHandle(ctx context.Context) *mockHandle {
	return &mockHandle{ctx: ctx, ch: make(chan struct{})}
}

func (h *mockHandle) Wait() (string, error) {
	select {
	case <-h.ch:
		return h.summary, h.err
	case <-h.ctx.Done():
		return "", h.ctx.Err()
	}
}

func (h *mockHandle) complete(summary string, err error) {
	h.summary = summary
	h.err = err
	close(h.ch)
}

// mockSpawner implements AgentSpawner, tracking spawn calls and controlling
// when agents complete.
type mockSpawner struct {
	mu             sync.Mutex
	workerCalls    []*WorkerSpec
	workerHandles  []*mockHandle
	reviewCalls    []*ReviewSpec
	reviewHandles  []*mockHandle
	spawnWorkerErr error
}

func (s *mockSpawner) SpawnWorker(ctx context.Context, spec *WorkerSpec) (AgentHandle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.spawnWorkerErr != nil {
		return nil, s.spawnWorkerErr
	}
	h := newMockHandle(ctx)
	s.workerCalls = append(s.workerCalls, spec)
	s.workerHandles = append(s.workerHandles, h)
	return h, nil
}

func (s *mockSpawner) SpawnReviewer(ctx context.Context, spec *ReviewSpec) (AgentHandle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := newMockHandle(ctx)
	s.reviewCalls = append(s.reviewCalls, spec)
	s.reviewHandles = append(s.reviewHandles, h)
	return h, nil
}

func (s *mockSpawner) workerCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.workerCalls)
}

func (s *mockSpawner) reviewCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.reviewCalls)
}

func (s *mockSpawner) getWorkerHandle(i int) *mockHandle {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.workerHandles[i]
}

func (s *mockSpawner) getReviewHandle(i int) *mockHandle {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reviewHandles[i]
}

// ---------------------------------------------------------------------------
// storeWorkerDispatcher — dispatches from store without git worktrees
// ---------------------------------------------------------------------------

type storeWorkerDispatcher struct {
	store            Store
	mu               sync.Mutex
	releasedWorktrees []string // taskIDs whose worktrees were released
}

func (d *storeWorkerDispatcher) DispatchReadyTasks(ctx context.Context, missionID string) ([]*WorkerSpec, error) {
	m, err := d.store.GetMission(ctx, missionID)
	if err != nil {
		return nil, err
	}
	if m.Status != MissionRunning {
		return nil, fmt.Errorf("mission %s is %s, not running", missionID, m.Status)
	}

	tasks, err := d.store.GetReadyTasks(ctx, missionID)
	if err != nil || len(tasks) == 0 {
		return nil, err
	}

	// Respect concurrency limit.
	runs, _ := d.store.ListRuns(ctx, missionID)
	running := 0
	for _, r := range runs {
		if r.Status == RunRunning {
			running++
		}
	}
	maxWorkers := m.Budget.MaxConcurrentWorkers
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	slots := maxWorkers - running
	if slots <= 0 {
		return nil, nil
	}

	var specs []*WorkerSpec
	for _, t := range tasks {
		if len(specs) >= slots {
			break
		}

		now := time.Now().UTC()
		run := &Run{
			ID:           generateID("r"),
			MissionID:    missionID,
			TaskID:       t.ID,
			Mode:         RunModeWorker,
			Status:       RunRunning,
			StartedAt:    &now,
			WorktreePath: "/test/worktree/" + t.ID,
		}
		if err := d.store.CreateRun(ctx, run); err != nil {
			continue
		}

		t.Status = TaskRunning
		t.AttemptCount++
		t.UpdatedAt = now
		d.store.UpdateTask(ctx, t) //nolint:errcheck

		specs = append(specs, &WorkerSpec{
			Run:          run,
			Task:         t,
			WorktreePath: run.WorktreePath,
			Prompt:       BuildWorkerPrompt(t, run.WorktreePath),
		})
	}
	return specs, nil
}

func (d *storeWorkerDispatcher) CompleteWorker(ctx context.Context, spec *WorkerSpec, summary string) error {
	now := time.Now().UTC()
	spec.Run.Status = RunSucceeded
	spec.Run.EndedAt = &now
	spec.Run.Summary = summary
	if err := d.store.UpdateRun(ctx, spec.Run); err != nil {
		return err
	}
	spec.Task.Status = TaskAwaitingReview
	spec.Task.UpdatedAt = now
	return d.store.UpdateTask(ctx, spec.Task)
}

func (d *storeWorkerDispatcher) FailWorker(ctx context.Context, spec *WorkerSpec, errText string, maxAttempts int) error {
	now := time.Now().UTC()
	spec.Run.Status = RunFailed
	spec.Run.EndedAt = &now
	spec.Run.ErrorText = errText
	d.store.UpdateRun(ctx, spec.Run) //nolint:errcheck

	if maxAttempts > 0 && spec.Task.AttemptCount >= maxAttempts {
		spec.Task.Status = TaskFailed
	} else {
		spec.Task.Status = TaskReady
	}
	spec.Task.UpdatedAt = now
	return d.store.UpdateTask(ctx, spec.Task)
}

func (d *storeWorkerDispatcher) HeartbeatWorker(ctx context.Context, spec *WorkerSpec) error {
	now := time.Now().UTC()
	spec.Run.HeartbeatAt = &now
	return d.store.UpdateRun(ctx, spec.Run)
}

func (d *storeWorkerDispatcher) ReleaseWorkerWorktree(_ context.Context, _, taskID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.releasedWorktrees = append(d.releasedWorktrees, taskID)
}

func (d *storeWorkerDispatcher) worktreeReleasedFor(taskID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, id := range d.releasedWorktrees {
		if id == taskID {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// storeReviewDispatcher — dispatches reviews from store without git diff
// ---------------------------------------------------------------------------

type storeReviewDispatcher struct {
	store Store
}

func (d *storeReviewDispatcher) DispatchPendingReviews(ctx context.Context, missionID, _ string) ([]*ReviewSpec, error) {
	tasks, err := d.store.GetTasksByStatus(ctx, missionID, TaskAwaitingReview)
	if err != nil || len(tasks) == 0 {
		return nil, err
	}

	var specs []*ReviewSpec
	for _, t := range tasks {
		// Skip if a review is already running.
		runs, _ := d.store.GetRunsForTask(ctx, t.ID)
		hasActive := false
		var workerRun *Run
		for _, r := range runs {
			if r.Mode == RunModeReview && r.Status == RunRunning {
				hasActive = true
			}
			if r.Mode == RunModeWorker && r.Status == RunSucceeded {
				workerRun = r
			}
		}
		if hasActive {
			continue
		}

		now := time.Now().UTC()
		run := &Run{
			ID:        generateID("r"),
			MissionID: missionID,
			TaskID:    t.ID,
			Mode:      RunModeReview,
			Status:    RunRunning,
			StartedAt: &now,
		}
		d.store.CreateRun(ctx, run) //nolint:errcheck

		specs = append(specs, &ReviewSpec{
			Run:       run,
			Task:      t,
			WorkerRun: workerRun,
			Prompt:    "review prompt for " + t.ID,
			DiffText:  "fake diff",
		})
	}
	return specs, nil
}

func (d *storeReviewDispatcher) CompleteReview(ctx context.Context, spec *ReviewSpec, result *ReviewResult) error {
	now := time.Now().UTC()
	spec.Run.Status = RunSucceeded
	spec.Run.EndedAt = &now
	spec.Run.Summary = result.Summary
	d.store.UpdateRun(ctx, spec.Run) //nolint:errcheck

	switch result.Verdict {
	case ReviewPass:
		spec.Task.Status = TaskAccepted
	case ReviewReject:
		spec.Task.Status = TaskReady
	case ReviewRequestChanges:
		spec.Task.Status = TaskReady
		spec.Task.BlockingReason = result.Suggestion
	}
	spec.Task.UpdatedAt = now
	return d.store.UpdateTask(ctx, spec.Task)
}

func (d *storeReviewDispatcher) FailReview(ctx context.Context, spec *ReviewSpec, errText string) error {
	now := time.Now().UTC()
	spec.Run.Status = RunFailed
	spec.Run.EndedAt = &now
	spec.Run.ErrorText = errText
	return d.store.UpdateRun(ctx, spec.Run)
}

// ---------------------------------------------------------------------------
// storeIntegrator — integrates in store without git merge
// ---------------------------------------------------------------------------

type storeIntegrator struct {
	store Store
}

func (si *storeIntegrator) IntegrateReady(ctx context.Context, missionID string) ([]*IntegrationResult, error) {
	tasks, err := si.store.GetTasksByStatus(ctx, missionID, TaskAccepted)
	if err != nil {
		return nil, err
	}

	var results []*IntegrationResult
	for _, t := range tasks {
		now := time.Now().UTC()
		t.Status = TaskIntegrated
		t.UpdatedAt = now
		si.store.UpdateTask(ctx, t) //nolint:errcheck

		results = append(results, &IntegrationResult{
			TaskID:       t.ID,
			MergedCommit: "fake-commit-" + t.ID,
			Success:      true,
		})
	}

	// Resolve pending → ready for downstream tasks.
	si.resolveReady(ctx, missionID)

	return results, nil
}

func (si *storeIntegrator) CheckMissionComplete(ctx context.Context, missionID string) (bool, error) {
	tasks, err := si.store.ListTasks(ctx, missionID)
	if err != nil {
		return false, err
	}
	if len(tasks) == 0 {
		return false, nil
	}
	for _, t := range tasks {
		if t.Status != TaskIntegrated && t.Status != TaskDone {
			return false, nil
		}
	}
	return true, nil
}

func (si *storeIntegrator) CompleteMission(ctx context.Context, missionID string) error {
	m, err := si.store.GetMission(ctx, missionID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	m.Status = MissionCompleted
	m.UpdatedAt = now
	m.EndedAt = &now
	return si.store.UpdateMission(ctx, m)
}

func (si *storeIntegrator) resolveReady(ctx context.Context, missionID string) {
	tasks, _ := si.store.ListTasks(ctx, missionID)
	deps, _ := si.store.ListDependencies(ctx, missionID)

	doneSet := make(map[string]bool)
	for _, t := range tasks {
		if t.Status == TaskDone || t.Status == TaskIntegrated {
			doneSet[t.ID] = true
		}
	}

	unsatisfied := make(map[string]int)
	for _, d := range deps {
		if !doneSet[d.DependsOnID] {
			unsatisfied[d.TaskID]++
		}
	}

	now := time.Now().UTC()
	for _, t := range tasks {
		if t.Status == TaskPending && unsatisfied[t.ID] == 0 {
			t.Status = TaskReady
			t.UpdatedAt = now
			si.store.UpdateTask(ctx, t) //nolint:errcheck
		}
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestOrchestratorWithDispatcher(store Store, spawner *mockSpawner, events chan OrchestratorEvent) (*Orchestrator, *storeWorkerDispatcher) {
	wd := &storeWorkerDispatcher{store: store}
	orch := newTestOrchestratorFull(store, spawner, wd, events)
	return orch, wd
}

func newTestOrchestratorFull(store Store, spawner AgentSpawner, workers workerDispatcher, events chan OrchestratorEvent) *Orchestrator {
	return &Orchestrator{
		cfg: OrchestratorConfig{
			MissionID:         "m1",
			RepoRoot:          "/test/repo",
			TickInterval:      50 * time.Millisecond,
			MaxAttempts:       3,
			HeartbeatInterval: time.Minute,
		},
		store:   store,
		spawner: spawner,
		workers: workers,
		reviews: &storeReviewDispatcher{store: store},
		integr:  &storeIntegrator{store: store},
		onEvent: func(e OrchestratorEvent) {
			select {
			case events <- e:
			default:
			}
		},
		logger: slog.Default(),
		active: make(map[string]*activeAgent),
		done:   make(chan struct{}),
	}
}

func newTestOrchestrator(store Store, spawner *mockSpawner, events chan OrchestratorEvent) *Orchestrator {
	orch, _ := newTestOrchestratorWithDispatcher(store, spawner, events)
	return orch
}


func waitFor(t *testing.T, timeout time.Duration, desc string, fn func() bool) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if fn() {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for: %s", desc)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func waitForEvent(t *testing.T, events chan OrchestratorEvent, eventType string, timeout time.Duration) OrchestratorEvent {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case e := <-events:
			if e.Type == eventType {
				return e
			}
		case <-deadline:
			t.Fatalf("timeout waiting for event: %s", eventType)
		}
	}
}

func setupRunningMission(t *testing.T, store *memoryStore) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	if err := store.CreateMission(ctx, &Mission{
		ID:         "m1",
		Title:      "Test mission",
		Goal:       "Implement and test a feature",
		Status:     MissionRunning,
		Budget:     Budget{MaxConcurrentWorkers: 4},
		CreatedAt:  now,
		UpdatedAt:  now,
		StartedAt:  &now,
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.CreateTask(ctx, &Task{
		ID:        "t1",
		MissionID: "m1",
		Title:     "Implement feature",
		Kind:      TaskKindCode,
		Objective: "Build the feature",
		Status:    TaskReady,
		Priority:  1,
		RiskLevel: RiskLow,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.CreateTask(ctx, &Task{
		ID:        "t2",
		MissionID: "m1",
		Title:     "Add tests",
		Kind:      TaskKindTest,
		Objective: "Write tests for the feature",
		Status:    TaskPending,
		Priority:  2,
		RiskLevel: RiskLow,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.AddDependency(ctx, TaskDependency{
		TaskID:      "t2",
		DependsOnID: "t1",
	}); err != nil {
		t.Fatal(err)
	}
}

const reviewPassJSON = `Here is my review:

` + "```json" + `
{
  "verdict": "pass",
  "summary": "LGTM - implementation looks correct"
}
` + "```"

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestOrchestratorFullLifecycle(t *testing.T) {
	store := newMemoryStore()
	setupRunningMission(t, store)
	spawner := &mockSpawner{}
	events := make(chan OrchestratorEvent, 100)
	orch := newTestOrchestrator(store, spawner, events)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	orch.Start(ctx)
	defer orch.Stop()

	// --- Phase 1: Worker dispatched for t1 ---
	waitFor(t, 2*time.Second, "worker spawn for t1", func() bool {
		return spawner.workerCount() >= 1
	})
	t.Log("Worker spawned for t1")

	// Verify t1 is now running.
	t1, _ := store.GetTask(ctx, "t1")
	if t1.Status != TaskRunning {
		t.Fatalf("t1 status = %s, want running", t1.Status)
	}

	// Complete the worker.
	spawner.getWorkerHandle(0).complete("Implemented the feature", nil)
	waitForEvent(t, events, "worker.completed", 2*time.Second)
	t.Log("Worker completed for t1")

	// --- Phase 2: Review dispatched for t1 ---
	waitFor(t, 2*time.Second, "review spawn for t1", func() bool {
		return spawner.reviewCount() >= 1
	})
	t.Log("Review spawned for t1")

	// Complete the review with pass.
	spawner.getReviewHandle(0).complete(reviewPassJSON, nil)
	waitForEvent(t, events, "review.pass", 2*time.Second)
	t.Log("Review passed for t1")

	// --- Phase 3: Integration + dependency resolution ---
	// The next tick should integrate t1 (accepted → integrated) and
	// resolve t2 (pending → ready), then dispatch worker for t2.
	waitFor(t, 2*time.Second, "worker spawn for t2", func() bool {
		return spawner.workerCount() >= 2
	})
	t.Log("Worker spawned for t2")

	// Verify t2 is running.
	t2, _ := store.GetTask(ctx, "t2")
	if t2.Status != TaskRunning {
		t.Fatalf("t2 status = %s, want running", t2.Status)
	}

	// Complete worker for t2.
	spawner.getWorkerHandle(1).complete("Added comprehensive tests", nil)
	waitForEvent(t, events, "worker.completed", 2*time.Second)
	t.Log("Worker completed for t2")

	// --- Phase 4: Review for t2 ---
	waitFor(t, 2*time.Second, "review spawn for t2", func() bool {
		return spawner.reviewCount() >= 2
	})

	spawner.getReviewHandle(1).complete(reviewPassJSON, nil)
	waitForEvent(t, events, "review.pass", 2*time.Second)
	t.Log("Review passed for t2")

	// --- Phase 5: Mission completion ---
	waitForEvent(t, events, "mission.completed", 2*time.Second)
	t.Log("Mission completed!")

	// Verify final state.
	m, _ := store.GetMission(ctx, "m1")
	if m.Status != MissionCompleted {
		t.Fatalf("mission status = %s, want completed", m.Status)
	}
	if m.EndedAt == nil {
		t.Fatal("expected EndedAt to be set")
	}

	// Verify all tasks are integrated.
	tasks, _ := store.ListTasks(ctx, "m1")
	for _, task := range tasks {
		if task.Status != TaskIntegrated {
			t.Fatalf("task %s status = %s, want integrated", task.ID, task.Status)
		}
	}
}

func TestOrchestratorWorkerFailureRetries(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	store.CreateMission(ctx, &Mission{
		ID:        "m1",
		Status:    MissionRunning,
		Budget:    Budget{MaxConcurrentWorkers: 2},
		CreatedAt: now,
		UpdatedAt: now,
		StartedAt: &now,
	})
	store.CreateTask(ctx, &Task{
		ID:        "t1",
		MissionID: "m1",
		Title:     "Flaky task",
		Kind:      TaskKindCode,
		Objective: "Do something flaky",
		Status:    TaskReady,
		Priority:  1,
		RiskLevel: RiskLow,
		CreatedAt: now,
		UpdatedAt: now,
	})

	spawner := &mockSpawner{}
	events := make(chan OrchestratorEvent, 100)
	orch := newTestOrchestrator(store, spawner, events)

	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	orch.Start(testCtx)
	defer orch.Stop()

	// Wait for first worker spawn.
	waitFor(t, 2*time.Second, "worker spawn 1", func() bool {
		return spawner.workerCount() >= 1
	})

	// Fail the worker.
	spawner.getWorkerHandle(0).complete("", fmt.Errorf("compile error"))
	waitForEvent(t, events, "worker.failed", 2*time.Second)

	// Task should be back to ready (attempt 1 of 3).
	task, _ := store.GetTask(ctx, "t1")
	if task.Status != TaskReady {
		t.Fatalf("task status after failure = %s, want ready (retry)", task.Status)
	}

	// Wait for second worker spawn (retry).
	waitFor(t, 2*time.Second, "worker spawn 2", func() bool {
		return spawner.workerCount() >= 2
	})
	t.Log("Worker retried after failure")

	// This time succeed.
	spawner.getWorkerHandle(1).complete("Fixed it", nil)
	waitForEvent(t, events, "worker.completed", 2*time.Second)

	task, _ = store.GetTask(ctx, "t1")
	if task.Status != TaskAwaitingReview {
		t.Fatalf("task status = %s, want awaiting_review", task.Status)
	}
}

func TestOrchestratorStopCancelsAgents(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	store.CreateMission(ctx, &Mission{
		ID:        "m1",
		Status:    MissionRunning,
		Budget:    Budget{MaxConcurrentWorkers: 2},
		CreatedAt: now,
		UpdatedAt: now,
		StartedAt: &now,
	})
	store.CreateTask(ctx, &Task{
		ID:        "t1",
		MissionID: "m1",
		Title:     "Long running task",
		Kind:      TaskKindCode,
		Objective: "Do something slow",
		Status:    TaskReady,
		Priority:  1,
		RiskLevel: RiskLow,
		CreatedAt: now,
		UpdatedAt: now,
	})

	spawner := &mockSpawner{}
	events := make(chan OrchestratorEvent, 100)
	orch := newTestOrchestrator(store, spawner, events)

	orch.Start(context.Background())

	// Wait for worker to be spawned.
	waitFor(t, 2*time.Second, "worker spawn", func() bool {
		return spawner.workerCount() >= 1
	})

	if orch.ActiveRunCount() != 1 {
		t.Fatalf("active runs = %d, want 1", orch.ActiveRunCount())
	}

	// Stop the orchestrator — should cancel in-flight agents.
	// The mock handle's Wait returns immediately because its context is cancelled.
	orch.Stop()

	// Orchestrator should be fully stopped.
	if orch.ActiveRunCount() != 0 {
		t.Fatalf("active runs after stop = %d, want 0", orch.ActiveRunCount())
	}
}

func TestOrchestratorSkipsPausedMission(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	store.CreateMission(ctx, &Mission{
		ID:        "m1",
		Status:    MissionPaused,
		Budget:    Budget{MaxConcurrentWorkers: 2},
		CreatedAt: now,
		UpdatedAt: now,
	})
	store.CreateTask(ctx, &Task{
		ID:        "t1",
		MissionID: "m1",
		Title:     "Waiting task",
		Status:    TaskReady,
		Priority:  1,
		RiskLevel: RiskLow,
		CreatedAt: now,
		UpdatedAt: now,
	})

	spawner := &mockSpawner{}
	events := make(chan OrchestratorEvent, 100)
	orch := newTestOrchestrator(store, spawner, events)

	testCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	orch.Start(testCtx)
	<-orch.done // Wait for orchestrator to finish (context timeout)

	if spawner.workerCount() != 0 {
		t.Fatalf("expected no workers spawned for paused mission, got %d", spawner.workerCount())
	}
}

func TestOrchestratorExitsOnTerminalMission(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	store.CreateMission(ctx, &Mission{
		ID:        "m1",
		Status:    MissionCompleted,
		CreatedAt: now,
		UpdatedAt: now,
	})

	spawner := &mockSpawner{}
	events := make(chan OrchestratorEvent, 100)
	orch := newTestOrchestrator(store, spawner, events)

	orch.Start(context.Background())

	// Should exit quickly upon seeing terminal state.
	select {
	case <-orch.done:
		// Good — orchestrator stopped.
	case <-time.After(2 * time.Second):
		t.Fatal("orchestrator did not exit for terminal mission")
	}
}

func TestParseReviewResult(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		verdict ReviewVerdict
		wantErr bool
	}{
		{
			name: "json in code block",
			input: "Here is my review:\n```json\n" +
				`{"verdict":"pass","summary":"LGTM"}` +
				"\n```",
			verdict: ReviewPass,
		},
		{
			name:    "raw json",
			input:   `{"verdict":"reject","summary":"Bugs found","suggestion":"Fix the null check"}`,
			verdict: ReviewReject,
		},
		{
			name: "json with surrounding text",
			input: "After careful analysis:\n" +
				`{"verdict":"request_changes","summary":"Needs work","suggestion":"Add error handling"}` +
				"\nPlease address these issues.",
			verdict: ReviewRequestChanges,
		},
		{
			name:    "no json",
			input:   "This review has no structured output",
			wantErr: true,
		},
		{
			name:    "empty verdict",
			input:   `{"summary":"Missing verdict field"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseReviewResult(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Verdict != tt.verdict {
				t.Fatalf("verdict = %s, want %s", result.Verdict, tt.verdict)
			}
		})
	}
}

func TestOrchestratorReviewRejectCausesRetry(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	store.CreateMission(ctx, &Mission{
		ID:        "m1",
		Status:    MissionRunning,
		Budget:    Budget{MaxConcurrentWorkers: 2},
		CreatedAt: now,
		UpdatedAt: now,
		StartedAt: &now,
	})
	store.CreateTask(ctx, &Task{
		ID:        "t1",
		MissionID: "m1",
		Title:     "Task needing revision",
		Kind:      TaskKindCode,
		Objective: "Do the thing correctly",
		Status:    TaskReady,
		Priority:  1,
		RiskLevel: RiskLow,
		CreatedAt: now,
		UpdatedAt: now,
	})

	spawner := &mockSpawner{}
	events := make(chan OrchestratorEvent, 100)
	orch := newTestOrchestrator(store, spawner, events)

	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	orch.Start(testCtx)
	defer orch.Stop()

	// Worker 1 spawned.
	waitFor(t, 2*time.Second, "worker spawn 1", func() bool {
		return spawner.workerCount() >= 1
	})
	spawner.getWorkerHandle(0).complete("First attempt", nil)
	waitForEvent(t, events, "worker.completed", 2*time.Second)

	// Review spawned.
	waitFor(t, 2*time.Second, "review spawn 1", func() bool {
		return spawner.reviewCount() >= 1
	})

	// Reject the review.
	rejectJSON := "```json\n" +
		`{"verdict":"reject","summary":"Code has bugs"}` +
		"\n```"
	spawner.getReviewHandle(0).complete(rejectJSON, nil)
	waitForEvent(t, events, "review.reject", 2*time.Second)

	// Task should be back to ready for retry.
	task, _ := store.GetTask(ctx, "t1")
	if task.Status != TaskReady {
		t.Fatalf("task status after rejection = %s, want ready", task.Status)
	}

	// Worker should be re-spawned.
	waitFor(t, 2*time.Second, "worker spawn 2 (retry after rejection)", func() bool {
		return spawner.workerCount() >= 2
	})
	t.Log("Worker retried after review rejection")
}

func TestOrchestratorRequestChangesPreservesWorktree(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	store.CreateMission(ctx, &Mission{
		ID:        "m1",
		Status:    MissionRunning,
		Budget:    Budget{MaxConcurrentWorkers: 2},
		CreatedAt: now,
		UpdatedAt: now,
		StartedAt: &now,
	})
	store.CreateTask(ctx, &Task{
		ID:        "t1",
		MissionID: "m1",
		Title:     "Task needing iteration",
		Kind:      TaskKindCode,
		Objective: "Implement with feedback",
		Status:    TaskReady,
		Priority:  1,
		RiskLevel: RiskLow,
		CreatedAt: now,
		UpdatedAt: now,
	})

	spawner := &mockSpawner{}
	events := make(chan OrchestratorEvent, 100)
	orch, wd := newTestOrchestratorWithDispatcher(store, spawner, events)

	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	orch.Start(testCtx)
	defer orch.Stop()

	// Worker 1 spawned and completes.
	waitFor(t, 2*time.Second, "worker spawn 1", func() bool {
		return spawner.workerCount() >= 1
	})
	spawner.getWorkerHandle(0).complete("First attempt done", nil)
	waitForEvent(t, events, "worker.completed", 2*time.Second)

	// Review spawned.
	waitFor(t, 2*time.Second, "review spawn 1", func() bool {
		return spawner.reviewCount() >= 1
	})

	// Review returns request_changes (NOT reject).
	requestChangesJSON := "```json\n" +
		`{"verdict":"request_changes","summary":"Needs improvements","suggestion":"Add error handling for edge cases"}` +
		"\n```"
	spawner.getReviewHandle(0).complete(requestChangesJSON, nil)
	waitForEvent(t, events, "review.request_changes", 2*time.Second)

	// Worktree should NOT have been released.
	if wd.worktreeReleasedFor("t1") {
		t.Fatal("worktree was released on request_changes — should be preserved for worker retry")
	}

	// Task should be back to ready with feedback in BlockingReason.
	task, _ := store.GetTask(ctx, "t1")
	if task.Status != TaskReady {
		t.Fatalf("task status after request_changes = %s, want ready", task.Status)
	}
	if task.BlockingReason != "Add error handling for edge cases" {
		t.Fatalf("blocking reason = %q, want feedback suggestion", task.BlockingReason)
	}

	// Worker should be re-spawned to iterate on feedback.
	waitFor(t, 2*time.Second, "worker spawn 2 (retry after request_changes)", func() bool {
		return spawner.workerCount() >= 2
	})
	t.Log("Worker retried after request_changes — worktree preserved")
}

// panicHandle is a mock AgentHandle whose Wait panics.
type panicHandle struct {
	ctx context.Context
	msg string
}

func (h *panicHandle) Wait() (string, error) {
	panic(h.msg)
}

// panicSpawner spawns handles that panic on Wait.
type panicSpawner struct {
	mu           sync.Mutex
	workerCalls  []*WorkerSpec
	reviewCalls  []*ReviewSpec
	panicMsg     string
	panicWorker  bool // if true, worker panics; otherwise reviewer panics
}

func (s *panicSpawner) SpawnWorker(ctx context.Context, spec *WorkerSpec) (AgentHandle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workerCalls = append(s.workerCalls, spec)
	if s.panicWorker {
		return &panicHandle{ctx: ctx, msg: s.panicMsg}, nil
	}
	return newMockHandle(ctx), nil
}

func (s *panicSpawner) SpawnReviewer(ctx context.Context, spec *ReviewSpec) (AgentHandle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reviewCalls = append(s.reviewCalls, spec)
	if !s.panicWorker {
		return &panicHandle{ctx: ctx, msg: s.panicMsg}, nil
	}
	return newMockHandle(ctx), nil
}

func TestOrchestratorWorkerPanicRecovery(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	store.CreateMission(ctx, &Mission{
		ID:        "m1",
		Status:    MissionRunning,
		Budget:    Budget{MaxConcurrentWorkers: 2},
		CreatedAt: now,
		UpdatedAt: now,
		StartedAt: &now,
	})
	store.CreateTask(ctx, &Task{
		ID:        "t1",
		MissionID: "m1",
		Title:     "Panicking task",
		Kind:      TaskKindCode,
		Objective: "Trigger a panic",
		Status:    TaskReady,
		Priority:  1,
		RiskLevel: RiskLow,
		CreatedAt: now,
		UpdatedAt: now,
	})

	spawner := &panicSpawner{panicMsg: "nil pointer dereference", panicWorker: true}
	events := make(chan OrchestratorEvent, 100)
	wd := &storeWorkerDispatcher{store: store}
	orch := newTestOrchestratorFull(store, spawner, wd, events)

	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	orch.Start(testCtx)
	defer orch.Stop()

	// Wait for the worker.panic event — proves the goroutine recovered.
	e := waitForEvent(t, events, "worker.panic", 3*time.Second)
	if e.Error == nil || e.Error.Error() != "panic: nil pointer dereference" {
		t.Fatalf("expected panic error, got: %v", e.Error)
	}

	// Active agents should be drained (removeActive ran after recover).
	waitFor(t, 2*time.Second, "active agents drained", func() bool {
		return orch.ActiveRunCount() == 0
	})
}

func TestOrchestratorReviewerPanicRecovery(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	store.CreateMission(ctx, &Mission{
		ID:        "m1",
		Status:    MissionRunning,
		Budget:    Budget{MaxConcurrentWorkers: 2},
		CreatedAt: now,
		UpdatedAt: now,
		StartedAt: &now,
	})
	store.CreateTask(ctx, &Task{
		ID:        "t1",
		MissionID: "m1",
		Title:     "Task with panicking reviewer",
		Kind:      TaskKindCode,
		Objective: "Trigger reviewer panic",
		Status:    TaskReady,
		Priority:  1,
		RiskLevel: RiskLow,
		CreatedAt: now,
		UpdatedAt: now,
	})

	normalSpawner := &mockSpawner{}
	events := make(chan OrchestratorEvent, 100)
	wd := &storeWorkerDispatcher{store: store}
	orch := newTestOrchestratorFull(store, normalSpawner, wd, events)

	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	orch.Start(testCtx)
	defer orch.Stop()

	// Worker completes normally.
	waitFor(t, 2*time.Second, "worker spawn", func() bool {
		return normalSpawner.workerCount() >= 1
	})
	normalSpawner.getWorkerHandle(0).complete("Done", nil)
	waitForEvent(t, events, "worker.completed", 2*time.Second)

	// Now swap spawner to one that panics on review.
	panicSp := &panicSpawner{panicMsg: "index out of range", panicWorker: false}
	orch.spawner = panicSp

	// Wait for the review.panic event.
	e := waitForEvent(t, events, "review.panic", 3*time.Second)
	if e.Error == nil || e.Error.Error() != "panic: index out of range" {
		t.Fatalf("expected panic error, got: %v", e.Error)
	}

	// Active agents should be drained.
	waitFor(t, 2*time.Second, "active agents drained", func() bool {
		return orch.ActiveRunCount() == 0
	})
}

func TestOrchestratorRejectReleasesWorktree(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	store.CreateMission(ctx, &Mission{
		ID:        "m1",
		Status:    MissionRunning,
		Budget:    Budget{MaxConcurrentWorkers: 2},
		CreatedAt: now,
		UpdatedAt: now,
		StartedAt: &now,
	})
	store.CreateTask(ctx, &Task{
		ID:        "t1",
		MissionID: "m1",
		Title:     "Task to reject",
		Kind:      TaskKindCode,
		Objective: "Try something",
		Status:    TaskReady,
		Priority:  1,
		RiskLevel: RiskLow,
		CreatedAt: now,
		UpdatedAt: now,
	})

	spawner := &mockSpawner{}
	events := make(chan OrchestratorEvent, 100)
	orch, wd := newTestOrchestratorWithDispatcher(store, spawner, events)

	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	orch.Start(testCtx)
	defer orch.Stop()

	// Worker completes.
	waitFor(t, 2*time.Second, "worker spawn", func() bool {
		return spawner.workerCount() >= 1
	})
	spawner.getWorkerHandle(0).complete("Done", nil)
	waitForEvent(t, events, "worker.completed", 2*time.Second)

	// Review rejects.
	waitFor(t, 2*time.Second, "review spawn", func() bool {
		return spawner.reviewCount() >= 1
	})
	rejectJSON := "```json\n" +
		`{"verdict":"reject","summary":"Fundamentally wrong approach"}` +
		"\n```"
	spawner.getReviewHandle(0).complete(rejectJSON, nil)
	waitForEvent(t, events, "review.reject", 2*time.Second)

	// Worktree SHOULD be released on reject.
	waitFor(t, 2*time.Second, "worktree released", func() bool {
		return wd.worktreeReleasedFor("t1")
	})
	t.Log("Worktree correctly released on reject")
}
