package mission

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

func testStore(t *testing.T) *DoltStore {
	t.Helper()
	dbName := fmt.Sprintf("testmissions_%d", time.Now().UnixNano())

	// Create the test database on the Dolt server.
	rootDB, err := sql.Open("mysql", "root@tcp(127.0.0.1:3307)/")
	if err != nil {
		t.Skip("Dolt server not available:", err)
	}
	if err := rootDB.Ping(); err != nil {
		rootDB.Close()
		t.Skip("Dolt server not reachable:", err)
	}
	if _, err := rootDB.Exec("CREATE DATABASE `" + dbName + "`"); err != nil {
		rootDB.Close()
		t.Skip("Cannot create test database:", err)
	}
	rootDB.Close()

	dsn := "root@tcp(127.0.0.1:3307)/" + dbName
	s, err := OpenDoltStore(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.Close()
		cleanup, _ := sql.Open("mysql", "root@tcp(127.0.0.1:3307)/")
		cleanup.Exec("DROP DATABASE `" + dbName + "`")
		cleanup.Close()
	})
	return s
}

func TestMissionCRUD(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	m := &Mission{
		ID:        "m_test1",
		Title:     "Test Mission",
		Goal:      "Test the store",
		RepoRoot:  "/tmp/repo",
		Status:    MissionDraft,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.CreateMission(ctx, m); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetMission(ctx, "m_test1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Test Mission" {
		t.Errorf("got title %q, want %q", got.Title, "Test Mission")
	}
	if got.Status != MissionDraft {
		t.Errorf("got status %q, want %q", got.Status, MissionDraft)
	}

	got.Status = MissionPlanning
	got.UpdatedAt = now.Add(time.Second)
	if err := s.UpdateMission(ctx, got); err != nil {
		t.Fatal(err)
	}

	got2, err := s.GetMission(ctx, "m_test1")
	if err != nil {
		t.Fatal(err)
	}
	if got2.Status != MissionPlanning {
		t.Errorf("got status %q, want %q", got2.Status, MissionPlanning)
	}

	missions, err := s.ListMissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(missions) != 1 {
		t.Errorf("got %d missions, want 1", len(missions))
	}
}

func TestTaskCRUD(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	m := &Mission{ID: "m_t2", Title: "M", Goal: "G", Status: MissionDraft, CreatedAt: now, UpdatedAt: now}
	if err := s.CreateMission(ctx, m); err != nil {
		t.Fatal(err)
	}

	task := &Task{
		ID:        "t_1",
		MissionID: "m_t2",
		Title:     "Task 1",
		Kind:      TaskKindCode,
		Objective: "Do something",
		Status:    TaskPending,
		Priority:  1,
		RiskLevel: RiskLow,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetTask(ctx, "t_1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Task 1" {
		t.Errorf("got title %q", got.Title)
	}

	task2 := &Task{
		ID: "t_2", MissionID: "m_t2", Title: "Task 2", Kind: TaskKindTest,
		Status: TaskPending, Priority: 2, RiskLevel: RiskMedium, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateTask(ctx, task2); err != nil {
		t.Fatal(err)
	}

	tasks, err := s.ListTasks(ctx, "m_t2")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Errorf("got %d tasks, want 2", len(tasks))
	}
	// Higher priority first.
	if tasks[0].ID != "t_2" {
		t.Errorf("expected t_2 first (higher priority), got %s", tasks[0].ID)
	}
}

func TestDependencies(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	m := &Mission{ID: "m_d1", Title: "M", Goal: "G", Status: MissionDraft, CreatedAt: now, UpdatedAt: now}
	s.CreateMission(ctx, m)

	s.CreateTask(ctx, &Task{ID: "t_a", MissionID: "m_d1", Title: "A", Kind: TaskKindCode, Status: TaskPending, CreatedAt: now, UpdatedAt: now})
	s.CreateTask(ctx, &Task{ID: "t_b", MissionID: "m_d1", Title: "B", Kind: TaskKindCode, Status: TaskPending, CreatedAt: now, UpdatedAt: now})

	if err := s.AddDependency(ctx, TaskDependency{TaskID: "t_b", DependsOnID: "t_a"}); err != nil {
		t.Fatal(err)
	}

	deps, err := s.ListDependencies(ctx, "m_d1")
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 {
		t.Fatalf("got %d deps, want 1", len(deps))
	}
	if deps[0].TaskID != "t_b" || deps[0].DependsOnID != "t_a" {
		t.Errorf("unexpected dep: %+v", deps[0])
	}
}

func TestEventLog(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	m := &Mission{ID: "m_ev", Title: "M", Goal: "G", Status: MissionDraft, CreatedAt: now, UpdatedAt: now}
	s.CreateMission(ctx, m)

	for i := 0; i < 5; i++ {
		s.AppendEvent(ctx, &Event{
			MissionID: "m_ev",
			Type:      "test.event",
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		})
	}

	events, err := s.ListEvents(ctx, "m_ev", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Errorf("got %d events, want 3", len(events))
	}
}

func TestMissionSummary(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	m := &Mission{ID: "m_sum", Title: "M", Goal: "G", Status: MissionRunning, CreatedAt: now, UpdatedAt: now}
	s.CreateMission(ctx, m)

	s.CreateTask(ctx, &Task{ID: "t_r1", MissionID: "m_sum", Title: "R1", Kind: TaskKindCode, Status: TaskReady, CreatedAt: now, UpdatedAt: now})
	s.CreateTask(ctx, &Task{ID: "t_r2", MissionID: "m_sum", Title: "R2", Kind: TaskKindCode, Status: TaskReady, CreatedAt: now, UpdatedAt: now})
	s.CreateTask(ctx, &Task{ID: "t_d1", MissionID: "m_sum", Title: "D1", Kind: TaskKindCode, Status: TaskDone, CreatedAt: now, UpdatedAt: now})
	s.CreateTask(ctx, &Task{ID: "t_b1", MissionID: "m_sum", Title: "B1", Kind: TaskKindCode, Status: TaskBlocked, CreatedAt: now, UpdatedAt: now})

	summary, err := s.GetMissionSummary(ctx, "m_sum")
	if err != nil {
		t.Fatal(err)
	}
	if summary.TaskCounts.Total != 4 {
		t.Errorf("total tasks = %d, want 4", summary.TaskCounts.Total)
	}
	if summary.TaskCounts.Ready != 2 {
		t.Errorf("ready tasks = %d, want 2", summary.TaskCounts.Ready)
	}
	if summary.TaskCounts.Done != 1 {
		t.Errorf("done tasks = %d, want 1", summary.TaskCounts.Done)
	}
	if summary.TaskCounts.Blocked != 1 {
		t.Errorf("blocked tasks = %d, want 1", summary.TaskCounts.Blocked)
	}
}

func TestControllerApplyPlan(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ctrl := NewController(s)

	m, err := ctrl.CreateMission(ctx, CreateMissionRequest{
		Title: "Plan Test",
		Goal:  "Test planning flow",
	})
	if err != nil {
		t.Fatal(err)
	}

	plan := &PlanResult{
		Summary:         "A simple two-task plan",
		SuccessCriteria: []string{"All tests pass"},
		Tasks: []PlanTask{
			{ID: "t_impl", Title: "Implement feature", Kind: TaskKindCode, Objective: "Write the code", Priority: 2, RiskLevel: RiskLow},
			{ID: "t_test", Title: "Write tests", Kind: TaskKindTest, Objective: "Test the code", Priority: 1, RiskLevel: RiskLow},
		},
		Dependencies: []TaskDependency{
			{TaskID: "t_test", DependsOnID: "t_impl"},
		},
	}

	if err := ctrl.ApplyPlan(ctx, m.ID, plan); err != nil {
		t.Fatal(err)
	}

	// Mission should be awaiting_approval.
	updated, _ := ctrl.GetMission(ctx, m.ID)
	if updated.Status != MissionAwaitingApproval {
		t.Errorf("mission status = %s, want awaiting_approval", updated.Status)
	}

	// t_impl should be ready (no deps), t_test should be pending (depends on t_impl).
	tasks, _ := s.ListTasks(ctx, m.ID)
	taskMap := make(map[string]*Task)
	for _, t := range tasks {
		taskMap[t.ID] = t
	}

	if taskMap["t_impl"].Status != TaskReady {
		t.Errorf("t_impl status = %s, want ready", taskMap["t_impl"].Status)
	}
	if taskMap["t_test"].Status != TaskPending {
		t.Errorf("t_test status = %s, want pending", taskMap["t_test"].Status)
	}
}
