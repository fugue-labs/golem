package dashboard

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fugue-labs/golem/internal/mission"
)

func setupTestDashboard(t *testing.T) (*Model, *mission.Controller) {
	t.Helper()
	store := openTestDoltStore(t)
	ctrl := mission.NewController(store)
	m := New("")
	m.ctrl = ctrl
	m.width = 120
	m.height = 40
	return m, ctrl
}

func openTestDoltStore(t *testing.T) *mission.DoltStore {
	t.Helper()
	dbName := fmt.Sprintf("testdash_%d", time.Now().UnixNano())

	const dsnParams = "?timeout=5s&readTimeout=5s&writeTimeout=5s"
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
		store *mission.DoltStore
		err   error
	}
	ch := make(chan storeResult, 1)
	go func() {
		s, e := mission.OpenDoltStore(dsn)
		ch <- storeResult{s, e}
	}()
	var store *mission.DoltStore
	select {
	case res := <-ch:
		if res.err != nil {
			t.Skip("Cannot open Dolt store:", res.err)
		}
		store = res.store
	case <-time.After(15 * time.Second):
		t.Skip("Dolt store open timed out")
	}
	t.Cleanup(func() {
		store.Close()
		cleanup, err := sql.Open("mysql", "root@tcp(127.0.0.1:3307)/"+dsnParams)
		if err != nil {
			return
		}
		cctx, ccancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer ccancel()
		cleanup.ExecContext(cctx, "DROP DATABASE IF EXISTS `"+dbName+"`")
		cleanup.Close()
	})
	return store
}

func createTestMission(t *testing.T, ctrl *mission.Controller) *mission.Mission {
	t.Helper()
	ctx := context.Background()
	ms, err := ctrl.CreateMission(ctx, mission.CreateMissionRequest{
		Title:      "Test mission: refactor auth module",
		Goal:       "Split monolithic auth into OAuth2 services, add tests, update docs",
		RepoRoot:   "/tmp/test-repo",
		BaseCommit: "abc123",
		BaseBranch: "main",
		Budget:     mission.Budget{MaxConcurrentWorkers: 3},
	})
	if err != nil {
		t.Fatalf("create mission: %v", err)
	}
	return ms
}

func applyTestPlan(t *testing.T, ctrl *mission.Controller, missionID string) {
	t.Helper()
	ctx := context.Background()
	plan := &mission.PlanResult{
		Summary:         "Refactor auth into OAuth2 services",
		SuccessCriteria: []string{"All tests pass", "OAuth2 flow works"},
		Tasks: []mission.PlanTask{
			{ID: "t1", Title: "Scaffold OAuth2 types", Kind: mission.TaskKindCode, Objective: "Create base types", Priority: 3, RiskLevel: mission.RiskLow},
			{ID: "t2", Title: "Implement token handler", Kind: mission.TaskKindCode, Objective: "Token refresh flow", Priority: 2, RiskLevel: mission.RiskMedium},
			{ID: "t3", Title: "Add unit tests", Kind: mission.TaskKindTest, Objective: "Test coverage for auth", Priority: 2, RiskLevel: mission.RiskLow},
			{ID: "t4", Title: "Update API docs", Kind: mission.TaskKindDocs, Objective: "Document new auth endpoints", Priority: 1, RiskLevel: mission.RiskLow},
		},
		Dependencies: []mission.TaskDependency{
			{TaskID: "t2", DependsOnID: "t1"},
			{TaskID: "t3", DependsOnID: "t2"},
			{TaskID: "t4", DependsOnID: "t2"},
		},
	}
	if err := ctrl.ApplyPlan(ctx, missionID, plan); err != nil {
		t.Fatalf("apply plan: %v", err)
	}
}

func viewString(m *Model) string {
	return m.View().Content
}

func TestNew(t *testing.T) {
	m := New("test-123")
	if m.missionID != "test-123" {
		t.Errorf("expected missionID=test-123, got %s", m.missionID)
	}
	if m.sty == nil {
		t.Error("expected styles to be initialized")
	}
}

func TestViewNoMission(t *testing.T) {
	m := New("")
	m.width = 80
	m.height = 24
	view := viewString(m)
	if view == "" {
		t.Error("expected non-empty view")
	}
	if !containsAny(view, "Mission Control") {
		t.Error("expected Mission Control header in no-mission state")
	}
	if !containsAny(view, "No active mission", "no missions", "error") {
		t.Errorf("expected error or empty state message, got: %s", view[:min(100, len(view))])
	}
	if !containsAny(view, "/mission new", "golem mission new") {
		t.Errorf("expected mission creation hint in empty state, got: %s", truncStr(view, 160))
	}
}

