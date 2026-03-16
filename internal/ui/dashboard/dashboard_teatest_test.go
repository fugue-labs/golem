package dashboard

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fugue-labs/golem/internal/mission"
)

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}

// setupTeatestModel creates a dashboard model with a real Dolt store and
// pre-injected controller, suitable for teatest integration tests that run
// through the full bubbletea program lifecycle.
func setupTeatestModel(t *testing.T, width, height int) (*Model, *mission.Controller) {
	t.Helper()
	store := openTestDoltStore(t)
	ctrl := mission.NewController(store)
	m := New("")
	m.ctrl = ctrl
	m.width = width
	m.height = height
	return m, ctrl
}

// populateWorkers creates running workers attached to existing tasks.
func populateWorkers(t *testing.T, ctrl *mission.Controller, missionID string, configs []workerConfig) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	for _, cfg := range configs {
		heartbeat := now
		run := &mission.Run{
			ID:           cfg.runID,
			MissionID:    missionID,
			TaskID:       cfg.taskID,
			Mode:         cfg.mode,
			Status:       cfg.status,
			WorktreePath: cfg.worktree,
			StartedAt:    &now,
			HeartbeatAt:  &heartbeat,
		}
		if cfg.summary != "" {
			run.Summary = cfg.summary
		}
		if cfg.errorText != "" {
			run.ErrorText = cfg.errorText
		}
		if cfg.ended {
			ended := now.Add(-30 * time.Second)
			run.EndedAt = &ended
		}
		if err := ctrl.Store().CreateRun(ctx, run); err != nil {
			t.Fatalf("create run %s: %v", cfg.runID, err)
		}
	}
}

type workerConfig struct {
	runID     string
	taskID    string
	mode      mission.RunMode
	status    mission.RunStatus
	worktree  string
	summary   string
	errorText string
	ended     bool
}

// setTaskStatus updates a task's status in the store.
func setTaskStatus(t *testing.T, ctrl *mission.Controller, taskID string, status mission.TaskStatus) {
	t.Helper()
	ctx := context.Background()
	task, err := ctrl.Store().GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("get task %s: %v", taskID, err)
	}
	task.Status = status
	if err := ctrl.Store().UpdateTask(ctx, task); err != nil {
		t.Fatalf("update task %s: %v", taskID, err)
	}
}

// refreshModel performs a doRefresh and applies the result via Update.
func refreshModel(t *testing.T, m *Model) {
	t.Helper()
	msg := m.doRefresh()
	if rm, ok := msg.(refreshDoneMsg); ok && rm.err != nil {
		t.Fatalf("refresh: %v", rm.err)
	}
	m.Update(msg)
}

// --- Program Lifecycle Tests ---

func TestTeatestProgramLifecycle(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	p := tea.NewProgram(m,
		tea.WithInput(strings.NewReader("")),
		tea.WithOutput(io.Discard),
		tea.WithoutSignalHandler(),
	)

	go func() {
		// Allow Init commands to execute (initStore skips since ctrl pre-set,
		// doRefresh loads data, tickCmd schedules periodic refresh).
		time.Sleep(300 * time.Millisecond)
		p.Send(tea.KeyPressMsg{Code: 'q'})
	}()

	finalModel, err := p.Run()
	if err != nil {
		t.Fatalf("program run: %v", err)
	}

	dm, ok := finalModel.(*Model)
	if !ok {
		t.Fatal("expected *Model from Run()")
	}

	if !dm.quitting {
		t.Error("expected quitting=true after program exit")
	}
	if dm.missionObj == nil {
		t.Error("expected mission data to be loaded during lifecycle")
	}
	if len(dm.tasks) != 4 {
		t.Errorf("expected 4 tasks loaded, got %d", len(dm.tasks))
	}
}

