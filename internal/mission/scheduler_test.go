package mission

import (
	"context"
	"testing"
)

// --- Mock store ---

type mockStore struct {
	missions    map[string]*Mission
	tasks       map[string]*Task
	deps        []TaskDependency
	runs        map[string]*Run
	events      []*Event
	artifacts   []*Artifact
	approvals   map[string]*Approval
	ready       []*Task
	nextEventID int64
}

func newMockStore() *mockStore {
	return &mockStore{
		missions:  make(map[string]*Mission),
		tasks:     make(map[string]*Task),
		runs:      make(map[string]*Run),
		approvals: make(map[string]*Approval),
	}
}

func (s *mockStore) allTasks() []*Task {
	seen := make(map[string]bool, len(s.tasks)+len(s.ready))
	out := make([]*Task, 0, len(s.tasks)+len(s.ready))
	for _, t := range s.tasks {
		if t == nil {
			continue
		}
		cp := cloneTask(t)
		seen[cp.ID] = true
		out = append(out, cp)
	}
	for _, t := range s.ready {
		if t == nil || seen[t.ID] {
			continue
		}
		out = append(out, cloneTask(t))
	}
	return out
}

func (s *mockStore) CreateMission(_ context.Context, m *Mission) error {
	if _, ok := s.missions[m.ID]; ok {
		return alreadyExistsError("mission", m.ID)
	}
	s.missions[m.ID] = cloneMission(m)
	return nil
}

func (s *mockStore) GetMission(_ context.Context, id string) (*Mission, error) {
	m, ok := s.missions[id]
	if !ok {
		return nil, notFoundError("mission", id)
	}
	return cloneMission(m), nil
}

func (s *mockStore) UpdateMission(_ context.Context, m *Mission) error {
	if _, ok := s.missions[m.ID]; !ok {
		return notFoundError("mission", m.ID)
	}
	s.missions[m.ID] = cloneMission(m)
	return nil
}

func (s *mockStore) ListMissions(context.Context) ([]*Mission, error) {
	out := make([]*Mission, 0, len(s.missions))
	for _, m := range s.missions {
		out = append(out, cloneMission(m))
	}
	sortMissions(out)
	return out, nil
}

func (s *mockStore) CreateTask(_ context.Context, t *Task) error {
	if _, ok := s.tasks[t.ID]; ok {
		return alreadyExistsError("task", t.ID)
	}
	s.tasks[t.ID] = cloneTask(t)
	return nil
}

func (s *mockStore) GetTask(_ context.Context, id string) (*Task, error) {
	t, ok := s.tasks[id]
	if !ok {
		return nil, notFoundError("task", id)
	}
	return cloneTask(t), nil
}

func (s *mockStore) UpdateTask(_ context.Context, t *Task) error {
	if _, ok := s.tasks[t.ID]; !ok {
		return notFoundError("task", t.ID)
	}
	s.tasks[t.ID] = cloneTask(t)
	return nil
}

func (s *mockStore) ListTasks(_ context.Context, missionID string) ([]*Task, error) {
	out := make([]*Task, 0)
	for _, t := range s.allTasks() {
		if t.MissionID == missionID || missionID == "" {
			out = append(out, t)
		}
	}
	sortTasks(out)
	return out, nil
}

func (s *mockStore) AddDependency(_ context.Context, dep TaskDependency) error {
	s.deps = append(s.deps, dep)
	return nil
}

func (s *mockStore) ListDependencies(_ context.Context, missionID string) ([]TaskDependency, error) {
	if missionID == "" {
		out := append([]TaskDependency(nil), s.deps...)
		sortDependencies(out)
		return out, nil
	}
	taskIDs := make(map[string]bool)
	for _, t := range s.allTasks() {
		if t.MissionID == missionID {
			taskIDs[t.ID] = true
		}
	}
	out := make([]TaskDependency, 0, len(s.deps))
	for _, dep := range s.deps {
		if taskIDs[dep.TaskID] || taskIDs[dep.DependsOnID] {
			out = append(out, dep)
		}
	}
	sortDependencies(out)
	return out, nil
}

func (s *mockStore) CreateRun(_ context.Context, r *Run) error {
	if _, ok := s.runs[r.ID]; ok {
		return alreadyExistsError("run", r.ID)
	}
	s.runs[r.ID] = cloneRun(r)
	return nil
}

