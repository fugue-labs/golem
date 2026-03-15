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
	tasks, _ := ctrl.Store().ListTasks(ctx, m.activeMissionID)
	runs, _ := ctrl.Store().ListRuns(ctx, m.activeMissionID)
	approvals, _ := ctrl.Store().ListApprovals(ctx, m.activeMissionID)

	focusTask := missionFocusTask(tasks)
	nextTask := missionNextTask(tasks, focusTask)
	statusIcon := missionStatusIcon(ms.Status)
	header := fmt.Sprintf("Mission %s %s", statusIcon, ms.Status)
	if ms.Status == mission.MissionBlocked {
		header = fmt.Sprintf("Mission %s blocked", styles.BlockedIcon)
	} else if ms.Status == mission.MissionAwaitingApproval || pendingApprovalsCount(approvals) > 0 || summary.PendingApprovals > 0 {
		header = "Mission ⏳ awaiting approval"
	}
	lines := []string{m.workflowProgressLine(header, width)}
	if limit == 1 {
		return lines
	}

	if focusTask != nil {
		lines = append(lines, m.renderMissionSpotlight(ms.Status, focusTask, approvals, width))
	}
	if len(lines) >= limit {
		return lines[:limit]
	}

	if nextTask != nil && (focusTask == nil || nextTask.ID != focusTask.ID) {
		lines = append(lines, m.workflowBullet(m.sty.Panel.IconPending.Render(styles.PendingIcon), "Next: "+nextTask.Title, width, false))
	}
	if len(lines) >= limit {
		return lines[:limit]
	}

	progress := missionProgressSummary(tc)
	if progress != "" {
		lines = append(lines, m.workflowProgressLine(progress, width))
	}
	if len(lines) >= limit {
		return lines[:limit]
	}

	title := ms.Title
	if strings.TrimSpace(title) == "" {
		title = ms.Goal
	}
	lines = append(lines, m.workflowBullet(m.sty.Panel.IconPending.Render(styles.HollowIcon), title, width, false))
	if len(lines) >= limit {
		return lines[:limit]
	}

	activeRuns := filterActiveRuns(runs)
	for i := 0; i < len(activeRuns) && len(lines) < limit; i++ {
		task := missionTaskByID(tasks, activeRuns[i].TaskID)
		lines = append(lines, m.renderWorkerCard(activeRuns[i], task, width))
	}
	for i := range tasks {
		if len(lines) >= limit {
			break
		}
		task := tasks[i]
		if focusTask != nil && task.ID == focusTask.ID {
			continue
		}
		if nextTask != nil && task.ID == nextTask.ID {
			continue
		}
		icon := m.taskIcon(task.Status)
		done := task.Status == mission.TaskDone || task.Status == mission.TaskIntegrated || task.Status == mission.TaskAccepted
		lines = append(lines, m.workflowBullet(icon, task.Title, width, done))
	}
	if len(lines) < limit {
		remaining := len(tasks) + len(activeRuns) + 4 - len(lines)
		if remaining > 0 {
			lines = append(lines, m.sty.Muted.Render(ansi.Truncate(fmt.Sprintf("… +%d mission details", remaining), max(1, width), "…")))
		}
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

	ctx := m.appCtx
	summary, err := ctrl.GetMissionSummary(ctx, m.activeMissionID)
	if err != nil {
		return ""
	}

	tc := summary.TaskCounts
	done := tc.Done + tc.Integrated + tc.Accepted
	switch summary.Mission.Status {
	case mission.MissionBlocked:
		return fmt.Sprintf("mission blocked · %d/%d", done, tc.Total)
	case mission.MissionAwaitingApproval:
		return fmt.Sprintf("mission awaiting approval · %d/%d", done, tc.Total)
	}
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

	var suffix string
	if dur != "" && wtDir != "" {
		suffix = fmt.Sprintf(" (%s) [%s]", dur, wtDir)
	} else if dur != "" {
		suffix = fmt.Sprintf(" (%s)", dur)
	} else if wtDir != "" {
		suffix = fmt.Sprintf(" [%s]", wtDir)
	}

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

func missionFocusTask(tasks []*mission.Task) *mission.Task {
	for i := range tasks {
		if tasks[i].Status == mission.TaskBlocked {
			return tasks[i]
		}
	}
	for i := range tasks {
		if tasks[i].Status == mission.TaskRunning || tasks[i].Status == mission.TaskLeased {
			return tasks[i]
		}
	}
	for i := range tasks {
		if tasks[i].Status == mission.TaskAwaitingReview {
			return tasks[i]
		}
	}
	for i := range tasks {
		if tasks[i].Status == mission.TaskReady {
			return tasks[i]
		}
	}
	for i := range tasks {
		if tasks[i].Status == mission.TaskPending {
			return tasks[i]
		}
	}
	if len(tasks) == 0 {
		return nil
	}
	return tasks[0]
}

func missionNextTask(tasks []*mission.Task, focus *mission.Task) *mission.Task {
	for i := range tasks {
		if focus != nil && tasks[i].ID == focus.ID {
			continue
		}
		if tasks[i].Status == mission.TaskReady || tasks[i].Status == mission.TaskPending {
			return tasks[i]
		}
	}
	return nil
}

func missionTaskByID(tasks []*mission.Task, id string) *mission.Task {
	for i := range tasks {
		if tasks[i].ID == id {
			return tasks[i]
		}
	}
	return nil
}

func missionProgressSummary(tc mission.TaskCounts) string {
	parts := []string{}
	if tc.Total > 0 {
		parts = append(parts, fmt.Sprintf("Tasks %d/%d", tc.Done+tc.Integrated+tc.Accepted, tc.Total))
	}
	if tc.Done+tc.Integrated+tc.Accepted > 0 {
		parts = append(parts, fmt.Sprintf("%d✓", tc.Done+tc.Integrated+tc.Accepted))
	}
	if tc.Running > 0 {
		parts = append(parts, fmt.Sprintf("%d◐", tc.Running))
	}
	if tc.Ready > 0 {
		parts = append(parts, fmt.Sprintf("%d●", tc.Ready))
	}
	if tc.Blocked > 0 {
		parts = append(parts, fmt.Sprintf("%d✗", tc.Blocked))
	}
	if tc.Pending > 0 {
		parts = append(parts, fmt.Sprintf("%d○", tc.Pending))
	}
	if tc.AwaitingReview > 0 {
		parts = append(parts, fmt.Sprintf("%d◎", tc.AwaitingReview))
	}
	return strings.Join(parts, " · ")
}

func pendingApprovalsCount(approvals []*mission.Approval) int {
	count := 0
	for _, approval := range approvals {
		if approval.Status == mission.ApprovalPending {
			count++
		}
	}
	return count
}

func (m *Model) renderMissionSpotlight(status mission.MissionStatus, task *mission.Task, approvals []*mission.Approval, width int) string {
	icon := m.taskIcon(task.Status)
	prefix := "Focus: "
	label := task.Title
	switch {
	case status == mission.MissionAwaitingApproval || pendingApprovalsCount(approvals) > 0:
		icon = m.sty.Panel.IconPending.Render("⏳")
		prefix = "Approval: "
		label = "Review mission plan and approve start"
	case task.Status == mission.TaskBlocked:
		prefix = "Blocked: "
		if task.BlockingReason != "" {
			label += " — " + task.BlockingReason
		}
	case task.Status == mission.TaskRunning || task.Status == mission.TaskLeased:
		prefix = "In progress: "
	case task.Status == mission.TaskAwaitingReview:
		icon = m.sty.Panel.IconInProgress.Render("◎")
		prefix = "Review: "
	case task.Status == mission.TaskReady:
		icon = m.sty.Panel.IconPending.Render(styles.PendingIcon)
		prefix = "Next action: "
	case task.Status == mission.TaskPending:
		icon = m.sty.Panel.IconPending.Render(styles.HollowIcon)
		prefix = "Queued: "
	case task.Status == mission.TaskDone || task.Status == mission.TaskIntegrated || task.Status == mission.TaskAccepted:
		prefix = "Done: "
	}
	return m.workflowBullet(icon, prefix+label, width, task.Status == mission.TaskDone || task.Status == mission.TaskIntegrated || task.Status == mission.TaskAccepted)
}