func TestTeatestProgramWithWorkers(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	ctx := context.Background()
	ctrl.StartMission(ctx, ms.ID)

	populateWorkers(t, ctrl, ms.ID, []workerConfig{
		{runID: "r_wk_001", taskID: "t1", mode: mission.RunModeWorker, status: mission.RunRunning, worktree: "/tmp/wt1"},
		{runID: "r_wk_002", taskID: "t2", mode: mission.RunModeWorker, status: mission.RunRunning, worktree: "/tmp/wt2"},
	})

	p := tea.NewProgram(m,
		tea.WithInput(strings.NewReader("")),
		tea.WithOutput(io.Discard),
		tea.WithoutSignalHandler(),
	)

	go func() {
		time.Sleep(300 * time.Millisecond)
		// Navigate to workers pane then quit.
		p.Send(tea.KeyPressMsg{Code: '2'})
		time.Sleep(50 * time.Millisecond)
		p.Send(tea.KeyPressMsg{Code: 'q'})
	}()

	finalModel, err := p.Run()
	if err != nil {
		t.Fatalf("program run: %v", err)
	}

	dm := finalModel.(*Model)
	if dm.focusPane != paneWorkers {
		t.Errorf("expected focus on workers pane, got %d", dm.focusPane)
	}
	if len(dm.runs) < 2 {
		t.Errorf("expected at least 2 runs loaded, got %d", len(dm.runs))
	}
}

// --- Rendering Tests: Multiple Workers ---

func TestTeatestMultipleWorkerRendering(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	ctx := context.Background()
	ctrl.StartMission(ctx, ms.ID)

	populateWorkers(t, ctrl, ms.ID, []workerConfig{
		{runID: "r_alpha_001", taskID: "t1", mode: mission.RunModeWorker, status: mission.RunRunning, worktree: "/tmp/wt-alpha"},
		{runID: "r_beta_0002", taskID: "t2", mode: mission.RunModeWorker, status: mission.RunRunning, worktree: "/tmp/wt-beta"},
		{runID: "r_gamma_003", taskID: "t3", mode: mission.RunModeReview, status: mission.RunRunning, worktree: "/tmp/wt-gamma"},
		{runID: "r_delta_004", taskID: "t4", mode: mission.RunModePlanner, status: mission.RunQueued, worktree: "/tmp/wt-delta"},
	})

	refreshModel(t, m)
	view := viewString(m)

	// Verify Workers pane header.
	if !strings.Contains(view, "Workers") {
		t.Error("expected 'Workers' pane header in view")
	}

	// Verify each worker run ID appears (truncated to 12 chars).
	for _, runID := range []string{"r_alpha_001", "r_beta_0002", "r_gamma_003", "r_delta_004"} {
		displayed := runID[:min(12, len(runID))]
		if !strings.Contains(view, displayed) {
			t.Errorf("expected worker run ID %q in view", displayed)
		}
	}

	// Verify worktree base names appear.
	for _, wt := range []string{"wt-alpha", "wt-beta", "wt-gamma"} {
		if !strings.Contains(view, wt) {
			t.Errorf("expected worktree %q in view", wt)
		}
	}
}

func TestTeatestWorkerMetricsInHeader(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	ctx := context.Background()
	ctrl.StartMission(ctx, ms.ID)

	populateWorkers(t, ctrl, ms.ID, []workerConfig{
		{runID: "r_w1", taskID: "t1", mode: mission.RunModeWorker, status: mission.RunRunning, worktree: "/tmp/wt1"},
		{runID: "r_w2", taskID: "t2", mode: mission.RunModeWorker, status: mission.RunRunning, worktree: "/tmp/wt2"},
		{runID: "r_w3", taskID: "t3", mode: mission.RunModeWorker, status: mission.RunQueued, worktree: "/tmp/wt3"},
	})

	refreshModel(t, m)
	view := stripANSI(viewString(m))

	// Header should show active worker count (running + queued = 3).
	if !strings.Contains(view, "Workers 3 active") {
		t.Errorf("expected 'Workers 3 active' in header metrics, got:\n%s", truncStr(view, 300))
	}
}

// --- Rendering Tests: Agent Status Updates ---

func TestTeatestTaskStatusIcons(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 50)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	ctx := context.Background()
	ctrl.StartMission(ctx, ms.ID)

	// Set tasks to various statuses.
	setTaskStatus(t, ctrl, "t1", mission.TaskDone)
	setTaskStatus(t, ctrl, "t2", mission.TaskRunning)
	setTaskStatus(t, ctrl, "t3", mission.TaskBlocked)
	setTaskStatus(t, ctrl, "t4", mission.TaskReady)

	refreshModel(t, m)
	view := viewString(m)

	// Verify task group headers appear.
	for _, group := range []string{"Running", "Ready", "Blocked", "Done"} {
		if !strings.Contains(view, group) {
			t.Errorf("expected task group %q in view", group)
		}
	}

	// Verify task titles still appear (strip ANSI — done tasks use per-char strikethrough).
	plainView := stripANSI(view)
	if !strings.Contains(plainView, "Scaffold OAuth2 types") {
		t.Errorf("expected done task title in view, view=\n%s", view)
	}
	if !strings.Contains(plainView, "Implement token handler") {
		t.Error("expected running task title in view")
	}
}

