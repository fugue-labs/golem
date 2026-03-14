package mission

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) *DoltStore {
	t.Helper()
	dbName := fmt.Sprintf("testmissions_%d", time.Now().UnixNano())

	const dsnParams = "?timeout=5s&readTimeout=5s&writeTimeout=5s"

	// Create the test database on the Dolt server.
	rootDB, err := sql.Open("mysql", "root@tcp(127.0.0.1:3307)/"+dsnParams)
	if err != nil {
		t.Skip("Dolt server not available:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use a single connection to avoid database/sql retry loop on ErrBadConn.
	conn, err := rootDB.Conn(ctx)
	if err != nil {
		rootDB.Close()
		t.Skip("Dolt server not reachable:", err)
	}
	if _, err := conn.ExecContext(ctx, "CREATE DATABASE `"+dbName+"`"); err != nil {
		conn.Close()
		rootDB.Close()
		t.Skip("Cannot create test database:", err)
	}
	conn.Close()
	rootDB.Close()

	dsn := "root@tcp(127.0.0.1:3307)/" + dbName + dsnParams
	type storeResult struct {
		store *DoltStore
		err   error
	}
	ch := make(chan storeResult, 1)
	go func() {
		st, e := OpenDoltStore(dsn)
		ch <- storeResult{st, e}
	}()
	var s *DoltStore
	select {
	case res := <-ch:
		if res.err != nil {
			t.Skip("Cannot open Dolt store:", res.err)
		}
		s = res.store
	case <-time.After(15 * time.Second):
		t.Skip("Dolt store open timed out")
	}
	t.Cleanup(func() {
		s.Close()
		cleanup, err := sql.Open("mysql", "root@tcp(127.0.0.1:3307)/"+dsnParams)
		if err != nil {
			return
		}
		cctx, ccancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer ccancel()
		cleanup.ExecContext(cctx, "DROP DATABASE IF EXISTS `"+dbName+"`")
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

	// Look up tasks by title since ApplyPlan generates unique IDs.
	tasks, _ := s.ListTasks(ctx, m.ID)
	taskByTitle := make(map[string]*Task)
	for _, tk := range tasks {
		taskByTitle[tk.Title] = tk
	}

	implTask := taskByTitle["Implement feature"]
	testTask := taskByTitle["Write tests"]
	if implTask == nil || testTask == nil {
		t.Fatalf("expected tasks by title, got titles: %v", func() []string {
			var names []string
			for n := range taskByTitle {
				names = append(names, n)
			}
			return names
		}())
	}

	if implTask.Status != TaskReady {
		t.Errorf("impl task status = %s, want ready", implTask.Status)
	}
	if testTask.Status != TaskPending {
		t.Errorf("test task status = %s, want pending", testTask.Status)
	}
}

func TestOpenDoltStoreRecoverDroppedDB(t *testing.T) {
	// Verify that OpenDoltStore recovers a database that was previously dropped.
	// This simulates the gt dolt cleanup scenario where golem_missions gets
	// dropped as an "orphan" database.
	dbName := fmt.Sprintf("testmissions_undrop_%d", time.Now().UnixNano())
	const dsnParams = "?timeout=5s&readTimeout=5s&writeTimeout=5s"
	dsn := "root@tcp(127.0.0.1:3307)/" + dbName + dsnParams

	// Step 1: Create store and write data.
	type storeResult struct {
		store *DoltStore
		err   error
	}
	ch := make(chan storeResult, 1)
	go func() {
		s, e := OpenDoltStore(dsn)
		ch <- storeResult{s, e}
	}()
	var s1 *DoltStore
	select {
	case res := <-ch:
		if res.err != nil {
			t.Skip("Cannot open Dolt store:", res.err)
		}
		s1 = res.store
	case <-time.After(15 * time.Second):
		t.Skip("Dolt store open timed out")
	}

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	m := &Mission{ID: "m_undrop", Title: "Undrop Test", Goal: "Survive a drop", Status: MissionDraft, CreatedAt: now, UpdatedAt: now}
	if err := s1.CreateMission(ctx, m); err != nil {
		t.Fatal("create mission:", err)
	}
	s1.Close()

	// Step 2: Drop the database (simulates gt dolt cleanup).
	rootDB, err := sql.Open("mysql", "root@tcp(127.0.0.1:3307)/"+dsnParams)
	if err != nil {
		t.Fatal("open root:", err)
	}
	if _, err := rootDB.ExecContext(ctx, "DROP DATABASE IF EXISTS `"+dbName+"`"); err != nil {
		rootDB.Close()
		t.Fatal("drop database:", err)
	}
	rootDB.Close()

	// Step 3: Re-open the store — should recover via DOLT_UNDROP.
	ch2 := make(chan storeResult, 1)
	go func() {
		s, e := OpenDoltStore(dsn)
		ch2 <- storeResult{s, e}
	}()
	var s2 *DoltStore
	select {
	case res := <-ch2:
		if res.err != nil {
			t.Fatal("reopen after drop:", res.err)
		}
		s2 = res.store
	case <-time.After(15 * time.Second):
		t.Fatal("reopen timed out")
	}
	t.Cleanup(func() {
		s2.Close()
		cleanup, err := sql.Open("mysql", "root@tcp(127.0.0.1:3307)/"+dsnParams)
		if err != nil {
			return
		}
		cctx, ccancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer ccancel()
		cleanup.ExecContext(cctx, "DROP DATABASE IF EXISTS `"+dbName+"`")
		cleanup.Close()
	})

	// Step 4: Verify the mission data survived the drop+recovery.
	got, err := s2.GetMission(ctx, "m_undrop")
	if err != nil {
		t.Fatal("get mission after recovery:", err)
	}
	if got.Title != "Undrop Test" {
		t.Errorf("recovered mission title = %q, want %q", got.Title, "Undrop Test")
	}
}

func TestResolveDSN_Defaults(t *testing.T) {
	dsn := ResolveDSN()
	if !strings.Contains(dsn, DefaultDoltHost) {
		t.Errorf("expected default host in DSN, got %s", dsn)
	}
	if !strings.Contains(dsn, DefaultDoltDB) {
		t.Errorf("expected default db in DSN, got %s", dsn)
	}
}

func TestResolveDSN_FullOverride(t *testing.T) {
	t.Setenv("GOLEM_DOLT_DSN", "root@tcp(remote:3308)/custom_db")
	dsn := ResolveDSN()
	if dsn != "root@tcp(remote:3308)/custom_db" {
		t.Errorf("expected full DSN override, got %s", dsn)
	}
}

func TestResolveDSN_ComponentOverride(t *testing.T) {
	t.Setenv("GOLEM_DOLT_HOST", "10.0.0.5:3309")
	t.Setenv("GOLEM_DOLT_DB", "staging_missions")
	dsn := ResolveDSN()
	if !strings.Contains(dsn, "10.0.0.5:3309") {
		t.Errorf("expected host override in DSN, got %s", dsn)
	}
	if !strings.Contains(dsn, "staging_missions") {
		t.Errorf("expected db override in DSN, got %s", dsn)
	}
}

func TestResolveDSN_FullOverrideTakesPrecedence(t *testing.T) {
	t.Setenv("GOLEM_DOLT_DSN", "root@tcp(full:9999)/fulldb")
	t.Setenv("GOLEM_DOLT_HOST", "ignored:1111")
	t.Setenv("GOLEM_DOLT_DB", "ignored_db")
	dsn := ResolveDSN()
	if dsn != "root@tcp(full:9999)/fulldb" {
		t.Errorf("full DSN should take precedence, got %s", dsn)
	}
}