func (s *mockStore) CreateRunExclusive(_ context.Context, r *Run) (bool, error) {
	for _, existing := range s.runs {
		if existing.TaskID == r.TaskID && existing.Mode == r.Mode && existing.Status == RunRunning {
			return false, nil
		}
	}
	if _, ok := s.runs[r.ID]; ok {
		return false, alreadyExistsError("run", r.ID)
	}
	s.runs[r.ID] = cloneRun(r)
	return true, nil
}

func (s *mockStore) GetRun(_ context.Context, id string) (*Run, error) {
	r, ok := s.runs[id]
	if !ok {
		return nil, notFoundError("run", id)
	}
	return cloneRun(r), nil
}

func (s *mockStore) UpdateRun(_ context.Context, r *Run) error {
	if _, ok := s.runs[r.ID]; !ok {
		return notFoundError("run", r.ID)
	}
	s.runs[r.ID] = cloneRun(r)
	return nil
}

func (s *mockStore) ListRuns(_ context.Context, missionID string) ([]*Run, error) {
	out := make([]*Run, 0, len(s.runs))
	for _, r := range s.runs {
		if r.MissionID == missionID || missionID == "" {
			out = append(out, cloneRun(r))
		}
	}
	sortRuns(out)
	return out, nil
}

func (s *mockStore) AppendEvent(_ context.Context, e *Event) error {
	s.nextEventID++
	cp := cloneEvent(e)
	cp.ID = s.nextEventID
	s.events = append(s.events, cp)
	return nil
}

func (s *mockStore) ListEvents(_ context.Context, missionID string, limit int) ([]*Event, error) {
	out := make([]*Event, 0, len(s.events))
	for _, e := range s.events {
		if e.MissionID == missionID || missionID == "" {
			out = append(out, cloneEvent(e))
		}
	}
	sortEvents(out)
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (s *mockStore) CreateArtifact(_ context.Context, a *Artifact) error {
	s.artifacts = append(s.artifacts, cloneArtifact(a))
	return nil
}

func (s *mockStore) ListArtifacts(_ context.Context, missionID string) ([]*Artifact, error) {
	out := make([]*Artifact, 0, len(s.artifacts))
	for _, a := range s.artifacts {
		if a.MissionID == missionID || missionID == "" {
			out = append(out, cloneArtifact(a))
		}
	}
	sortArtifacts(out)
	return out, nil
}

func (s *mockStore) CreateApproval(_ context.Context, a *Approval) error {
	if _, ok := s.approvals[a.ID]; ok {
		return alreadyExistsError("approval", a.ID)
	}
	s.approvals[a.ID] = cloneApproval(a)
	return nil
}

func (s *mockStore) GetApproval(_ context.Context, id string) (*Approval, error) {
	a, ok := s.approvals[id]
	if !ok {
		return nil, notFoundError("approval", id)
	}
	return cloneApproval(a), nil
}

func (s *mockStore) UpdateApproval(_ context.Context, a *Approval) error {
	if _, ok := s.approvals[a.ID]; !ok {
		return notFoundError("approval", a.ID)
	}
	s.approvals[a.ID] = cloneApproval(a)
	return nil
}

func (s *mockStore) ListApprovals(_ context.Context, missionID string) ([]*Approval, error) {
	out := make([]*Approval, 0, len(s.approvals))
	for _, a := range s.approvals {
		if a.MissionID == missionID || missionID == "" {
			out = append(out, cloneApproval(a))
		}
	}
	sortApprovals(out)
	return out, nil
}

func (s *mockStore) GetMissionSummary(ctx context.Context, missionID string) (*MissionSummary, error) {
	return BuildMissionSummary(ctx, s, missionID)
}

func (s *mockStore) GetReadyTasks(_ context.Context, missionID string) ([]*Task, error) {
	if len(s.ready) > 0 {
		out := make([]*Task, 0, len(s.ready))
		for _, t := range s.ready {
			if t.MissionID == missionID || t.MissionID == "" || missionID == "" {
				out = append(out, cloneTask(t))
			}
		}
		sortTasks(out)
		return out, nil
	}
	return s.GetTasksByStatus(context.Background(), missionID, TaskReady)
}

func (s *mockStore) GetTasksByStatus(_ context.Context, missionID string, status TaskStatus) ([]*Task, error) {
	out := make([]*Task, 0)
	for _, t := range s.allTasks() {
		if (t.MissionID == missionID || missionID == "") && t.Status == status {
			out = append(out, t)
		}
	}
	sortTasks(out)
	return out, nil
}

func (s *mockStore) GetRunsForTask(_ context.Context, taskID string) ([]*Run, error) {
	out := make([]*Run, 0)
	for _, r := range s.runs {
		if r.TaskID == taskID {
			out = append(out, cloneRun(r))
		}
	}
	sortRuns(out)
	return out, nil
}

func (s *mockStore) Close() error { return nil }

// --- Tests ---

func TestSelectTasks_NoReady(t *testing.T) {
	ms := newMockStore()
	ms.missions["m1"] = &Mission{ID: "m1", Budget: Budget{MaxConcurrentWorkers: 4}}
	ms.ready = nil

	sched := NewScheduler(ms)
	got, err := sched.SelectTasks(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(got))
	}
}

