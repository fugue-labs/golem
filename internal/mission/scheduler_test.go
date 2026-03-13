package mission

import (
	"context"
	"testing"
)

// --- Mock store ---

type mockStore struct {
	missions map[string]*Mission
	tasks    map[string]*Task
	runs     []*Run
	ready    []*Task
}

func newMockStore() *mockStore {
	return &mockStore{
		missions: make(map[string]*Mission),
		tasks:    make(map[string]*Task),
	}
}

func (s *mockStore) GetMission(_ context.Context, id string) (*Mission, error)    { return s.missions[id], nil }
func (s *mockStore) GetReadyTasks(_ context.Context, _ string) ([]*Task, error)   { return s.ready, nil }
func (s *mockStore) ListRuns(_ context.Context, _ string) ([]*Run, error)         { return s.runs, nil }

// Unused Store methods — panic to catch unintended calls.
func (s *mockStore) CreateMission(context.Context, *Mission) error                   { panic("not implemented") }
func (s *mockStore) UpdateMission(context.Context, *Mission) error                   { panic("not implemented") }
func (s *mockStore) ListMissions(context.Context) ([]*Mission, error)                { panic("not implemented") }
func (s *mockStore) CreateTask(context.Context, *Task) error                         { panic("not implemented") }
func (s *mockStore) GetTask(context.Context, string) (*Task, error)                  { panic("not implemented") }
func (s *mockStore) UpdateTask(context.Context, *Task) error                         { panic("not implemented") }
func (s *mockStore) ListTasks(context.Context, string) ([]*Task, error)              { panic("not implemented") }
func (s *mockStore) AddDependency(context.Context, TaskDependency) error             { panic("not implemented") }
func (s *mockStore) ListDependencies(context.Context, string) ([]TaskDependency, error) { panic("not implemented") }
func (s *mockStore) CreateRun(context.Context, *Run) error                           { panic("not implemented") }
func (s *mockStore) GetRun(context.Context, string) (*Run, error)                    { panic("not implemented") }
func (s *mockStore) UpdateRun(context.Context, *Run) error                           { panic("not implemented") }
func (s *mockStore) AppendEvent(context.Context, *Event) error                       { panic("not implemented") }
func (s *mockStore) ListEvents(context.Context, string, int) ([]*Event, error)       { panic("not implemented") }
func (s *mockStore) CreateArtifact(context.Context, *Artifact) error                 { panic("not implemented") }
func (s *mockStore) ListArtifacts(context.Context, string) ([]*Artifact, error)      { panic("not implemented") }
func (s *mockStore) CreateApproval(context.Context, *Approval) error                 { panic("not implemented") }
func (s *mockStore) GetApproval(context.Context, string) (*Approval, error)          { panic("not implemented") }
func (s *mockStore) UpdateApproval(context.Context, *Approval) error                 { panic("not implemented") }
func (s *mockStore) ListApprovals(context.Context, string) ([]*Approval, error)      { panic("not implemented") }
func (s *mockStore) GetMissionSummary(context.Context, string) (*MissionSummary, error) { panic("not implemented") }
func (s *mockStore) Close() error                                                    { return nil }

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
	ms.runs = []*Run{
		{ID: "r1", Status: RunRunning, WorktreePath: "/wt/r1"},
	}

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
	ms.runs = []*Run{
		{ID: "r1", Status: RunRunning, WorktreePath: "/wt/r1"},
	}

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
		{ID: "t2", Priority: 2, Scope: TaskScope{WritePaths: []string{"pkg/api/handler"}}}, // overlaps with t1
		{ID: "t3", Priority: 3, Scope: TaskScope{WritePaths: []string{"pkg/db"}}},
	}

	// A worker is already writing to "pkg/api/routes".
	ms.runs = []*Run{
		{ID: "r1", Status: RunRunning, WorktreePath: "pkg/api/routes"},
	}

	sched := NewScheduler(ms)
	got, err := sched.SelectTasks(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	// t1 conflicts with active run (pkg/api is parent of pkg/api/routes).
	// t2 conflicts with active run (pkg/api/handler is sibling but both under pkg/api? No:
	//    pkg/api/handler vs pkg/api/routes — no overlap).
	// Actually: t1 write="pkg/api", active="pkg/api/routes" => pkg/api is parent => conflict.
	// t2 write="pkg/api/handler", active="pkg/api/routes" => no parent/child => no conflict.
	// But t2 also write="pkg/api/handler" vs already-selected t1 write="pkg/api" => t1 was skipped (conflict),
	// so t2 should be checked against active only.
	//
	// Let me re-check: t1 is skipped because it conflicts with the active run.
	// t2: "pkg/api/handler" vs active "pkg/api/routes" — neither is prefix of other => no conflict. Selected.
	// t3: "pkg/db" vs active "pkg/api/routes" — no conflict. vs selected t2 "pkg/api/handler" — no conflict. Selected.

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
		{ID: "t2", Priority: 2, Scope: TaskScope{WritePaths: []string{"pkg/api/handler"}}}, // child of t1
		{ID: "t3", Priority: 3, Scope: TaskScope{WritePaths: []string{"pkg/db"}}},
	}

	// No active runs.
	ms.runs = nil

	sched := NewScheduler(ms)
	got, err := sched.SelectTasks(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	// t1 selected first (priority 1). t2 conflicts with t1 (pkg/api/handler under pkg/api). t3 is fine.
	ids := taskIDs(got)
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks, got %d: %v", len(got), ids)
	}
	if ids[0] != "t1" || ids[1] != "t3" {
		t.Fatalf("expected [t1 t3], got %v", ids)
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

	ms.runs = nil

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

	ms.runs = nil

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
	ms.runs = []*Run{
		{ID: "r1", Status: RunSucceeded, WorktreePath: "/wt/r1"},
		{ID: "r2", Status: RunFailed, WorktreePath: "/wt/r2"},
		{ID: "r3", Status: RunCancelled, WorktreePath: "/wt/r3"},
	}

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
