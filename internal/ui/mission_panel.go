package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/fugue-labs/golem/internal/mission"
	"github.com/fugue-labs/golem/internal/ui/styles"
)

// hasMissionState returns true if there is an active mission to display.
func (m *Model) hasMissionState() bool {
	return m.activeMissionID != ""
}

// missionController returns the lazily-initialized mission controller.
func (m *Model) missionController() *mission.Controller {
	if m.missionCtrl != nil {
		return m.missionCtrl
	}

	// Use a local SQLite database for mission state — no external server needed.
	// Falls back to in-memory store if SQLite fails (e.g. read-only filesystem).
	store, err := mission.OpenSQLiteStore(mission.ResolveSQLitePath())
	if err != nil {
		m.missionCtrl = mission.NewController(mission.NewInMemoryStore())
		return m.missionCtrl
	}
	m.missionCtrl = mission.NewController(store)
	return m.missionCtrl
}

// renderMissionPanelLines renders mission status for the workflow panel.
func (m *Model) renderMissionPanelLines(limit, width int) []string {
	if limit <= 0 || !m.hasMissionState() {
		return nil
	}

	ctrl := m.missionController()
	if ctrl == nil {
		return nil
	}

	ctx := m.appCtx
	summary, err := ctrl.GetMissionSummary(ctx, m.activeMissionID)
	if err != nil {
		return []string{m.sty.Panel.Progress.Render("Mission: error")}
	}

	ms := summary.Mission
	tc := summary.TaskCounts
	statusSummary := fmt.Sprintf("%s %s", missionStatusIcon(ms.Status), ms.Status)
	if summary.PendingApprovals > 0 {
		statusSummary += fmt.Sprintf(" · %d approvals", summary.PendingApprovals)
	}
	lines := []string{m.renderPanelSectionHeader(m.panelSectionTitle("Mission"), statusSummary+" · "+m.missionPanelSummaryWidth(max(12, width-12)), width)}
	if limit == 1 {
		return lines
	}

	if title := strings.TrimSpace(ms.Title); title != "" {
		lines = append(lines, m.renderPanelDetail("title", title, width))
		if len(lines) >= limit {
			return lines[:limit]
		}
	}
	goalText := strings.TrimSpace(ms.Goal)
	if goalText == "" {
		goalText = strings.TrimSpace(ms.Title)
	}
	if goalText != "" {
		lines = append(lines, m.renderPanelDetail("goal", goalText, width))
		if len(lines) >= limit {
			return lines[:limit]
		}
	}

	if tc.Total > 0 {
		var countParts []string
		if tc.Done > 0 || tc.Integrated > 0 || tc.Accepted > 0 {
			countParts = append(countParts, fmt.Sprintf("%d✓", tc.Done+tc.Integrated+tc.Accepted))
		}
		if tc.Running > 0 {
			countParts = append(countParts, fmt.Sprintf("%d◐", tc.Running))
		}
		if tc.Ready > 0 {
			countParts = append(countParts, fmt.Sprintf("%d●", tc.Ready))
		}
		if tc.Blocked > 0 {
			countParts = append(countParts, fmt.Sprintf("%d✗", tc.Blocked))
		}
		if tc.Pending > 0 {
			countParts = append(countParts, fmt.Sprintf("%d○", tc.Pending))
		}
		lines = append(lines, m.renderPanelDetail("Tasks", strings.Join(countParts, " "), width))
		if len(lines) >= limit {
			return lines[:limit]
		}
	}

	runs, _ := ctrl.Store().ListRuns(ctx, m.activeMissionID)
	if activeRuns := filterActiveRuns(runs); len(activeRuns) > 0 && len(lines) < limit {
		allTasks, _ := ctrl.Store().ListTasks(ctx, m.activeMissionID)
		taskMap := make(map[string]*mission.Task, len(allTasks))
		for _, t := range allTasks {
			taskMap[t.ID] = t
		}
		workerBudget := limit - len(lines)
		if len(allTasks) > 0 && workerBudget > len(activeRuns)+2 {
			workerBudget = min(workerBudget-2, len(activeRuns))
		} else {
			workerBudget = min(workerBudget, len(activeRuns))
		}
		for i := range workerBudget {
			r := activeRuns[i]
			card := m.renderWorkerCard(r, taskMap[r.TaskID], width)
			lines = append(lines, card)
		}
		if len(activeRuns) > workerBudget && len(lines) < limit {
			lines = append(lines, m.renderPanelOverflow("workers", len(activeRuns)-workerBudget))
		}
	}

	tasks, _ := ctrl.Store().ListTasks(ctx, m.activeMissionID)
	itemBudget := limit - len(lines)
	if itemBudget <= 0 {
		return lines[:limit]
	}
	maxItems := min(itemBudget, len(tasks))
	if len(tasks) > itemBudget && itemBudget > 0 {
		maxItems = itemBudget - 1
	}
	for i := range maxItems {
		t := tasks[i]
		icon := m.taskIcon(t.Status)
		desc := ansi.Truncate(t.Title, max(1, width-4), "…")
		if t.Status == mission.TaskDone || t.Status == mission.TaskIntegrated {
			desc = m.sty.Panel.TaskDone.Render(desc)
		} else {
			desc = m.sty.Panel.TaskText.Render(desc)
		}
		lines = append(lines, fmt.Sprintf(" %s %s", icon, desc))
	}
	remaining := len(tasks) - maxItems
	if remaining > 0 && len(lines) < limit {
		lines = append(lines, m.renderPanelOverflow("tasks", remaining))
	}
	for len(lines) < limit {
		lines = append(lines, "")
	}
	return lines[:limit]
}