func TestSelectTasks_ConcurrencyLimit(t *testing.T) {
	ms := newMockStore()
	ms.missions["m1"] = &Mission{ID: "m1", Budget: Budget{MaxConcurrentWorkers: 2}}

	ms.ready = []*Task{
		{ID: "t1", Priority: 1, Scope: TaskScope{WritePaths: []string{"pkg/a"}}},
		{ID: "t2", Priority: 2, Scope: TaskScope{WritePaths: []string{"pkg/b"}}},
		{ID: "t3", Priority: 3, Scope: TaskScope{WritePaths: []string{"pkg/c"}}},
	}

	// One worker already running.
	ms.runs["r1"] = &Run{ID: "r1", MissionID: "m1", Status: RunRunning, WorktreePath: "/wt/r1"}

	sched := NewScheduler(ms)
	got, err := sched.SelectTasks(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	// MaxConcurrentWorkers=2, 1 running => 1 slot available.
	if len(got) != 1 {
		t.Fatalf("expected 1 task, got %d", len(got))
	}
	if got[0].ID != "t1" {
		t.Fatalf("expected t1 (highest priority), got %s", got[0].ID)
	}
}

func TestSelectTasks_ConcurrencyFullyUsed(t *testing.T) {
	ms := newMockStore()
	ms.missions["m1"] = &Mission{ID: "m1", Budget: Budget{MaxConcurrentWorkers: 1}}

	ms.ready = []*Task{
		{ID: "t1", Priority: 1, Scope: TaskScope{WritePaths: []string{"pkg/a"}}},
	}

	// One worker already running => 0 slots.
	ms.runs["r1"] = &Run{ID: "r1", MissionID: "m1", Status: RunRunning, WorktreePath: "/wt/r1"}

	sched := NewScheduler(ms)
	got, err := sched.SelectTasks(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 tasks (concurrency full), got %d", len(got))
	}
}

func TestSelectTasks_WritePathConflict(t *testing.T) {
	ms := newMockStore()
	ms.missions["m1"] = &Mission{ID: "m1", Budget: Budget{MaxConcurrentWorkers: 4}}

	ms.ready = []*Task{
		{ID: "t1", Priority: 1, Scope: TaskScope{WritePaths: []string{"pkg/api"}}},
		{ID: "t2", Priority: 2, Scope: TaskScope{WritePaths: []string{"pkg/api/handler"}}},
		{ID: "t3", Priority: 3, Scope: TaskScope{WritePaths: []string{"pkg/db"}}},
	}

	// A worker is already writing to "pkg/api/routes".
	ms.runs["r1"] = &Run{ID: "r1", MissionID: "m1", Status: RunRunning, WorktreePath: "pkg/api/routes"}

	sched := NewScheduler(ms)
	got, err := sched.SelectTasks(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	ids := taskIDs(got)
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks, got %d: %v", len(got), ids)
	}
	if ids[0] != "t2" || ids[1] != "t3" {
		t.Fatalf("expected [t2 t3], got %v", ids)
	}
}

func TestSelectTasks_IntraBatchWriteConflict(t *testing.T) {
	ms := newMockStore()
	ms.missions["m1"] = &Mission{ID: "m1", Budget: Budget{MaxConcurrentWorkers: 4}}

	ms.ready = []*Task{
		{ID: "t1", Priority: 1, Scope: TaskScope{WritePaths: []string{"pkg/api"}}},
		{ID: "t2", Priority: 2, Scope: TaskScope{WritePaths: []string{"pkg/api/handler"}}},
		{ID: "t3", Priority: 3, Scope: TaskScope{WritePaths: []string{"pkg/db"}}},
	}

	sched := NewScheduler(ms)
	got, err := sched.SelectTasks(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	ids := taskIDs(got)
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks, got %d: %v", len(got), ids)
	}
	if ids[0] != "t1" || ids[1] != "t3" {
		t.Fatalf("expected [t1 t3], got %v", ids)
	}
}

func TestSelectTasks_PathNormalizationConflict(t *testing.T) {
	ms := newMockStore()
	ms.missions["m1"] = &Mission{ID: "m1", Budget: Budget{MaxConcurrentWorkers: 3}}
	ms.ready = []*Task{
		{ID: "t1", Priority: 1, Scope: TaskScope{WritePaths: []string{"pkg/api/"}}},
		{ID: "t2", Priority: 2, Scope: TaskScope{WritePaths: []string{"pkg/worker"}}},
	}
	ms.runs["r1"] = &Run{ID: "r1", MissionID: "m1", Status: RunRunning, WorktreePath: "pkg/api"}

	got, err := NewScheduler(ms).SelectTasks(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	ids := taskIDs(got)
	if len(ids) != 1 || ids[0] != "t2" {
		t.Fatalf("expected normalized conflict to skip t1, got %v", ids)
	}
}

func TestSelectTasks_PriorityOrdering(t *testing.T) {
	ms := newMockStore()
	ms.missions["m1"] = &Mission{ID: "m1", Budget: Budget{MaxConcurrentWorkers: 10}}

	ms.ready = []*Task{
		{ID: "t3", Priority: 30, Scope: TaskScope{WritePaths: []string{"pkg/c"}}},
		{ID: "t1", Priority: 10, Scope: TaskScope{WritePaths: []string{"pkg/a"}}},
		{ID: "t2", Priority: 20, Scope: TaskScope{WritePaths: []string{"pkg/b"}}},
	}

	sched := NewScheduler(ms)
	got, err := sched.SelectTasks(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(got))
	}
	ids := taskIDs(got)
	if ids[0] != "t1" || ids[1] != "t2" || ids[2] != "t3" {
		t.Fatalf("expected [t1 t2 t3] (priority order), got %v", ids)
	}
}

func TestSelectTasks_DefaultConcurrency(t *testing.T) {
	ms := newMockStore()
	// MaxConcurrentWorkers=0 should default to 1.
	ms.missions["m1"] = &Mission{ID: "m1", Budget: Budget{}}

	ms.ready = []*Task{
		{ID: "t1", Priority: 1, Scope: TaskScope{WritePaths: []string{"pkg/a"}}},
		{ID: "t2", Priority: 2, Scope: TaskScope{WritePaths: []string{"pkg/b"}}},
	}

	sched := NewScheduler(ms)
	got, err := sched.SelectTasks(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 task (default concurrency=1), got %d", len(got))
	}
	if got[0].ID != "t1" {
		t.Fatalf("expected t1, got %s", got[0].ID)
	}
}

func TestSelectTasks_FinishedRunsNotCounted(t *testing.T) {
	ms := newMockStore()
	ms.missions["m1"] = &Mission{ID: "m1", Budget: Budget{MaxConcurrentWorkers: 2}}

	ms.ready = []*Task{
		{ID: "t1", Priority: 1, Scope: TaskScope{WritePaths: []string{"pkg/a"}}},
		{ID: "t2", Priority: 2, Scope: TaskScope{WritePaths: []string{"pkg/b"}}},
	}

	// Finished runs should not count against concurrency.
	ms.runs["r1"] = &Run{ID: "r1", MissionID: "m1", Status: RunSucceeded, WorktreePath: "/wt/r1"}
	ms.runs["r2"] = &Run{ID: "r2", MissionID: "m1", Status: RunFailed, WorktreePath: "/wt/r2"}
	ms.runs["r3"] = &Run{ID: "r3", MissionID: "m1", Status: RunCancelled, WorktreePath: "/wt/r3"}

	sched := NewScheduler(ms)
	got, err := sched.SelectTasks(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 tasks (finished runs don't count), got %d", len(got))
	}
}

// --- Helpers ---

func taskIDs(tasks []*Task) []string {
	ids := make([]string, len(tasks))
	for i, t := range tasks {
		ids[i] = t.ID
	}
	return ids
}
