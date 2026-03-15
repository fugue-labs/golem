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
	if summary == nil {
		return nil
	}
	summary.FillDisplayDefaults()

	ms := summary.Mission
	tasks, _ := ctrl.Store().ListTasks(ctx, m.activeMissionID)
	runs, _ := ctrl.Store().ListRuns(ctx, m.activeMissionID)
	focusTask := missionTaskByID(tasks, missionSummaryTaskID(summary.FocusTask))
	nextTask := missionTaskByID(tasks, missionSummaryTaskID(summary.NextTask))

	lines := []string{m.workflowProgressLine(missionPanelHeader(summary), width)}
	if limit == 1 {
		return lines
	}

	if focusTask != nil || summary.NextAction != "" {
		lines = append(lines, m.renderMissionSpotlight(summary, focusTask, width))
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

	if progress := missionProgressSummary(summary); progress != "" {
		lines = append(lines, m.workflowProgressLine(progress, width))
	}
	if len(lines) >= limit {
		return lines[:limit]
	}

	title := strings.TrimSpace(ms.Title)
	if title == "" {
		title = ms.Goal
	}
	lines = append(lines, m.workflowBullet(m.sty.Panel.IconPending.Render(styles.HollowIcon), title, width, false))
	if len(lines) >= limit {
		return lines[:limit]
	}

	for _, run := range filterActiveRuns(runs) {
		if len(lines) >= limit {
			break
		}
		lines = append(lines, m.renderWorkerCard(run, missionTaskByID(tasks, run.TaskID), width))
	}
	for _, task := range tasks {
		if len(lines) >= limit {
			break
		}
		if focusTask != nil && task.ID == focusTask.ID {
			continue
		}
		if nextTask != nil && task.ID == nextTask.ID {
			continue
		}
		done := task.Status == mission.TaskDone || task.Status == mission.TaskIntegrated || task.Status == mission.TaskAccepted
		lines = append(lines, m.workflowBullet(m.taskIcon(task.Status), task.Title, width, done))
	}
	if len(lines) < limit {
		remaining := len(tasks) + len(filterActiveRuns(runs)) + 4 - len(lines)
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
	summary, err := ctrl.GetMissionSummary(m.appCtx, m.activeMissionID)
	if err != nil {
		return ""
	}
	if summary == nil {
		return ""
	}
	summary.FillDisplayDefaults()

	tc := summary.TaskCounts
	done := tc.Completed()
	switch {
	case summary.HasApprovalGate():
		return fmt.Sprintf("mission awaiting approval · %d/%d", done, tc.Total)
	case summary.HasBlockers():
		return fmt.Sprintf("mission blocked · %d/%d", done, tc.Total)
	case summary.HasBlockedTasks():
		return fmt.Sprintf("mission attention · %d/%d (%d blocked)", done, tc.Total, tc.Blocked)
	case summary.HasPendingApprovals():
		return fmt.Sprintf("mission attention · %d/%d (%d approvals)", done, tc.Total, summary.PendingApprovals)
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
func (m *Model) renderWorkerCard(run *mission.Run, task *mission.Task, width int) string {
	icon := m.sty.Panel.IconInProgress.Render(styles.InProgressIcon)
	label := run.TaskID
	if task != nil {
		label = task.Title
	}
	var dur string
	if run.StartedAt != nil {
		dur = formatDuration(time.Since(*run.StartedAt))
	}
	var wtDir string
	if run.WorktreePath != "" {
		wtDir = filepath.Base(run.WorktreePath)
	}
	suffix := ""
	switch {
	case dur != "" && wtDir != "":
		suffix = fmt.Sprintf(" (%s) [%s]", dur, wtDir)
	case dur != "":
		suffix = fmt.Sprintf(" (%s)", dur)
	case wtDir != "":
		suffix = fmt.Sprintf(" [%s]", wtDir)
	}
	maxLabel := max(1, width-4-len(suffix))
	label = m.sty.Panel.TaskText.Render(ansi.Truncate(label, maxLabel, "…"))
	return fmt.Sprintf(" %s %s%s", icon, label, m.sty.Muted.Render(suffix))
}

func filterActiveRuns(runs []*mission.Run) []*mission.Run {
	active := make([]*mission.Run, 0, len(runs))
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

func missionTaskByID(tasks []*mission.Task, id string) *mission.Task {
	for _, task := range tasks {
		if task.ID == id {
			return task
		}
	}
	return nil
}

func missionProgressSummary(summary *mission.MissionSummary) string {
	if summary == nil {
		return ""
	}
	tc := summary.TaskCounts
	parts := make([]string, 0, 8)
	if tc.Total > 0 {
		parts = append(parts, fmt.Sprintf("Tasks %d/%d", tc.Completed(), tc.Total))
	}
	if summary.DependencyEdges > 0 {
		parts = append(parts, fmt.Sprintf("DAG %d edge(s)", summary.DependencyEdges))
	}
	if tc.Completed() > 0 {
		parts = append(parts, fmt.Sprintf("%d✓", tc.Completed()))
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
	if summary.PendingApprovals > 0 {
		parts = append(parts, fmt.Sprintf("%d⏳", summary.PendingApprovals))
	}
	return strings.Join(parts, " · ")
}

func missionSummaryTaskID(task *mission.MissionTaskView) string {
	if task == nil {
		return ""
	}
	return task.ID
}

func missionPanelHeader(summary *mission.MissionSummary) string {
	if summary == nil || summary.Mission == nil {
		return "Mission"
	}
	icon := missionStatusIcon(summary.Mission.Status)
	switch {
	case summary.HasApprovalGate():
		return "Mission ⏳ awaiting approval"
	case summary.HasBlockers():
		return fmt.Sprintf("Mission %s blocked", styles.BlockedIcon)
	case strings.TrimSpace(summary.PhaseLabel) != "":
		return fmt.Sprintf("Mission %s %s", icon, strings.ToLower(summary.PhaseLabel))
	default:
		return fmt.Sprintf("Mission %s %s", icon, summary.Mission.Status)
	}
}

func (m *Model) renderMissionSpotlight(summary *mission.MissionSummary, task *mission.Task, width int) string {
	if summary == nil {
		return ""
	}
	if task == nil {
		icon := m.sty.Panel.IconPending.Render(styles.PendingIcon)
		prefix := "Next action: "
		if summary.HasApprovalGate() {
			icon = m.sty.Panel.IconPending.Render("⏳")
			prefix = "Approval: "
		} else if summary.HasPendingApprovals() {
			icon = m.sty.Panel.IconPending.Render("⏳")
			prefix = "Attention: "
		} else if summary.HasBlockedTasks() {
			icon = m.sty.Panel.IconBlocked.Render(styles.BlockedIcon)
			prefix = "Blocked: "
		}
		label := strings.TrimSpace(summary.NextAction)
		if label == "" {
			label = "Mission state is up to date"
		}
		return m.workflowBullet(icon, prefix+label, width, false)
	}

	icon := m.taskIcon(task.Status)
	prefix := "Focus: "
	label := task.Title
	switch {
	case summary.HasApprovalGate():
		icon = m.sty.Panel.IconPending.Render("⏳")
		prefix = "Approval: "
		label = summary.NextAction
	case task.Status == mission.TaskBlocked:
		prefix = "Blocked: "
		if task.BlockingReason != "" {
			label += " — " + task.BlockingReason
		}
	case summary.HasPendingApprovals() && task.Status != mission.TaskRunning && task.Status != mission.TaskLeased:
		icon = m.sty.Panel.IconPending.Render("⏳")
		prefix = "Attention: "
		label = summary.NextAction
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
	if strings.TrimSpace(summary.NextAction) != "" && (summary.HasApprovalGate() || (summary.HasPendingApprovals() && task.Status != mission.TaskRunning && task.Status != mission.TaskLeased) || task.Status == mission.TaskReady || task.Status == mission.TaskPending) {
		label = summary.NextAction
	}
	done := task.Status == mission.TaskDone || task.Status == mission.TaskIntegrated || task.Status == mission.TaskAccepted
	return m.workflowBullet(icon, prefix+label, width, done)
}
