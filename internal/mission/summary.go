package mission

import (
	"context"
	"fmt"
)

// BuildMissionSummary derives a mission summary from durable mission, task,
// dependency, run, and approval state so the TUI can render phase, blockers,
// next actions, and DAG progress without relying on transcript text.
func BuildMissionSummary(ctx context.Context, store Store, missionID string) (*MissionSummary, error) {
	m, err := store.GetMission(ctx, missionID)
	if err != nil {
		return nil, err
	}
	tasks, err := store.ListTasks(ctx, missionID)
	if err != nil {
		return nil, err
	}
	deps, err := store.ListDependencies(ctx, missionID)
	if err != nil {
		return nil, err
	}
	runs, err := store.ListRuns(ctx, missionID)
	if err != nil {
		return nil, err
	}
	approvals, err := store.ListApprovals(ctx, missionID)
	if err != nil {
		return nil, err
	}

	depMap := make(map[string][]string, len(tasks))
	for _, dep := range deps {
		depMap[dep.TaskID] = append(depMap[dep.TaskID], dep.DependsOnID)
	}

	var counts TaskCounts
	for _, task := range tasks {
		counts.Total++
		switch task.Status {
		case TaskPending:
			counts.Pending++
		case TaskReady:
			counts.Ready++
		case TaskRunning, TaskLeased:
			counts.Running++
		case TaskAwaitingReview:
			counts.AwaitingReview++
		case TaskAccepted:
			counts.Accepted++
		case TaskIntegrated:
			counts.Integrated++
		case TaskDone:
			counts.Done++
		case TaskBlocked:
			counts.Blocked++
		case TaskFailed, TaskRejected:
			counts.Failed++
		}
	}

	activeRuns := 0
	for _, run := range runs {
		if run.Status == RunQueued || run.Status == RunRunning {
			activeRuns++
		}
	}

	pendingApprovals := 0
	for _, approval := range approvals {
		if approval.Status == ApprovalPending {
			pendingApprovals++
		}
	}

	summary := &MissionSummary{
		Mission:          m,
		TaskCounts:       counts,
		ActiveRuns:       activeRuns,
		PendingApprovals: pendingApprovals,
		DependencyEdges:  len(deps),
	}

	for _, task := range tasks {
		view := MissionTaskView{
			ID:             task.ID,
			Title:          task.Title,
			Status:         task.Status,
			BlockingReason: task.BlockingReason,
			DependsOn:      append([]string(nil), depMap[task.ID]...),
		}
		switch task.Status {
		case TaskRunning, TaskLeased:
			summary.RunningTasks = append(summary.RunningTasks, view)
		case TaskAwaitingReview:
			summary.ReviewTasks = append(summary.ReviewTasks, view)
		case TaskReady:
			summary.ReadyTasks = append(summary.ReadyTasks, view)
		case TaskBlocked:
			summary.BlockedTasks = append(summary.BlockedTasks, view)
		}
	}

	focus := selectMissionTaskView(tasks, depMap,
		TaskBlocked,
		TaskRunning, TaskLeased,
		TaskAwaitingReview,
		TaskReady,
		TaskPending,
		TaskAccepted,
		TaskIntegrated,
		TaskDone,
		TaskFailed,
		TaskRejected,
	)
	if focus != nil {
		summary.FocusTask = focus
	}
	next := selectMissionNextTask(tasks, depMap, focus)
	if next != nil {
		summary.NextTask = next
	}

	summary.PhaseLabel = missionPhaseLabel(summary)
	summary.NextAction = missionNextAction(summary)
	summary.Attention = missionAttention(summary)

	return summary, nil
}

func selectMissionTaskView(tasks []*Task, depMap map[string][]string, statuses ...TaskStatus) *MissionTaskView {
	for _, status := range statuses {
		for _, task := range tasks {
			if task.Status != status {
				continue
			}
			return &MissionTaskView{
				ID:             task.ID,
				Title:          task.Title,
				Status:         task.Status,
				BlockingReason: task.BlockingReason,
				DependsOn:      append([]string(nil), depMap[task.ID]...),
			}
		}
	}
	return nil
}