// missionPanelSummary returns a compact summary string for the panel header.
func (m *Model) missionPanelSummary() string {
	return m.missionPanelSummaryWidth(28)
}

func (m *Model) missionPanelSummaryWidth(width int) string {
	if !m.hasMissionState() {
		return ""
	}

	ctrl := m.missionController()
	if ctrl == nil {
		return ""
	}

	ctx := m.appCtx
	summary, err := ctrl.GetMissionSummary(ctx, m.activeMissionID)
	if err != nil {
		return ""
	}

	tc := summary.TaskCounts
	done := tc.Done + tc.Integrated + tc.Accepted
	if width < 12 {
		return fmt.Sprintf("%d/%d", done, tc.Total)
	}
	base := fmt.Sprintf("%d/%d done", done, tc.Total)
	if width < 20 {
		return base
	}
	if width < 30 {
		parts := []string{base}
		if tc.Ready > 0 {
			parts = append(parts, fmt.Sprintf("%d ready", tc.Ready))
		} else if summary.ActiveRuns > 0 {
			parts = append(parts, fmt.Sprintf("%d active", summary.ActiveRuns))
		}
		return strings.Join(parts, " · ")
	}
	parts := []string{base}
	if tc.Running > 0 {
		parts = append(parts, fmt.Sprintf("%d running", tc.Running))
	}
	if tc.Ready > 0 {
		parts = append(parts, fmt.Sprintf("%d ready", tc.Ready))
	}
	if tc.Blocked > 0 {
		parts = append(parts, fmt.Sprintf("%d blocked", tc.Blocked))
	}
	if summary.ActiveRuns > 0 {
		parts = append(parts, fmt.Sprintf("%d workers", summary.ActiveRuns))
	}
	return strings.Join(parts, " · ")
}

func missionStatusIcon(s mission.MissionStatus) string {
	switch s {
	case mission.MissionCompleted:
		return styles.CheckIcon
	case mission.MissionRunning:
		return styles.InProgressIcon
	case mission.MissionPaused:
		return "⏸"
	case mission.MissionBlocked:
		return styles.BlockedIcon
	case mission.MissionFailed, mission.MissionCancelled:
		return styles.ErrorIcon
	case mission.MissionAwaitingApproval:
		return "⏳"
	case mission.MissionPlanning:
		return styles.SpinnerIcon
	default:
		return styles.HollowIcon
	}
}

// renderWorkerCard renders a compact status card for a running worker.
// Format: " ◐ TaskTitle (2m13s) [worktree-dir]"
func (m *Model) renderWorkerCard(run *mission.Run, task *mission.Task, width int) string {
	icon := m.sty.Panel.IconInProgress.Render(styles.InProgressIcon)

	var label string
	if task != nil {
		label = task.Title
	} else {
		label = run.TaskID
	}

	var dur string
	if run.StartedAt != nil {
		dur = formatDuration(time.Since(*run.StartedAt))
	}

	var wtDir string
	if run.WorktreePath != "" {
		wtDir = filepath.Base(run.WorktreePath)
	}

	// Build suffix: " (2m13s) [worker-t1]"
	var suffix string
	if dur != "" && wtDir != "" {
		suffix = fmt.Sprintf(" (%s) [%s]", dur, wtDir)
	} else if dur != "" {
		suffix = fmt.Sprintf(" (%s)", dur)
	} else if wtDir != "" {
		suffix = fmt.Sprintf(" [%s]", wtDir)
	}

	// Truncate label to fit within width, accounting for icon + suffix.
	maxLabel := max(1, width-4-len(suffix))
	label = ansi.Truncate(label, maxLabel, "…")
	label = m.sty.Panel.TaskText.Render(label)
	suffix = m.sty.Muted.Render(suffix)

	return fmt.Sprintf(" %s %s%s", icon, label, suffix)
}

// filterActiveRuns returns only runs with status RunRunning.
func filterActiveRuns(runs []*mission.Run) []*mission.Run {
	var active []*mission.Run
	for _, r := range runs {
		if r.Status == mission.RunRunning {
			active = append(active, r)
		}
	}
	return active
}

func (m *Model) taskIcon(s mission.TaskStatus) string {
	switch s {
	case mission.TaskDone, mission.TaskIntegrated, mission.TaskAccepted:
		return m.sty.Panel.IconCompleted.Render(styles.CheckIcon)
	case mission.TaskRunning, mission.TaskLeased:
		return m.sty.Panel.IconInProgress.Render(styles.InProgressIcon)
	case mission.TaskBlocked, mission.TaskFailed, mission.TaskRejected:
		return m.sty.Panel.IconBlocked.Render(styles.BlockedIcon)
	case mission.TaskReady:
		return m.sty.Panel.IconInProgress.Render(styles.PendingIcon)
	case mission.TaskAwaitingReview:
		return m.sty.Panel.IconInProgress.Render("◎")
	default:
		return m.sty.Panel.IconPending.Render(styles.HollowIcon)
	}
}
