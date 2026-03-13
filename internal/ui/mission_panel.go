package ui

import (
	"context"
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

	// Initialize mission store on first use (connects to Dolt server).
	store, err := mission.OpenDoltStore(mission.DefaultDSN())
	if err != nil {
		return nil
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

	ctx := context.Background()
	summary, err := ctrl.GetMissionSummary(ctx, m.activeMissionID)
	if err != nil {
		return []string{m.sty.Panel.Progress.Render("Mission: error")}
	}

	ms := summary.Mission
	tc := summary.TaskCounts

	// Header line: Mission status.
	statusIcon := missionStatusIcon(ms.Status)
	header := fmt.Sprintf("Mission %s %s", statusIcon, ms.Status)
	lines := []string{m.sty.Panel.Progress.Render(header)}
	if limit == 1 {
		return lines
	}

	// Title line (truncated).
	title := ansi.Truncate(ms.Title, max(1, width-2), "…")
	lines = append(lines, m.sty.Panel.TaskText.Render(title))
	if len(lines) >= limit {
		return lines[:limit]
	}

	// Task summary counts.
	if tc.Total > 0 {
		var countParts []string
		if tc.Done > 0 {
			countParts = append(countParts, fmt.Sprintf("%d✓", tc.Done))
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
		taskLine := fmt.Sprintf("Tasks %d/%d %s", tc.Done+tc.Integrated+tc.Accepted, tc.Total, strings.Join(countParts, " "))
		lines = append(lines, m.sty.Panel.Progress.Render(taskLine))
		if len(lines) >= limit {
			return lines[:limit]
		}
	}

	// Active worker status cards.
	runs, _ := ctrl.Store().ListRuns(ctx, m.activeMissionID)
	if activeRuns := filterActiveRuns(runs); len(activeRuns) > 0 && len(lines) < limit {
		// Build a task lookup for worker card labels.
		allTasks, _ := ctrl.Store().ListTasks(ctx, m.activeMissionID)
		taskMap := make(map[string]*mission.Task, len(allTasks))
		for _, t := range allTasks {
			taskMap[t.ID] = t
		}

		workerBudget := limit - len(lines)
		// Reserve at least 2 lines for task list if there are tasks.
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
			lines = append(lines, m.sty.Muted.Render(fmt.Sprintf("… +%d workers", len(activeRuns)-workerBudget)))
		}
	}

	// Individual tasks (remaining space).
	tasks, _ := ctrl.Store().ListTasks(ctx, m.activeMissionID)
	itemBudget := limit - len(lines)
	if itemBudget <= 0 {
		return lines[:limit]
	}

	maxItems := min(itemBudget, len(tasks))
	if len(tasks) > itemBudget && itemBudget > 0 {
		maxItems = itemBudget - 1 // leave room for "... +N" line
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
		lines = append(lines, m.sty.Muted.Render(fmt.Sprintf("… +%d tasks", remaining)))
	}

	for len(lines) < limit {
		lines = append(lines, "")
	}
	return lines[:limit]
}

// missionPanelSummary returns a compact summary string for the panel header.
func (m *Model) missionPanelSummary() string {
	if !m.hasMissionState() {
		return ""
	}

	ctrl := m.missionController()
	if ctrl == nil {
		return ""
	}

	ctx := context.Background()
	summary, err := ctrl.GetMissionSummary(ctx, m.activeMissionID)
	if err != nil {
		return ""
	}

	tc := summary.TaskCounts
	done := tc.Done + tc.Integrated + tc.Accepted
	base := fmt.Sprintf("mission %d/%d", done, tc.Total)
	if summary.ActiveRuns > 0 {
		return fmt.Sprintf("%s (%d workers)", base, summary.ActiveRuns)
	}
	return base
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