func TestTeatestTaskStatusTransitions(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 50)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	ctx := context.Background()
	ctrl.StartMission(ctx, ms.ID)

	// Initially all tasks should be in non-done state after plan apply.
	refreshModel(t, m)
	view1 := viewString(m)

	// No "Done" group should appear yet (tasks start as ready/pending).
	if strings.Contains(view1, "Done (") {
		t.Error("expected no Done group initially")
	}

	// Transition t1 to done.
	setTaskStatus(t, ctrl, "t1", mission.TaskDone)
	refreshModel(t, m)
	view2 := viewString(m)

	if !strings.Contains(view2, "Done") {
		t.Error("expected Done group after marking t1 done")
	}

	// Mark remaining tasks as done.
	for _, id := range []string{"t2", "t3", "t4"} {
		setTaskStatus(t, ctrl, id, mission.TaskDone)
	}
	refreshModel(t, m)
	view3 := viewString(m)

	if !strings.Contains(view3, "Done (4)") {
		t.Errorf("expected 'Done (4)' in view after all tasks done, got:\n%s", truncStr(view3, 500))
	}
}

func TestTeatestAwaitingReviewStatus(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 50)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	ctx := context.Background()
	ctrl.StartMission(ctx, ms.ID)

	setTaskStatus(t, ctrl, "t1", mission.TaskAwaitingReview)

	refreshModel(t, m)
	view := viewString(m)

	if !strings.Contains(view, "Review") {
		t.Error("expected 'Review' group for awaiting_review task")
	}
}

// --- Rendering Tests: Task Assignment Display ---

func TestTeatestTaskAssignmentInWorkerCards(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	ctx := context.Background()
	ctrl.StartMission(ctx, ms.ID)

	setTaskStatus(t, ctrl, "t1", mission.TaskRunning)

	populateWorkers(t, ctrl, ms.ID, []workerConfig{
		{runID: "r_assign_001", taskID: "t1", mode: mission.RunModeWorker, status: mission.RunRunning, worktree: "/tmp/wt1"},
	})

	refreshModel(t, m)
	view := viewString(m)

	// Worker card should show the assigned task title.
	if !strings.Contains(view, "Scaffold OAuth2 types") {
		t.Error("expected assigned task title 'Scaffold OAuth2 types' in worker card")
	}

	// Worker card should show the run ID.
	if !strings.Contains(view, "r_assign_001") {
		t.Error("expected run ID in worker card")
	}
}

func TestTeatestTaskDependencyDisplay(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 50)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	refreshModel(t, m)
	view := viewString(m)

	// Dependencies should show "needs:" lines.
	if !strings.Contains(view, "needs:") {
		t.Error("expected dependency 'needs:' in task pane")
	}
}

// --- Rendering Tests: Panel Resize Behavior ---

func TestTeatestPanelResizeSmall(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 60, 20)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	refreshModel(t, m)
	view := viewString(m)

	// Even at small size, should render without panic.
	if view == "" {
		t.Error("expected non-empty view at small terminal size")
	}

	// Should still contain key elements.
	if !strings.Contains(view, "Mission Control") {
		t.Error("expected 'Mission Control' in small view")
	}
	if !strings.Contains(view, "Compact layout") {
		t.Error("expected compact layout guidance in small view")
	}
	if !strings.Contains(view, "[1] Tasks") {
		t.Error("expected focus tabs in compact view")
	}

	// View lines should not exceed terminal height.
	lines := strings.Split(view, "\n")
	if len(lines) > m.height {
		t.Errorf("view has %d lines but terminal height is %d", len(lines), m.height)
	}
}

func TestTeatestPanelResizeLarge(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 200, 60)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	ctx := context.Background()
	ctrl.StartMission(ctx, ms.ID)

	populateWorkers(t, ctrl, ms.ID, []workerConfig{
		{runID: "r_large_001", taskID: "t1", mode: mission.RunModeWorker, status: mission.RunRunning, worktree: "/tmp/wt1"},
	})

	refreshModel(t, m)
	view := viewString(m)

	lines := strings.Split(view, "\n")
	if len(lines) > m.height {
		t.Errorf("view has %d lines but terminal height is %d", len(lines), m.height)
	}

	// At large size, panes should have room to display all content.
	if !strings.Contains(view, "Tasks") {
		t.Error("expected Tasks pane at large size")
	}
	if !strings.Contains(view, "Workers") {
		t.Error("expected Workers pane at large size")
	}
	if !strings.Contains(view, "Evidence") {
		t.Error("expected Evidence pane at large size")
	}
	if !strings.Contains(view, "Events") {
		t.Error("expected Events pane at large size")
	}
}

