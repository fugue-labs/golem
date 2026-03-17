package mission

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// memoryStore — full in-memory implementation of Store for integration tests.
// ---------------------------------------------------------------------------

type memoryStore struct {
	mu          sync.Mutex
	missions    map[string]*Mission
	tasks       map[string]*Task
	deps        []TaskDependency
	runs        map[string]*Run
	events      []*Event
	artifacts   []*Artifact
	approvals   map[string]*Approval
	nextEventID int64
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		missions:  make(map[string]*Mission),
		tasks:     make(map[string]*Task),
		runs:      make(map[string]*Run),
		approvals: make(map[string]*Approval),
	}
}

// --- Mission CRUD ---

func (s *memoryStore) CreateMission(_ context.Context, m *Mission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.missions[m.ID]; ok {
		return fmt.Errorf("mission %s already exists", m.ID)
	}
	cp := *m
	s.missions[m.ID] = &cp
	return nil
}

func (s *memoryStore) GetMission(_ context.Context, id string) (*Mission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.missions[id]
	if !ok {
		return nil, fmt.Errorf("mission %s not found", id)
	}
	cp := *m
	return &cp, nil
}

func (s *memoryStore) UpdateMission(_ context.Context, m *Mission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.missions[m.ID]; !ok {
		return fmt.Errorf("mission %s not found", m.ID)
	}
	cp := *m
	s.missions[m.ID] = &cp
	return nil
}

func (s *memoryStore) ListMissions(_ context.Context) ([]*Mission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Mission, 0, len(s.missions))
	for _, m := range s.missions {
		cp := *m
		out = append(out, &cp)
	}
	return out, nil
}

// --- Task CRUD ---

func (s *memoryStore) CreateTask(_ context.Context, t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[t.ID]; ok {
		return fmt.Errorf("task %s already exists", t.ID)
	}
	cp := *t
	s.tasks[t.ID] = &cp
	return nil
}

func (s *memoryStore) GetTask(_ context.Context, id string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %s not found", id)
	}
	cp := *t
	return &cp, nil
}

func (s *memoryStore) UpdateTask(_ context.Context, t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[t.ID]; !ok {
		return fmt.Errorf("task %s not found", t.ID)
	}
	cp := *t
	s.tasks[t.ID] = &cp
	return nil
}

func (s *memoryStore) ListTasks(_ context.Context, missionID string) ([]*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Task
	for _, t := range s.tasks {
		if t.MissionID == missionID {
			cp := *t
			out = append(out, &cp)
		}
	}
	return out, nil
}

// --- Dependencies ---

func (s *memoryStore) AddDependency(_ context.Context, dep TaskDependency) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deps = append(s.deps, dep)
	return nil
}

func (s *memoryStore) ListDependencies(_ context.Context, missionID string) ([]TaskDependency, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Return all deps where task belongs to this mission.
	taskIDs := make(map[string]bool)
	for _, t := range s.tasks {
		if t.MissionID == missionID {
			taskIDs[t.ID] = true
		}
	}
	var out []TaskDependency
	for _, d := range s.deps {
		if taskIDs[d.TaskID] || taskIDs[d.DependsOnID] {
			out = append(out, d)
		}
	}
	return out, nil
}

// --- Runs ---

func (s *memoryStore) CreateRun(_ context.Context, r *Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[r.ID]; ok {
		return fmt.Errorf("run %s already exists", r.ID)
	}
	cp := *r
	s.runs[r.ID] = &cp
	return nil
}

func (s *memoryStore) CreateRunExclusive(_ context.Context, r *Run) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.runs {
		if existing.TaskID == r.TaskID && existing.Mode == r.Mode && existing.Status == RunRunning {
			return false, nil
		}
	}
	if _, ok := s.runs[r.ID]; ok {
		return false, fmt.Errorf("run %s already exists", r.ID)
	}
	cp := *r
	s.runs[r.ID] = &cp
	return true, nil
}

func (s *memoryStore) GetRun(_ context.Context, id string) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return nil, fmt.Errorf("run %s not found", id)
	}
	cp := *r
	return &cp, nil
}