func selectMissionNextTask(tasks []*Task, depMap map[string][]string, focus *MissionTaskView) *MissionTaskView {
	for _, task := range tasks {
		if focus != nil && task.ID == focus.ID {
			continue
		}
		if task.Status != TaskReady && task.Status != TaskPending {
			continue
		}
		return &MissionTaskView{
			ID:             task.ID,
			Title:          task.Title,
			Status:         task.Status,
			BlockingReason: task.BlockingReason,
			DependsOn:      append([]string(nil), depMap[task.ID]...),
		}
	}
	return nil
}

func missionPhaseLabel(summary *MissionSummary) string {
	if summary == nil || summary.Mission == nil {
		return ""
	}
	if summary.HasApprovalGate() {
		return "Awaiting approval"
	}
	switch summary.Mission.Status {
	case MissionDraft:
		return "Draft"
	case MissionPlanning:
		return "Planning"
	case MissionRunning:
		if summary.TaskCounts.AwaitingReview > 0 {
			return "Running · review"
		}
		if summary.ActiveRuns > 0 {
			return "Running"
		}
		if summary.TaskCounts.Ready > 0 {
			return "Running · ready queue"
		}
		return "Running"
	case MissionBlocked:
		return "Blocked"
	case MissionPaused:
		return "Paused"
	case MissionCompleting:
		return "Completing"
	case MissionCompleted:
		return "Completed"
	case MissionFailed:
		return "Failed"
	case MissionCancelled:
		return "Cancelled"
	default:
		return string(summary.Mission.Status)
	}
}

func missionNextAction(summary *MissionSummary) string {
	if summary == nil || summary.Mission == nil {
		return ""
	}
	if summary.PendingApprovals > 0 || summary.Mission.Status == MissionAwaitingApproval {
		return "Review the proposed mission plan and approve start with /mission approve"
	}
	switch summary.Mission.Status {
	case MissionDraft:
		return "Generate a task DAG with /mission plan"
	case MissionPlanning:
		return "Wait for planning to finish, then review the DAG"
	case MissionPaused:
		return "Resume mission execution with /mission start"
	case MissionBlocked:
		if summary.FocusTask != nil && summary.FocusTask.BlockingReason != "" {
			return fmt.Sprintf("Unblock %s: %s", summary.FocusTask.Title, summary.FocusTask.BlockingReason)
		}
		return "Resolve the blocking issue or cancel the mission"
	case MissionCompleted:
		return "Mission complete"
	case MissionCancelled:
		return "Mission cancelled"
	case MissionFailed:
		return "Inspect failures before retrying or replanning"
	}
	if summary.TaskCounts.AwaitingReview > 0 && summary.FocusTask != nil {
		return fmt.Sprintf("Review %s before integration can proceed", summary.FocusTask.Title)
	}
	if summary.ActiveRuns > 0 && summary.FocusTask != nil {
		return fmt.Sprintf("Monitor active work on %s", summary.FocusTask.Title)
	}
	if summary.NextTask != nil {
		return fmt.Sprintf("Next ready task: %s", summary.NextTask.Title)
	}
	if summary.TaskCounts.Remaining() == 0 && summary.TaskCounts.Total > 0 {
		return "Wait for final integration or completion checks"
	}
	return "Mission state is up to date"
}

func missionAttention(summary *MissionSummary) string {
	if summary == nil || summary.Mission == nil {
		return ""
	}
	if summary.PendingApprovals > 0 {
		return fmt.Sprintf("%d approval gate(s) pending", summary.PendingApprovals)
	}
	if summary.TaskCounts.Blocked > 0 {
		return fmt.Sprintf("%d blocked task(s)", summary.TaskCounts.Blocked)
	}
	if summary.TaskCounts.AwaitingReview > 0 {
		return fmt.Sprintf("%d task(s) awaiting review", summary.TaskCounts.AwaitingReview)
	}
	if summary.ActiveRuns > 0 {
		return fmt.Sprintf("%d active run(s)", summary.ActiveRuns)
	}
	if summary.TaskCounts.Ready > 0 {
		return fmt.Sprintf("%d ready task(s)", summary.TaskCounts.Ready)
	}
	return ""
}