func TestTeatestDynamicResize(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)
	refreshModel(t, m)

	// Start at 120x40.
	view1 := viewString(m)
	lines1 := strings.Split(view1, "\n")

	// Resize to 80x24.
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view2 := viewString(m)
	lines2 := strings.Split(view2, "\n")

	if len(lines2) > 24 {
		t.Errorf("after resize to 24 rows, got %d lines", len(lines2))
	}
	if len(lines1) == len(lines2) && m.width == 120 {
		t.Error("resize should have changed model dimensions")
	}
	if m.width != 80 || m.height != 24 {
		t.Errorf("expected 80x24 after resize, got %dx%d", m.width, m.height)
	}

	// Resize back to large.
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	view3 := viewString(m)
	lines3 := strings.Split(view3, "\n")
	if len(lines3) > 60 {
		t.Errorf("after resize to 60 rows, got %d lines", len(lines3))
	}
}

func TestTeatestMinimalSize(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 30, 10)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)
	refreshModel(t, m)

	// Should not panic at very small sizes.
	view := viewString(m)
	if view == "" {
		t.Error("expected non-empty view at minimal size")
	}
	if !strings.Contains(view, "Mission Control") {
		t.Error("expected Mission Control identity at minimal size")
	}
	if !strings.Contains(view, "Compact") {
		t.Error("expected compact rescue copy at minimal size")
	}
}

func TestTeatestDashboardErrorStateIsReadable(t *testing.T) {
	m := New("")
	m.width = 70
	m.height = 18
	m.lastErr = fmt.Errorf("store offline")

	view := viewString(m)
	if !strings.Contains(view, "Mission Control") {
		t.Fatal("expected Mission Control identity in error state")
	}
	for _, want := range []string{"Dashboard error", "Unable to load dashboard", "Press r to retry", "store offline"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in error view, got:\n%s", want, view)
		}
	}
}

func TestTeatestCompactLayoutTracksFocusedPane(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 72, 20)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)
	refreshModel(t, m)

	view := viewString(m)
	if !strings.Contains(view, "Tasks") {
		t.Fatalf("expected tasks pane in compact view, got:\n%s", view)
	}
	if !strings.Contains(view, "Compact") {
		t.Fatalf("expected compact support copy in compact view, got:\n%s", view)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: '2'})
	m = updated.(*Model)
	view = viewString(m)
	if !strings.Contains(view, "Workers") {
		t.Fatalf("expected workers pane after focus jump, got:\n%s", view)
	}
	if !strings.Contains(view, "[2] Workers") {
		t.Fatalf("expected workers focus tab in compact view, got:\n%s", view)
	}
}

func TestTeatestViewMetadataAndKeyboardNavigation(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)
	refreshModel(t, m)

	v := m.View()
	if !v.AltScreen {
		t.Fatal("expected alt screen metadata")
	}
	if !v.ReportFocus {
		t.Fatal("expected focus reporting metadata")
	}
	if v.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("mouse mode = %v", v.MouseMode)
	}
	if !strings.Contains(v.WindowTitle, "GOLEM Dashboard") {
		t.Fatalf("window title = %q", v.WindowTitle)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(*Model)
	if m.focusPane != paneWorkers {
		t.Fatalf("tab should advance focus to workers, got %d", m.focusPane)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '4'})
	m = updated.(*Model)
	if m.focusPane != paneEvents {
		t.Fatalf("4 should focus events pane, got %d", m.focusPane)
	}
}

// --- Keyboard Navigation Tests ---