func TestViewWithMission(t *testing.T) {
	m, ctrl := setupTestDashboard(t)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID

	msg := m.doRefresh()
	if rm, ok := msg.(refreshDoneMsg); ok && rm.err != nil {
		t.Fatalf("refresh error: %v", rm.err)
	}

	view := viewString(m)
	if !containsAny(view, "Mission Control") {
		t.Error("expected 'Mission Control' in view")
	}
	if !containsAny(view, "Split monolithic auth", "refactor auth") {
		t.Errorf("expected mission goal in view, got: %s", truncStr(view, 200))
	}
}

func TestViewWithTasks(t *testing.T) {
	m, ctrl := setupTestDashboard(t)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	msg := m.doRefresh()
	if rm, ok := msg.(refreshDoneMsg); ok && rm.err != nil {
		t.Fatalf("refresh error: %v", rm.err)
	}

	view := viewString(m)
	if !containsAny(view, "Scaffold OAuth2 types") {
		t.Error("expected task title in view")
	}
	if !containsAny(view, "Tasks") {
		t.Error("expected 'Tasks' pane header in view")
	}
}

func TestViewWithRuns(t *testing.T) {
	m, ctrl := setupTestDashboard(t)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	ctx := context.Background()
	ctrl.StartMission(ctx, ms.ID)

	now := time.Now().UTC()
	heartbeat := now
	run := &mission.Run{
		ID:           "r_test_001",
		MissionID:    ms.ID,
		TaskID:       "t1",
		Mode:         mission.RunModeWorker,
		Status:       mission.RunRunning,
		WorktreePath: "/tmp/worktrees/wt1",
		StartedAt:    &now,
		HeartbeatAt:  &heartbeat,
	}
	if err := ctrl.Store().CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	msg := m.doRefresh()
	if rm, ok := msg.(refreshDoneMsg); ok && rm.err != nil {
		t.Fatalf("refresh error: %v", rm.err)
	}

	view := viewString(m)
	if !containsAny(view, "Workers") {
		t.Error("expected 'Workers' pane header in view")
	}
	if !containsAny(view, "r_test_001") {
		t.Error("expected run ID in workers pane")
	}
}