func (s *memoryStore) UpdateRun(_ context.Context, r *Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[r.ID]; !ok {
		return fmt.Errorf("run %s not found", r.ID)
	}
	cp := *r
	s.runs[r.ID] = &cp
	return nil
}

func (s *memoryStore) ListRuns(_ context.Context, missionID string) ([]*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Run
	for _, r := range s.runs {
		if r.MissionID == missionID {
			cp := *r
			out = append(out, &cp)
		}
	}
	return out, nil
}

// --- Events ---

func (s *memoryStore) AppendEvent(_ context.Context, e *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextEventID++
	e.ID = s.nextEventID
	cp := *e
	s.events = append(s.events, &cp)
	return nil
}

func (s *memoryStore) ListEvents(_ context.Context, missionID string, limit int) ([]*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Event
	for _, e := range s.events {
		if e.MissionID == missionID {
			cp := *e
			out = append(out, &cp)
		}
	}
	// Return most recent first, up to limit.
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

// --- Artifacts ---

func (s *memoryStore) CreateArtifact(_ context.Context, a *Artifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *a
	s.artifacts = append(s.artifacts, &cp)
	return nil
}

func (s *memoryStore) ListArtifacts(_ context.Context, missionID string) ([]*Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Artifact
	for _, a := range s.artifacts {
		if a.MissionID == missionID {
			cp := *a
			out = append(out, &cp)
		}
	}
	return out, nil
}

// --- Approvals ---

func (s *memoryStore) CreateApproval(_ context.Context, a *Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.approvals[a.ID]; ok {
		return fmt.Errorf("approval %s already exists", a.ID)
	}
	cp := *a
	s.approvals[a.ID] = &cp
	return nil
}

func (s *memoryStore) GetApproval(_ context.Context, id string) (*Approval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.approvals[id]
	if !ok {
		return nil, fmt.Errorf("approval %s not found", id)
	}
	cp := *a
	return &cp, nil
}

func (s *memoryStore) UpdateApproval(_ context.Context, a *Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.approvals[a.ID]; !ok {
		return fmt.Errorf("approval %s not found", a.ID)
	}
	cp := *a
	s.approvals[a.ID] = &cp
	return nil
}

func (s *memoryStore) ListApprovals(_ context.Context, missionID string) ([]*Approval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Approval
	for _, a := range s.approvals {
		if a.MissionID == missionID {
			cp := *a
			out = append(out, &cp)
		}
	}
	return out, nil
}

// --- Aggregate queries ---

func (s *memoryStore) GetMissionSummary(ctx context.Context, missionID string) (*MissionSummary, error) {
	s.mu.Lock()
	m, ok := s.missions[missionID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("mission %s not found", missionID)
	}
	mCopy := *m

	var counts TaskCounts
	for _, t := range s.tasks {
		if t.MissionID != missionID {
			continue
		}
		counts.Total++
		switch t.Status {
		case TaskPending:
			counts.Pending++
		case TaskReady:
			counts.Ready++
		case TaskRunning, TaskLeased:
			counts.Running++
		case TaskAwaitingReview:
			counts.AwaitingReview++
		case TaskAccepted:
			counts.Accepted++
		case TaskIntegrated:
			counts.Integrated++
		case TaskDone:
			counts.Done++
		case TaskBlocked:
			counts.Blocked++
		case TaskFailed, TaskRejected:
			counts.Failed++
		}
	}

	var activeRuns int
	for _, r := range s.runs {
		if r.MissionID == missionID && r.Status == RunRunning {
			activeRuns++
		}
	}

	var pendingApprovals int
	for _, a := range s.approvals {
		if a.MissionID == missionID && a.Status == ApprovalPending {
			pendingApprovals++
		}
	}
	s.mu.Unlock()

	return &MissionSummary{
		Mission:          &mCopy,
		TaskCounts:       counts,
		ActiveRuns:       activeRuns,
		PendingApprovals: pendingApprovals,
	}, nil
}

func (s *memoryStore) GetReadyTasks(_ context.Context, missionID string) ([]*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Task
	for _, t := range s.tasks {
		if t.MissionID == missionID && t.Status == TaskReady {
			cp := *t
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (s *memoryStore) GetTasksByStatus(_ context.Context, missionID string, status TaskStatus) ([]*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Task
	for _, t := range s.tasks {
		if t.MissionID == missionID && t.Status == status {
			cp := *t
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (s *memoryStore) GetRunsForTask(_ context.Context, taskID string) ([]*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Run
	for _, r := range s.runs {
		if r.TaskID == taskID {
			cp := *r
			out = append(out, &cp)
		}
	}
	return out, nil
}

// --- Lifecycle ---

func (s *memoryStore) Close() error { return nil }

// ---------------------------------------------------------------------------
// TestMissionLifecycle — exercises the complete mission lifecycle end-to-end
// without Dolt, git, or any external dependencies.
// ---------------------------------------------------------------------------

func TestMissionLifecycle(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	ctrl := NewController(store)
	sched := NewScheduler(store)

	// -----------------------------------------------------------------------
	// 1. Create mission → draft
	// -----------------------------------------------------------------------
	mission, err := ctrl.CreateMission(ctx, CreateMissionRequest{
		Title:      "Add widget feature",
		Goal:       "Implement widget support with tests",
		RepoRoot:   "/tmp/fake-repo",
		BaseCommit: "abc123",
		BaseBranch: "main",
		Budget:     Budget{MaxConcurrentWorkers: 4},
	})
	if err != nil {
		t.Fatalf("CreateMission: %v", err)
	}
	if mission.Status != MissionDraft {
		t.Fatalf("expected draft, got %s", mission.Status)
	}
	t.Logf("Step 1: mission %s created in %s", mission.ID, mission.Status)

	// -----------------------------------------------------------------------
	// 2. Apply plan → awaiting_approval (creates tasks + deps, resolves ready)
	// -----------------------------------------------------------------------
	plan := &PlanResult{
		Summary:         "Two-task plan: implement widget, then add tests",
		SuccessCriteria: []string{"widget works", "tests pass"},
		Tasks: []PlanTask{
			{
				ID:                 "task-impl",
				Title:              "Implement widget",
				Kind:               TaskKindCode,
				Objective:          "Create the widget package",
				Priority:           1,
				Scope:              TaskScope{WritePaths: []string{"pkg/widget"}},
				AcceptanceCriteria: []string{"widget.New() returns valid widget"},
				EstimatedEffort:    "medium",
				RiskLevel:          RiskLow,
			},
			{
				ID:                 "task-test",
				Title:              "Add widget tests",
				Kind:               TaskKindTest,
				Objective:          "Write unit tests for widget",
				Priority:           2,
				Scope:              TaskScope{WritePaths: []string{"pkg/widget"}},
				AcceptanceCriteria: []string{"90% coverage"},
				EstimatedEffort:    "small",
				RiskLevel:          RiskLow,
			},
		},
		Dependencies: []TaskDependency{
			{TaskID: "task-test", DependsOnID: "task-impl"},
		},
	}

	if err := ctrl.ApplyPlan(ctx, mission.ID, plan); err != nil {
		t.Fatalf("ApplyPlan: %v", err)
	}

	// Refetch mission.
	mission, err = ctrl.GetMission(ctx, mission.ID)
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if mission.Status != MissionAwaitingApproval {
		t.Fatalf("expected awaiting_approval, got %s", mission.Status)
	}

	// Look up tasks by title since ApplyPlan generates unique IDs.
	allTasks, err := store.ListTasks(ctx, mission.ID)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	taskByTitle := make(map[string]*Task)
	for _, tk := range allTasks {
		taskByTitle[tk.Title] = tk
	}
	implTask := taskByTitle["Implement widget"]
	testTask := taskByTitle["Add widget tests"]
	if implTask == nil || testTask == nil {
		t.Fatalf("expected tasks by title, got %v", taskByTitle)
	}
	implTaskID := implTask.ID
	testTaskID := testTask.ID

	if implTask.Status != TaskReady {
		t.Fatalf("expected impl task ready, got %s", implTask.Status)
	}
	if testTask.Status != TaskPending {
		t.Fatalf("expected test task pending, got %s", testTask.Status)
	}
	t.Logf("Step 2: plan applied, mission %s, impl=%s(%s), test=%s(%s)",
		mission.Status, implTaskID, implTask.Status, testTaskID, testTask.Status)

	// -----------------------------------------------------------------------
	// 3. Start mission → running
	// -----------------------------------------------------------------------
	// Approve the durable mission-plan gate before starting.
	if err := ctrl.ApproveMission(ctx, mission.ID); err != nil {
		t.Fatalf("ApproveMission: %v", err)
	}
	if err := ctrl.StartMission(ctx, mission.ID); err != nil {
		t.Fatalf("StartMission: %v", err)
	}
	mission, _ = ctrl.GetMission(ctx, mission.ID)
	if mission.Status != MissionRunning {
		t.Fatalf("expected running, got %s", mission.Status)
	}
	t.Logf("Step 3: mission started → %s", mission.Status)

	// -----------------------------------------------------------------------
	// 4. Scheduler selects ready tasks
	// -----------------------------------------------------------------------
	selected, err := sched.SelectTasks(ctx, mission.ID)
	if err != nil {
		t.Fatalf("SelectTasks: %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected 1 ready task, got %d", len(selected))
	}
	if selected[0].ID != implTaskID {
		t.Fatalf("expected %s, got %s", implTaskID, selected[0].ID)
	}
	t.Logf("Step 4: scheduler selected %s", selected[0].ID)

	// -----------------------------------------------------------------------
	// 5. Simulate worker: create Run, update task to running, then
	//    use WorkerLauncher.CompleteWorker → task to awaiting_review
	// -----------------------------------------------------------------------
	now := time.Now().UTC()
	workerRun := &Run{
		ID:           generateID("r"),
		MissionID:    mission.ID,
		TaskID:       implTaskID,
		Mode:         RunModeWorker,
		Status:       RunRunning,
		LeaseOwner:   implTaskID,
		WorktreePath: "/tmp/fake-worktree/" + implTaskID,
		StartedAt:    &now,
	}
	if err := store.CreateRun(ctx, workerRun); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// Transition task to running (simulating what DispatchReadyTasks does).
	implTask, _ = store.GetTask(ctx, implTaskID)
	implTask.Status = TaskRunning
	implTask.AttemptCount = 1
	implTask.UpdatedAt = now
	if err := store.UpdateTask(ctx, implTask); err != nil {
		t.Fatalf("UpdateTask to running: %v", err)
	}

	// Build a WorkerSpec and call CompleteWorker.
	workerSpec := &WorkerSpec{
		Run:          workerRun,
		Task:         implTask,
		WorktreePath: workerRun.WorktreePath,
		Prompt:       "test prompt",
	}

	// WorkerLauncher needs a scheduler and worktree manager, but
	// CompleteWorker only uses the store — pass nil for worktrees.
	wl := NewWorkerLauncher(sched, nil, store)
	if err := wl.CompleteWorker(ctx, workerSpec, "Implemented widget package"); err != nil {
		t.Fatalf("CompleteWorker: %v", err)
	}

	// Verify task is now awaiting_review.
	implTask, _ = store.GetTask(ctx, implTaskID)
	if implTask.Status != TaskAwaitingReview {
		t.Fatalf("expected impl task awaiting_review, got %s", implTask.Status)
	}
	// Verify run is succeeded.
	workerRun, _ = store.GetRun(ctx, workerRun.ID)
	if workerRun.Status != RunSucceeded {
		t.Fatalf("expected run succeeded, got %s", workerRun.Status)
	}
	t.Logf("Step 5: worker completed, impl=%s, run=%s",
		implTask.Status, workerRun.Status)

	// -----------------------------------------------------------------------
	// 6. Review: CompleteReview with ReviewPass → task to accepted
	// -----------------------------------------------------------------------
	rl := NewReviewLauncher(store, false)

	// Manually create a review run (bypassing DispatchPendingReviews which
	// needs git diff).
	reviewRun := &Run{
		ID:        generateID("r"),
		MissionID: mission.ID,
		TaskID:    implTaskID,
		Mode:      RunModeReview,
		Status:    RunRunning,
		StartedAt: &now,
	}
	if err := store.CreateRun(ctx, reviewRun); err != nil {
		t.Fatalf("CreateRun (review): %v", err)
	}

	reviewSpec := &ReviewSpec{
		Run:       reviewRun,
		Task:      implTask,
		WorkerRun: workerRun,
		Prompt:    "review prompt",
		DiffText:  "+widget code",
	}

	reviewResult := &ReviewResult{
		Verdict: ReviewPass,
		Summary: "LGTM - widget looks good",
	}

	if err := rl.CompleteReview(ctx, reviewSpec, reviewResult); err != nil {
		t.Fatalf("CompleteReview: %v", err)
	}

	// Verify task is now accepted.
	implTask, _ = store.GetTask(ctx, implTaskID)
	if implTask.Status != TaskAccepted {
		t.Fatalf("expected impl task accepted, got %s", implTask.Status)
	}
	t.Logf("Step 6: review passed, impl=%s", implTask.Status)

	// -----------------------------------------------------------------------
	// 7. Integration: set task to integrated, resolve dependent tasks
	// -----------------------------------------------------------------------
	// In real flow IntegrateTask does the git merge + status transition.
	// We simulate by directly setting the status and calling resolveReadyTasks.
	implTask.Status = TaskIntegrated
	implTask.UpdatedAt = time.Now().UTC()
	if err := store.UpdateTask(ctx, implTask); err != nil {
		t.Fatalf("UpdateTask to integrated: %v", err)
	}

	// Call the controller's resolveReadyTasks to unlock dependents.
	if err := ctrl.resolveReadyTasks(ctx, mission.ID); err != nil {
		t.Fatalf("resolveReadyTasks: %v", err)
	}

	// Verify test task is now ready.
	testTask, _ = store.GetTask(ctx, testTaskID)
	if testTask.Status != TaskReady {
		t.Fatalf("expected test task ready after dependency resolved, got %s", testTask.Status)
	}
	t.Logf("Step 7: impl integrated, test=%s (dependency resolved)", testTask.Status)

	// -----------------------------------------------------------------------
	// 8. Repeat steps 4-7 for the dependent task (task-test)
	// -----------------------------------------------------------------------

	// 8a. Scheduler picks task-test.
	selected, err = sched.SelectTasks(ctx, mission.ID)
	if err != nil {
		t.Fatalf("SelectTasks (round 2): %v", err)
	}
	if len(selected) != 1 || selected[0].ID != testTaskID {
		var ids []string
		for _, s := range selected {
			ids = append(ids, s.ID)
		}
		t.Fatalf("expected [%s], got %v", testTaskID, ids)
	}
	t.Logf("Step 8a: scheduler selected %s", selected[0].ID)

	// 8b. Simulate worker for task-test.
	now2 := time.Now().UTC()
	workerRun2 := &Run{
		ID:           generateID("r"),
		MissionID:    mission.ID,
		TaskID:       testTaskID,
		Mode:         RunModeWorker,
		Status:       RunRunning,
		LeaseOwner:   testTaskID,
		WorktreePath: "/tmp/fake-worktree/" + testTaskID,
		StartedAt:    &now2,
	}
	if err := store.CreateRun(ctx, workerRun2); err != nil {
		t.Fatalf("CreateRun (worker2): %v", err)
	}

	testTask, _ = store.GetTask(ctx, testTaskID)
	testTask.Status = TaskRunning
	testTask.AttemptCount = 1
	testTask.UpdatedAt = now2
	if err := store.UpdateTask(ctx, testTask); err != nil {
		t.Fatalf("UpdateTask test task to running: %v", err)
	}

	workerSpec2 := &WorkerSpec{
		Run:          workerRun2,
		Task:         testTask,
		WorktreePath: workerRun2.WorktreePath,
		Prompt:       "test prompt 2",
	}

	if err := wl.CompleteWorker(ctx, workerSpec2, "Added comprehensive widget tests"); err != nil {
		t.Fatalf("CompleteWorker (task-test): %v", err)
	}

	testTask, _ = store.GetTask(ctx, testTaskID)
	if testTask.Status != TaskAwaitingReview {
		t.Fatalf("expected test task awaiting_review, got %s", testTask.Status)
	}
	t.Logf("Step 8b: worker completed test task → %s", testTask.Status)

	// 8c. Review task-test.
	reviewRun2 := &Run{
		ID:        generateID("r"),
		MissionID: mission.ID,
		TaskID:    testTaskID,
		Mode:      RunModeReview,
		Status:    RunRunning,
		StartedAt: &now2,
	}
	if err := store.CreateRun(ctx, reviewRun2); err != nil {
		t.Fatalf("CreateRun (review2): %v", err)
	}

	workerRun2, _ = store.GetRun(ctx, workerRun2.ID)
	reviewSpec2 := &ReviewSpec{
		Run:       reviewRun2,
		Task:      testTask,
		WorkerRun: workerRun2,
		Prompt:    "review prompt 2",
		DiffText:  "+test code",
	}

	if err := rl.CompleteReview(ctx, reviewSpec2, &ReviewResult{
		Verdict: ReviewPass,
		Summary: "Tests look solid",
	}); err != nil {
		t.Fatalf("CompleteReview (task-test): %v", err)
	}

	testTask, _ = store.GetTask(ctx, testTaskID)
	if testTask.Status != TaskAccepted {
		t.Fatalf("expected test task accepted, got %s", testTask.Status)
	}
	t.Logf("Step 8c: review passed, test=%s", testTask.Status)

	// 8d. Integrate task-test.
	testTask.Status = TaskIntegrated
	testTask.UpdatedAt = time.Now().UTC()
	if err := store.UpdateTask(ctx, testTask); err != nil {
		t.Fatalf("UpdateTask task-test to integrated: %v", err)
	}
	t.Logf("Step 8d: task-test integrated")

	// -----------------------------------------------------------------------
	// 9. IntegrationEngine.CheckMissionComplete → true
	// -----------------------------------------------------------------------
	ie := NewIntegrationEngine(store, "/tmp/fake-repo")

	complete, err := ie.CheckMissionComplete(ctx, mission.ID)
	if err != nil {
		t.Fatalf("CheckMissionComplete: %v", err)
	}
	if !complete {
		// Debug: list all tasks and their statuses.
		allTasks, _ := store.ListTasks(ctx, mission.ID)
		for _, task := range allTasks {
			t.Logf("  task %s: %s", task.ID, task.Status)
		}
		t.Fatalf("expected mission complete, got false")
	}
	t.Logf("Step 9: CheckMissionComplete = true")

	// -----------------------------------------------------------------------
	// 10. IntegrationEngine.CompleteMission → mission completed
	// -----------------------------------------------------------------------
	if err := ie.CompleteMission(ctx, mission.ID); err != nil {
		t.Fatalf("CompleteMission: %v", err)
	}

	mission, _ = ctrl.GetMission(ctx, mission.ID)
	if mission.Status != MissionCompleted {
		t.Fatalf("expected mission completed, got %s", mission.Status)
	}
	if mission.EndedAt == nil {
		t.Fatal("expected EndedAt to be set")
	}
	t.Logf("Step 10: mission %s → %s (ended at %s)",
		mission.ID, mission.Status, mission.EndedAt.Format(time.RFC3339))

	// -----------------------------------------------------------------------
	// Verify final aggregate state via GetMissionSummary.
	// -----------------------------------------------------------------------
	summary, err := store.GetMissionSummary(ctx, mission.ID)
	if err != nil {
		t.Fatalf("GetMissionSummary: %v", err)
	}
	if summary.TaskCounts.Total != 2 {
		t.Fatalf("expected 2 total tasks, got %d", summary.TaskCounts.Total)
	}
	if summary.TaskCounts.Integrated != 2 {
		t.Fatalf("expected 2 integrated tasks, got %d", summary.TaskCounts.Integrated)
	}
	if summary.ActiveRuns != 0 {
		t.Fatalf("expected 0 active runs, got %d", summary.ActiveRuns)
	}
	t.Logf("Final summary: %d tasks total, %d integrated, %d active runs",
		summary.TaskCounts.Total, summary.TaskCounts.Integrated, summary.ActiveRuns)

	// -----------------------------------------------------------------------
	// 11. Pause and restart a pre-existing mission after reopening.
	// -----------------------------------------------------------------------
	reopenedCtrl := NewController(store)
	reattach, err := reopenedCtrl.GetMission(ctx, mission.ID)
	if err != nil {
		t.Fatalf("GetMission for reattach: %v", err)
	}
	if reattach.Status != MissionCompleted {
		t.Fatalf("expected completed mission before reset, got %s", reattach.Status)
	}
	reatachReadyTask := &Task{
		ID:        "t_reattach_ready",
		MissionID: mission.ID,
		Title:     "Reconnect reopened worker lane",
		Kind:      TaskKindCode,
		Status:    TaskReady,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.CreateTask(ctx, reatachReadyTask); err != nil {
		t.Fatalf("CreateTask reattach ready task: %v", err)
	}
	reattach.UpdatedAt = time.Now().UTC()
	reattach.EndedAt = nil
	reattach.Status = MissionRunning
	if err := store.UpdateMission(ctx, reattach); err != nil {
		t.Fatalf("UpdateMission for reattach: %v", err)
	}

	reopenedPauseCtrl := NewController(store)
	if err := reopenedPauseCtrl.PauseMission(ctx, mission.ID); err != nil {
		t.Fatalf("PauseMission on reattached mission: %v", err)
	}
	pausedMission, err := reopenedPauseCtrl.GetMission(ctx, mission.ID)
	if err != nil {
		t.Fatalf("GetMission after pause: %v", err)
	}
	if pausedMission.Status != MissionPaused {
		t.Fatalf("mission status after pause = %s, want %s", pausedMission.Status, MissionPaused)
	}
	pausedSummary, err := reopenedPauseCtrl.GetMissionSummary(ctx, mission.ID)
	if err != nil {
		t.Fatalf("GetMissionSummary after pause: %v", err)
	}
	if pausedSummary.PhaseLabel != "Paused" {
		t.Fatalf("paused phase label = %q", pausedSummary.PhaseLabel)
	}
	if pausedSummary.NextAction != "Resume mission execution with /mission start" {
		t.Fatalf("paused next action = %q", pausedSummary.NextAction)
	}
	if pausedSummary.FocusTask == nil || pausedSummary.FocusTask.ID != reatachReadyTask.ID {
		t.Fatalf("paused focus task = %#v, want %s", pausedSummary.FocusTask, reatachReadyTask.ID)
	}

	reopenedStartCtrl := NewController(store)
	if err := reopenedStartCtrl.StartMission(ctx, mission.ID); err != nil {
		t.Fatalf("StartMission from paused reattach: %v", err)
	}
	resumedMission, err := reopenedStartCtrl.GetMission(ctx, mission.ID)
	if err != nil {
		t.Fatalf("GetMission after restart: %v", err)
	}
	if resumedMission.Status != MissionRunning {
		t.Fatalf("mission status after restart = %s, want %s", resumedMission.Status, MissionRunning)
	}
	resumedSummary, err := reopenedStartCtrl.GetMissionSummary(ctx, mission.ID)
	if err != nil {
		t.Fatalf("GetMissionSummary after restart: %v", err)
	}
	if resumedSummary.PhaseLabel != "Running · ready queue" {
		t.Fatalf("resumed phase label = %q", resumedSummary.PhaseLabel)
	}
	if resumedSummary.NextAction != "Next ready task: Reconnect reopened worker lane" {
		t.Fatalf("resumed next action = %q", resumedSummary.NextAction)
	}

	// Verify events were recorded throughout the lifecycle.
	events, err := store.ListEvents(ctx, mission.ID, 100)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected events to be recorded")
	}
	t.Logf("Lifecycle recorded %d events", len(events))
}