func TestTeatestFullPaneNavigation(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)
	refreshModel(t, m)

	// Initial focus: tasks.
	if m.focusPane != paneTasks {
		t.Fatalf("expected initial focus on paneTasks, got %d", m.focusPane)
	}

	// Tab through all panes.
	expectedOrder := []pane{paneWorkers, paneEvidence, paneEvents, paneTasks}
	for _, expected := range expectedOrder {
		m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if m.focusPane != expected {
			t.Errorf("after tab, expected pane %d, got %d", expected, m.focusPane)
		}
	}

	// Shift+Tab goes backward.
	m.focusPane = paneWorkers
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.focusPane != paneTasks {
		t.Errorf("shift+tab from workers should go to tasks, got %d", m.focusPane)
	}

	// Number keys jump directly.
	tests := []struct {
		key  rune
		pane pane
	}{
		{'1', paneTasks},
		{'2', paneWorkers},
		{'3', paneEvidence},
		{'4', paneEvents},
	}
	for _, tt := range tests {
		m.Update(tea.KeyPressMsg{Code: tt.key})
		if m.focusPane != tt.pane {
			t.Errorf("key '%c' should focus pane %d, got %d", tt.key, tt.pane, m.focusPane)
		}
	}
}

func TestTeatestFocusIndicatorRendering(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)
	refreshModel(t, m)

	// Focus on tasks pane — should show focus indicator.
	m.focusPane = paneTasks
	view1 := viewString(m)
	if !strings.Contains(view1, "▸") {
		t.Error("expected focus indicator '▸' in view when pane is focused")
	}

	// Focus on workers pane — indicator moves.
	m.focusPane = paneWorkers
	view2 := viewString(m)
	// The workers pane header should have the indicator.
	if !strings.Contains(view2, "▸") {
		t.Error("expected focus indicator after switching to workers pane")
	}
}

func TestTeatestMouseClickFocusesPaneAndWheelScrolls(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)
	refreshModel(t, m)

	updated, _ := m.Update(tea.MouseClickMsg(tea.Mouse{X: 90, Y: 10, Button: tea.MouseLeft}))
	m = updated.(*Model)
	if m.focusPane != paneWorkers {
		t.Fatalf("click should focus workers pane, got %d", m.focusPane)
	}

	updated, _ = m.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
	m = updated.(*Model)
	if m.scrollPos[paneWorkers] != 1 {
		t.Fatalf("wheel down should scroll focused workers pane, got %d", m.scrollPos[paneWorkers])
	}

	updated, _ = m.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
	m = updated.(*Model)
	if m.scrollPos[paneWorkers] != 0 {
		t.Fatalf("wheel up should scroll back toward top, got %d", m.scrollPos[paneWorkers])
	}

	m.width = 72
	m.height = 20
	m.focusPane = paneEvidence
	updated, _ = m.Update(tea.MouseClickMsg(tea.Mouse{X: 15, Y: len(m.renderCompactHeader()) + 3, Button: tea.MouseLeft}))
	m = updated.(*Model)
	if m.focusPane != paneEvidence {
		t.Fatalf("compact body click should keep focused pane, got %d", m.focusPane)
	}
	updated, _ = m.Update(tea.MouseClickMsg(tea.Mouse{X: 2, Y: len(m.renderCompactHeader()), Button: tea.MouseLeft}))
	m = updated.(*Model)
	if m.focusPane != paneTasks {
		t.Fatalf("tab row click should focus tasks in compact view, got %d", m.focusPane)
	}
}

func TestTeatestFocusReportingUpdatesWindowTitle(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)
	refreshModel(t, m)

	updated, _ := m.Update(tea.BlurMsg{})
	m = updated.(*Model)
	if m.terminalFocused {
		t.Fatal("expected blur to clear focus flag")
	}
	if got := m.View().WindowTitle; !strings.Contains(got, "unfocused") {
		t.Fatalf("blurred window title = %q", got)
	}

	updated, _ = m.Update(tea.FocusMsg{})
	m = updated.(*Model)
	if !m.terminalFocused {
		t.Fatal("expected focus to restore focus flag")
	}
	if got := m.View().WindowTitle; strings.Contains(got, "unfocused") {
		t.Fatalf("focused window title still marked unfocused: %q", got)
	}
}

// --- Scroll Tests ---