func TestKeyNavigation(t *testing.T) {
	m, ctrl := setupTestDashboard(t)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	m.doRefresh()

	if m.focusPane != paneTasks {
		t.Errorf("expected initial focus on tasks, got %d", m.focusPane)
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focusPane != paneWorkers {
		t.Errorf("expected focus on workers after tab, got %d", m.focusPane)
	}

	m.Update(tea.KeyPressMsg{Code: '3'})
	if m.focusPane != paneEvidence {
		t.Errorf("expected focus on evidence after '3', got %d", m.focusPane)
	}

	m.Update(tea.KeyPressMsg{Code: 'j'})
	if m.scrollPos[paneEvidence] != 1 {
		t.Errorf("expected scroll pos 1 after j, got %d", m.scrollPos[paneEvidence])
	}
	m.Update(tea.KeyPressMsg{Code: 'k'})
	if m.scrollPos[paneEvidence] != 0 {
		t.Errorf("expected scroll pos 0 after k, got %d", m.scrollPos[paneEvidence])
	}
}

func TestQuit(t *testing.T) {
	m, _ := setupTestDashboard(t)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q'})
	if !m.quitting {
		t.Error("expected quitting=true after 'q'")
	}
	if cmd == nil {
		t.Error("expected quit command")
	}
}

func TestMissionPriority(t *testing.T) {
	tests := []struct {
		status mission.MissionStatus
		want   int
	}{
		{mission.MissionRunning, 6},
		{mission.MissionBlocked, 5},
		{mission.MissionPaused, 4},
		{mission.MissionAwaitingApproval, 3},
		{mission.MissionPlanning, 2},
		{mission.MissionDraft, 1},
		{mission.MissionCompleted, 0},
	}
	for _, tt := range tests {
		got := missionPriority(tt.status)
		if got != tt.want {
			t.Errorf("missionPriority(%s) = %d, want %d", tt.status, got, tt.want)
		}
	}
}

func TestPadRight(t *testing.T) {
	if got := padRight("abc", 6); got != "abc   " {
		t.Errorf("padRight(abc, 6) = %q, want %q", got, "abc   ")
	}
	if got := padRight("abcdef", 3); lipgloss.Width(got) > 3 {
		t.Errorf("padRight(abcdef, 3) should truncate, got width %d", lipgloss.Width(got))
	}
}

func TestAutoSelectMission(t *testing.T) {
	m, ctrl := setupTestDashboard(t)
	ctx := context.Background()

	// Create two missions: draft and running.
	_, _ = ctrl.CreateMission(ctx, mission.CreateMissionRequest{
		Title: "Draft mission", Goal: "draft goal", RepoRoot: "/tmp",
		BaseBranch: "main",
	})
	running, _ := ctrl.CreateMission(ctx, mission.CreateMissionRequest{
		Title: "Running mission", Goal: "running goal", RepoRoot: "/tmp",
		BaseBranch: "main",
	})

	applyTestPlan(t, ctrl, running.ID)
	ctrl.StartMission(ctx, running.ID)

	m.missionID = ""
	msg := m.doRefresh()
	if rm, ok := msg.(refreshDoneMsg); ok && rm.err != nil {
		t.Fatalf("refresh error: %v", rm.err)
	}

	if m.missionID != running.ID {
		t.Errorf("expected auto-select running mission %s, got %s", running.ID, m.missionID)
	}
}

func TestWindowResize(t *testing.T) {
	m := New("")
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	if m.width != 200 || m.height != 50 {
		t.Errorf("expected 200x50, got %dx%d", m.width, m.height)
	}
}

func TestDashboardEnvIntegration(t *testing.T) {
	if _, err := os.UserHomeDir(); err != nil {
		t.Skip("no home dir")
	}

	m := New("")
	m.width = 80
	m.height = 24
	view := viewString(m)
	if view == "" {
		t.Error("expected non-empty view")
	}
}

// helpers

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if contains(s, sub) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func stripANSITest(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}

func TestRenderMetricSeparatesKeyAndValue(t *testing.T) {
	m := New("")
	metric := m.renderMetric("Workers", "3 active")
	plain := stripANSITest(metric)
	if plain != "Workers 3 active" {
		t.Fatalf("expected plain metric text, got %q", plain)
	}
	if metric == plain {
		t.Fatalf("expected styled metric output to differ from plain text")
	}
}

func TestRenderPaneHeaderFocusedCopy(t *testing.T) {
	m := New("")
	plain := stripANSITest(m.renderPaneHeader("Workers", true, 80))
	if !containsAny(plain, "▸ [2] Workers", "▸ [2] Workers ACTIVE") {
		t.Fatalf("expected focused workers header, got %q", plain)
	}
	if !containsAny(plain, "ACTIVE", "j/k scroll", "Shift+Tab") {
		t.Fatalf("expected active navigation hint, got %q", plain)
	}
}

func TestRenderHeaderNoMissionShowsReadableIdleState(t *testing.T) {
	m := New("")
	m.width = 80
	m.height = 24
	joined := stripANSITest(viewString(m))
	if !containsAny(joined, "Mission Control", "No mission") {
		t.Fatalf("expected Mission Control no-mission header, got %q", joined)
	}
	if !containsAny(joined, "No active mission", "/mission new", "Shift+Tab reverse") {
		t.Fatalf("expected no-mission guidance, got %q", joined)
	}
	if !containsAny(joined, "Open the main shell", "press r to refresh") {
		t.Fatalf("expected premium no-mission action guidance, got %q", joined)
	}
}

func TestRenderFocusTabsShowsActivePane(t *testing.T) {
	m := New("")
	m.focusPane = paneEvidence
	plain := stripANSITest(m.renderFocusTabs(80))
	for _, want := range []string{"[1] Tasks", "[2] Workers", "[3] Evidence", "[4] Events"} {
		if !containsAny(plain, want) {
			t.Fatalf("expected %q in focus tabs, got %q", want, plain)
		}
	}
}

func TestRenderEmptyStateIncludesActionHint(t *testing.T) {
	m := New("")
	plain := stripANSITest(stringsJoin(m.renderEmptyState(60, "No active workers", "Mission Control is idle."), "\n"))
	if !containsAny(plain, "Use /mission start", "No active workers") {
		t.Fatalf("expected actionable empty state, got %q", plain)
	}
}

func stringsJoin(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, part := range parts[1:] {
		out += sep + part
	}
	return out
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
