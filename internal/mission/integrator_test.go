package mission

import (
	"context"
	"strings"
	"testing"
)

// integratorMockStore extends reviewMockStore with methods for IntegrationEngine.
type integratorMockStore struct {
	reviewMockStore
	deps []TaskDependency
}

func newIntegratorMockStore() *integratorMockStore {
	return &integratorMockStore{
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
	}
}

func (s *integratorMockStore) ListTasks(_ context.Context, _ string) ([]*Task, error) {
	var tasks []*Task
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *integratorMockStore) ListDependencies(_ context.Context, _ string) ([]TaskDependency, error) {
	return s.deps, nil
}

func (s *integratorMockStore) UpdateMission(_ context.Context, m *Mission) error {
	s.missions[m.ID] = m
	return nil
}

// --- Tests ---

func TestCheckMissionComplete_AllDone(t *testing.T) {
	store := newIntegratorMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}
	store.tasks["t1"] = &Task{ID: "t1", MissionID: "m1", Status: TaskIntegrated}
	store.tasks["t2"] = &Task{ID: "t2", MissionID: "m1", Status: TaskDone}

	ie := NewIntegrationEngine(store, "/tmp/repo")
	complete, err := ie.CheckMissionComplete(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if !complete {
		t.Error("expected mission complete when all tasks integrated/done")
	}
}

func TestCheckMissionComplete_NotDone(t *testing.T) {
	store := newIntegratorMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}
	store.tasks["t1"] = &Task{ID: "t1", MissionID: "m1", Status: TaskIntegrated}
	store.tasks["t2"] = &Task{ID: "t2", MissionID: "m1", Status: TaskRunning}

	ie := NewIntegrationEngine(store, "/tmp/repo")
	complete, err := ie.CheckMissionComplete(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if complete {
		t.Error("expected mission not complete when tasks still running")
	}
}

func TestCheckMissionComplete_Empty(t *testing.T) {
	store := newIntegratorMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	ie := NewIntegrationEngine(store, "/tmp/repo")
	complete, err := ie.CheckMissionComplete(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if complete {
		t.Error("expected false for empty task list")
	}
}

func TestCompleteMission(t *testing.T) {
	store := newIntegratorMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	ie := NewIntegrationEngine(store, "/tmp/repo")
	err := ie.CompleteMission(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}

	if store.missions["m1"].Status != MissionCompleted {
		t.Errorf("expected completed, got %s", store.missions["m1"].Status)
	}
	if store.missions["m1"].EndedAt == nil {
		t.Error("expected EndedAt to be set")
	}
}

func TestCompleteMission_WrongState(t *testing.T) {
	store := newIntegratorMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionDraft}

	ie := NewIntegrationEngine(store, "/tmp/repo")
	err := ie.CompleteMission(context.Background(), "m1")
	if err == nil {
		t.Fatal("expected error for non-running mission")
	}
}

func TestCheckDependencies_AllMet(t *testing.T) {
	store := newIntegratorMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}
	store.tasks["t1"] = &Task{ID: "t1", MissionID: "m1", Status: TaskIntegrated}
	store.tasks["t2"] = &Task{ID: "t2", MissionID: "m1", Status: TaskAccepted}
	store.deps = []TaskDependency{
		{TaskID: "t2", DependsOnID: "t1"},
	}

	ie := NewIntegrationEngine(store, "/tmp/repo")
	// IntegrateTask will pass dependency checking (t1 is integrated) then
	// fail on gitMerge because /tmp/repo isn't a real repo. A git error
	// (not a dependency error) proves the dependency check passed.
	_, err := ie.IntegrateTask(context.Background(), "m1", "t2")
	if err == nil {
		t.Fatal("expected git error (no real repo)")
	}
	// Must be a git error, not a dependency error.
	errStr := err.Error()
	if strings.Contains(errStr, "dependency") {
		t.Fatalf("dependency check should have passed, got: %s", errStr)
	}
	if !strings.Contains(errStr, "checkout") && !strings.Contains(errStr, "git") {
		t.Fatalf("expected git-related error, got: %s", errStr)
	}
}

func TestCheckDependencies_NotMet(t *testing.T) {
	store := newIntegratorMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}
	store.tasks["t1"] = &Task{ID: "t1", MissionID: "m1", Status: TaskRunning}
	store.tasks["t2"] = &Task{ID: "t2", MissionID: "m1", Status: TaskAccepted}
	store.deps = []TaskDependency{
		{TaskID: "t2", DependsOnID: "t1"},
	}

	ie := NewIntegrationEngine(store, "/tmp/repo")
	result, err := ie.IntegrateTask(context.Background(), "m1", "t2")
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Error("expected failure due to unmet dependency")
	}
	if result.ErrorText == "" {
		t.Error("expected error text about dependency")
	}
}

func TestIntegrateTask_NotAccepted(t *testing.T) {
	store := newIntegratorMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}
	store.tasks["t1"] = &Task{ID: "t1", MissionID: "m1", Status: TaskRunning}

	ie := NewIntegrationEngine(store, "/tmp/repo")
	result, err := ie.IntegrateTask(context.Background(), "m1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Error("expected failure for non-accepted task")
	}
	if result.ErrorText == "" {
		t.Error("expected error text")
	}
}