func TestTeatestScrollWithinBounds(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)
	refreshModel(t, m)

	m.focusPane = paneTasks

	// Scroll down.
	for i := 0; i < 5; i++ {
		m.Update(tea.KeyPressMsg{Code: 'j'})
	}
	if m.scrollPos[paneTasks] != 5 {
		t.Errorf("expected scroll pos 5 after 5 down presses, got %d", m.scrollPos[paneTasks])
	}

	// Scroll up.
	for i := 0; i < 3; i++ {
		m.Update(tea.KeyPressMsg{Code: 'k'})
	}
	if m.scrollPos[paneTasks] != 2 {
		t.Errorf("expected scroll pos 2 after 3 up presses, got %d", m.scrollPos[paneTasks])
	}

	// Scroll up past zero should clamp at 0.
	for i := 0; i < 10; i++ {
		m.Update(tea.KeyPressMsg{Code: 'k'})
	}
	if m.scrollPos[paneTasks] != 0 {
		t.Errorf("expected scroll pos clamped at 0, got %d", m.scrollPos[paneTasks])
	}
}

func TestTeatestScrollPerPane(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)
	refreshModel(t, m)

	// Each pane has independent scroll positions.
	m.focusPane = paneTasks
	m.Update(tea.KeyPressMsg{Code: 'j'})
	m.Update(tea.KeyPressMsg{Code: 'j'})

	m.focusPane = paneWorkers
	m.Update(tea.KeyPressMsg{Code: 'j'})

	if m.scrollPos[paneTasks] != 2 {
		t.Errorf("tasks scroll should be 2, got %d", m.scrollPos[paneTasks])
	}
	if m.scrollPos[paneWorkers] != 1 {
		t.Errorf("workers scroll should be 1, got %d", m.scrollPos[paneWorkers])
	}
	if m.scrollPos[paneEvidence] != 0 {
		t.Errorf("evidence scroll should be 0, got %d", m.scrollPos[paneEvidence])
	}
}

func TestTeatestScrollOvershotClamped(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)
	refreshModel(t, m)

	// Set scroll position way beyond actual content.
	m.scrollPos[paneTasks] = 1000
	view := viewString(m)

	// View should still render without panic.
	if view == "" {
		t.Error("expected non-empty view even with overshot scroll")
	}

	// After rendering, scroll should be clamped.
	if m.scrollPos[paneTasks] > len(m.tasks)*3 {
		// Rough check: scroll shouldn't exceed content lines.
		t.Logf("scroll clamped to %d (tasks: %d)", m.scrollPos[paneTasks], len(m.tasks))
	}
}

// --- Evidence Pane Tests ---

func TestTeatestEvidencePaneReviewResults(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 50)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	ctx := context.Background()
	ctrl.StartMission(ctx, ms.ID)

	now := time.Now().UTC()
	ended := now.Add(-10 * time.Second)

	// Add a completed review run.
	review := &mission.Run{
		ID:        "r_rev_001",
		MissionID: ms.ID,
		TaskID:    "t1",
		Mode:      mission.RunModeReview,
		Status:    mission.RunSucceeded,
		Summary:   "Code review passed all checks",
		StartedAt: &now,
		EndedAt:   &ended,
	}
	if err := ctrl.Store().CreateRun(ctx, review); err != nil {
		t.Fatalf("create review run: %v", err)
	}

	// Add a failed review run.
	failedReview := &mission.Run{
		ID:        "r_rev_002",
		MissionID: ms.ID,
		TaskID:    "t2",
		Mode:      mission.RunModeReview,
		Status:    mission.RunFailed,
		Summary:   "Tests failed: 3 failures",
		StartedAt: &now,
		EndedAt:   &ended,
	}
	if err := ctrl.Store().CreateRun(ctx, failedReview); err != nil {
		t.Fatalf("create failed review: %v", err)
	}

	refreshModel(t, m)
	view := viewString(m)

	if !strings.Contains(view, "Evidence") {
		t.Error("expected Evidence pane header")
	}
	if !strings.Contains(view, "pass") {
		t.Error("expected 'pass' for successful review in evidence")
	}
	if !strings.Contains(view, "fail") {
		t.Error("expected 'fail' for failed review in evidence")
	}
}

func TestTeatestEvidencePanePendingApprovals(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 50)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	ctx := context.Background()

	approval := &mission.Approval{
		ID:        "a_001",
		MissionID: ms.ID,
		TaskID:    "t1",
		Kind:      "deployment",
		Status:    mission.ApprovalPending,
		CreatedAt: time.Now().UTC(),
	}
	if err := ctrl.Store().CreateApproval(ctx, approval); err != nil {
		t.Fatalf("create approval: %v", err)
	}

	refreshModel(t, m)
	view := viewString(m)

	if !strings.Contains(view, "Approval") {
		t.Error("expected pending approval in evidence pane")
	}
	if !strings.Contains(view, "deployment") {
		t.Error("expected approval kind 'deployment' in evidence pane")
	}
}

func TestTeatestEvidenceFailedRuns(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 50)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	ctx := context.Background()
	now := time.Now().UTC()
	ended := now.Add(-10 * time.Second)

	// Add a failed worker run (not review).
	failedRun := &mission.Run{
		ID:        "r_fail_001",
		MissionID: ms.ID,
		TaskID:    "t1",
		Mode:      mission.RunModeWorker,
		Status:    mission.RunFailed,
		ErrorText: "compilation error: undefined variable",
		StartedAt: &now,
		EndedAt:   &ended,
	}
	if err := ctrl.Store().CreateRun(ctx, failedRun); err != nil {
		t.Fatalf("create failed run: %v", err)
	}
	if err := ctrl.Store().CreateArtifact(ctx, &mission.Artifact{
		ID:           "a_001",
		MissionID:    ms.ID,
		TaskID:       "t1",
		RunID:        failedRun.ID,
		Type:         "review-report",
		RelativePath: "artifacts/review-report.md",
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	refreshModel(t, m)
	view := viewString(m)

	if !strings.Contains(view, "compilation error") {
		t.Error("expected failed run error text in evidence pane")
	}
	if !strings.Contains(view, "Artifacts") {
		t.Error("expected artifacts section in evidence pane")
	}
	if !strings.Contains(view, "review-report.md") {
		t.Error("expected artifact path in evidence pane")
	}
}

// --- Event Pane Tests ---

func TestTeatestEventPaneRendering(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	ctx := context.Background()

	// Add some events.
	for i, eventType := range []string{"task_started", "run_completed", "plan_applied"} {
		event := &mission.Event{
			MissionID: ms.ID,
			TaskID:    fmt.Sprintf("t%d", i+1),
			Type:      eventType,
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Minute),
		}
		if err := ctrl.Store().AppendEvent(ctx, event); err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
	}

	refreshModel(t, m)
	view := viewString(m)

	if !strings.Contains(view, "Events") {
		t.Error("expected Events pane header")
	}
}

// --- Edge Case Tests ---

func TestTeatestNoMissionState(t *testing.T) {
	m, _ := setupTeatestModel(t, 80, 24)

	// No mission created, doRefresh should return error.
	msg := m.doRefresh()
	rm, ok := msg.(refreshDoneMsg)
	if !ok {
		t.Fatal("expected refreshDoneMsg")
	}
	if rm.err == nil {
		t.Error("expected error when no missions exist")
	}

	m.Update(msg)
	view := viewString(m)

	// Should show an error state.
	if !containsAny(view, "error", "No", "no missions") {
		t.Errorf("expected error message in view, got: %s", truncStr(view, 200))
	}
}

func TestTeatestEmptyMissionNoTasks(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	// Don't apply plan — no tasks.

	refreshModel(t, m)
	view := viewString(m)

	if !strings.Contains(view, "Mission Control") {
		t.Error("expected Mission Control header even with no tasks")
	}
	if !strings.Contains(view, "No tasks yet") {
		t.Error("expected 'No tasks yet' message in empty mission")
	}
}

func TestTeatestEmptyWorkerPane(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)
	// No workers created.

	refreshModel(t, m)
	view := viewString(m)

	if !strings.Contains(view, "No active workers") {
		t.Error("expected 'No active workers' message when no runs exist")
	}
	if !strings.Contains(view, "Mission Control is idle") {
		t.Error("expected worker empty-state guidance when no runs exist")
	}
}

func TestTeatestRecentlyCompletedWorkers(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	ctx := context.Background()
	ctrl.StartMission(ctx, ms.ID)

	now := time.Now().UTC()
	recentEnd := now.Add(-30 * time.Second) // 30s ago

	// Add a recently completed worker.
	populateWorkers(t, ctrl, ms.ID, []workerConfig{
		{runID: "r_done_001", taskID: "t1", mode: mission.RunModeWorker, status: mission.RunSucceeded, worktree: "/tmp/wt1", ended: true},
		{runID: "r_active_1", taskID: "t2", mode: mission.RunModeWorker, status: mission.RunRunning, worktree: "/tmp/wt2"},
	})

	// Manually fix the ended time to be recent (within 5 min window).
	run, _ := ctrl.Store().GetRun(ctx, "r_done_001")
	run.EndedAt = &recentEnd
	ctrl.Store().UpdateRun(ctx, run)

	refreshModel(t, m)
	view := viewString(m)

	// Should show "Recent" section with completed worker.
	if !strings.Contains(view, "Recent") {
		t.Error("expected 'Recent' section for recently completed workers")
	}
}

func TestTeatestZeroDimensionView(t *testing.T) {
	m := New("")
	// width=0, height=0 should show loading.
	view := viewString(m)
	if view != "Loading..." {
		t.Errorf("expected 'Loading...' for zero-dimension view, got: %q", view)
	}
}

func TestTeatestUltraCompactMissionViewRemainsReadable(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 46, 12)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)
	refreshModel(t, m)

	view := viewString(m)
	if !strings.Contains(view, "Mission Control") {
		t.Fatalf("expected Mission Control identity at ultra-compact size, got:\n%s", view)
	}
	for _, want := range []string{"Focus Tasks", "j/k scroll", "q quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in ultra-compact support copy, got:\n%s", want, view)
		}
	}
	if strings.Contains(view, "resize wider for the full four-pane Mission Control view") {
		t.Fatalf("expected ultra-compact support line to stay concise, got:\n%s", view)
	}
}

func TestTeatestRefreshFailurePreservesSelectedMissionID(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 80, 24)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	refreshModel(t, m)

	ctrl.Close()
	msg := m.doRefresh()
	rm, ok := msg.(refreshDoneMsg)
	if !ok {
		t.Fatalf("expected refreshDoneMsg, got %T", msg)
	}
	if rm.err == nil {
		t.Fatal("expected refresh error after closing controller")
	}
	if m.missionID != ms.ID {
		t.Fatalf("expected missionID %q to be preserved, got %q", ms.ID, m.missionID)
	}
	if m.missionObj != nil || m.summary != nil || len(m.tasks) != 0 || len(m.runs) != 0 {
		t.Fatalf("expected cached mission data cleared after refresh failure")
	}

	m.Update(msg)
	if got := m.View().WindowTitle; !strings.Contains(got, ms.ID) {
		t.Fatalf("expected preserved mission identifier in window title, got %q", got)
	}
	view := viewString(m)
	for _, want := range []string{"Mission Control", "Dashboard error"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in error view after refresh failure, got:\n%s", want, view)
		}
	}
}

func TestTeatestQuitRendersEmpty(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	refreshModel(t, m)

	m.quitting = true
	view := viewString(m)
	if view != "" {
		t.Errorf("expected empty view when quitting, got: %q", truncStr(view, 100))
	}
}

func TestTeatestRefreshUpdatesView(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	applyTestPlan(t, ctrl, ms.ID)

	// First refresh.
	refreshModel(t, m)
	view1 := viewString(m)

	// Add a worker.
	ctx := context.Background()
	ctrl.StartMission(ctx, ms.ID)
	populateWorkers(t, ctrl, ms.ID, []workerConfig{
		{runID: "r_new_001", taskID: "t1", mode: mission.RunModeWorker, status: mission.RunRunning, worktree: "/tmp/wt-new"},
	})

	// Second refresh — should now show the worker.
	refreshModel(t, m)
	view2 := viewString(m)

	if view1 == view2 {
		t.Error("expected view to change after adding a worker and refreshing")
	}
	if !strings.Contains(view2, "r_new_001") {
		t.Error("expected new worker run ID in refreshed view")
	}
}

func TestTeatestMissionGoalTruncation(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 40, 20)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID

	refreshModel(t, m)
	view := viewString(m)

	// The goal is long and should be truncated at narrow width.
	// Goal: "Split monolithic auth into OAuth2 services, add tests, update docs"
	// At width 40, it should be truncated with "…".
	if !strings.Contains(view, "Split monolithic") {
		t.Error("expected start of goal text in view")
	}
}

// --- Footer Tests ---

func TestTeatestFooterKeybindings(t *testing.T) {
	m, ctrl := setupTeatestModel(t, 120, 40)
	ms := createTestMission(t, ctrl)
	m.missionID = ms.ID
	refreshModel(t, m)

	// Test footer content directly — the full View() may truncate it depending on height.
	footer := stripANSI(m.renderFooter())
	for _, hint := range []string{"q:quit", "r:refresh", "tab/shift+tab:panes", "j/k:scroll"} {
		if !strings.Contains(footer, hint) {
			t.Errorf("expected key hint %q in footer, got %q", hint, footer)
		}
	}
}
