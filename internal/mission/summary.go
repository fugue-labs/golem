package mission

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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
	events, err := store.ListEvents(ctx, missionID, 0)
	if err != nil {
		return nil, err
	}

	depMap := make(map[string][]string, len(tasks))
	taskByID := make(map[string]*Task, len(tasks))
	for _, dep := range deps {
		depMap[dep.TaskID] = append(depMap[dep.TaskID], dep.DependsOnID)
	}
	for _, task := range tasks {
		taskByID[task.ID] = task
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

	summary := &MissionSummary{
		Mission:         m,
		TaskCounts:      counts,
		ActiveRuns:      activeRuns,
		DependencyEdges: len(deps),
		Recovery:        latestMissionRecoveryState(events),
	}

	planApproval := latestMissionPlanApproval(approvals)
	if planApproval != nil {
		summary.PlanApprovalStatus = planApproval.Status
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
	sortMissionTaskViews(summary.RunningTasks)
	sortMissionTaskViews(summary.ReviewTasks)
	sortMissionTaskViews(summary.ReadyTasks)
	sortMissionTaskViews(summary.BlockedTasks)

	for _, approval := range approvals {
		if approval.Status != ApprovalPending {
			continue
		}
		summary.PendingApprovals++
		title := approvalDisplayTitle(approval, taskByID)
		summary.PendingApprovalItems = append(summary.PendingApprovalItems, MissionTaskView{
			ID:             approval.ID,
			Title:          title,
			ApprovalKind:   approval.Kind,
			BlockingReason: approvalBlockingReason(approval, taskByID),
		})
	}
	sortMissionTaskViews(summary.PendingApprovalItems)

	focus := selectMissionFocusTask(summary, tasks, depMap)
	if focus != nil {
		summary.FocusTask = focus
	}
	next := selectMissionNextTask(tasks, depMap, focus)
	if next != nil {
		summary.NextTask = next
	}

	summary.HealthStatus, summary.RepairReason = missionHealthState(summary)
	summary.PhaseLabel = missionPhaseLabel(summary)
	summary.NextAction = missionNextAction(summary)
	summary.Attention = missionAttention(summary)

	return summary, nil
}

func latestMissionRecoveryState(events []*Event) *MissionRecoveryState {
	ordered := append([]*Event(nil), events...)
	sortEvents(ordered)
	for i := len(ordered) - 1; i >= 0; i-- {
		event := ordered[i]
		if event == nil || event.Type != "recovery.completed" {
			continue
		}
		state := &MissionRecoveryState{LastReconciledAt: event.CreatedAt}
		if len(event.PayloadJSON) > 0 {
			var payload struct {
				StaleRecovered int  `json:"stale_recovered,string"`
				StuckReset     int  `json:"stuck_reset,string"`
				NewlyReady     int  `json:"newly_ready,string"`
				OrphanedRun    bool `json:"orphaned_running,string"`
			}
			if err := json.Unmarshal(event.PayloadJSON, &payload); err == nil {
				state.StaleRecovered = payload.StaleRecovered
				state.StuckReset = payload.StuckReset
				state.NewlyReady = payload.NewlyReady
				state.OrphanedRunning = payload.OrphanedRun
			}
		}
		state.RepairNeeded = state.OrphanedRunning || state.StaleRecovered > 0 || state.StuckReset > 0
		return state
	}
	return nil
}

func missionHealthState(summary *MissionSummary) (MissionHealthStatus, string) {
	if summary == nil || summary.Mission == nil {
		return "", ""
	}
	switch summary.Mission.Status {
	case MissionPaused:
		return MissionHealthPaused, ""
	case MissionBlocked:
		return MissionHealthBlockedState, ""
	}
	if summary.Recovery != nil && summary.Recovery.RepairNeeded {
		if summary.Recovery.OrphanedRunning {
			return MissionHealthRepairNeeded, "Recovered orphaned running work; operator should verify resumed execution"
		}
		return MissionHealthRepairNeeded, "Recovered stale mission work; verify the ready queue before continuing"
	}
	if summary.Mission.Status == MissionRunning && summary.ActiveRuns == 0 && summary.TaskCounts.Running > 0 {
		return MissionHealthRepairNeeded, "Running tasks have no active runs; reconcile mission health before continuing"
	}
	return MissionHealthHealthy, ""
}

func approvalDisplayTitle(approval *Approval, taskByID map[string]*Task) string {
	if approval == nil {
		return "Approval gate"
	}
	if approval.Kind == missionPlanApprovalKind {
		return "Mission plan approval"
	}
	if approval.TaskID != "" {
		if task := taskByID[approval.TaskID]; task != nil && strings.TrimSpace(task.Title) != "" {
			return task.Title
		}
	}
	if approval.Kind != "" {
		return fmt.Sprintf("%s approval", approval.Kind)
	}
	return "Approval gate"
}

func latestMissionPlanApproval(approvals []*Approval) *Approval {
	var latest *Approval
	for _, approval := range approvals {
		if approval == nil || approval.Kind != missionPlanApprovalKind {
			continue
		}
		if latest == nil || approval.CreatedAt.After(latest.CreatedAt) || (approval.CreatedAt.Equal(latest.CreatedAt) && approval.ID > latest.ID) {
			cp := *approval
			latest = &cp
		}
	}
	return latest
}

func approvalBlockingReason(approval *Approval, taskByID map[string]*Task) string {
	if approval == nil {
		return ""
	}
	if approval.Kind == missionPlanApprovalKind {
		return "Mission plan approval is still pending"
	}
	if approval.TaskID != "" {
		if task := taskByID[approval.TaskID]; task != nil && strings.TrimSpace(task.Title) != "" {
			return fmt.Sprintf("Approval pending for %s", task.Title)
		}
	}
	if approval.Kind != "" {
		return fmt.Sprintf("%s approval pending", approval.Kind)
	}
	return "Approval gate pending"
}

func primaryNonPlanPendingApproval(summary *MissionSummary) *MissionTaskView {
	if summary == nil {
		return nil
	}
	for i := range summary.PendingApprovalItems {
		item := &summary.PendingApprovalItems[i]
		if item.ApprovalKind == missionPlanApprovalKind {
			continue
		}
		return item
	}
	return nil
}

func sortMissionTaskViews(items []MissionTaskView) {
	sort.SliceStable(items, func(i, j int) bool {
		left := strings.TrimSpace(items[i].Title)
		right := strings.TrimSpace(items[j].Title)
		if left == right {
			return items[i].ID < items[j].ID
		}
		return left < right
	})
}

func awaitingApprovalBlocker(summary *MissionSummary) string {
	if summary == nil || summary.Mission == nil || summary.Mission.Status != MissionAwaitingApproval {
		return ""
	}
	if summary.HasApprovalGate() {
		return "Mission plan approval is waiting for approval"
	}
	if summary.PlanApprovalStatus == ApprovalApproved {
		if next := primaryNonPlanPendingApproval(summary); next != nil {
			if next.BlockingReason != "" {
				return next.BlockingReason
			}
			if next.Title != "" {
				return fmt.Sprintf("Approval pending: %s", next.Title)
			}
			return fmt.Sprintf("Resolve %d remaining approval(s) before starting", summary.PendingApprovals)
		}
		if summary.PendingApprovals > 0 {
			return fmt.Sprintf("Resolve %d remaining approval(s) before starting", summary.PendingApprovals)
		}
		return ""
	}
	if summary.PlanApprovalStatus != "" {
		return fmt.Sprintf("Mission plan approval is %s", summary.PlanApprovalStatus)
	}
	return "Mission is missing its durable plan approval record"
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

func selectMissionFocusTask(summary *MissionSummary, tasks []*Task, depMap map[string][]string) *MissionTaskView {
	if summary == nil {
		return nil
	}
	if summary.HasApprovalGate() {
		if ready := summary.PrimaryReadyTask(); ready != nil {
			copy := *ready
			return &copy
		}
	}
	if blocked := summary.PrimaryBlockedTask(); blocked != nil {
		copy := *blocked
		return &copy
	}
	if running := summary.PrimaryRunningTask(); running != nil {
		copy := *running
		return &copy
	}
	if review := summary.PrimaryReviewTask(); review != nil {
		copy := *review
		return &copy
	}
	if ready := summary.PrimaryReadyTask(); ready != nil {
		copy := *ready
		return &copy
	}
	return selectMissionTaskView(tasks, depMap,
		TaskPending,
		TaskAccepted,
		TaskIntegrated,
		TaskDone,
		TaskFailed,
		TaskRejected,
	)
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

func awaitingApprovalPhaseLabel(summary *MissionSummary) string {
	if blocker := awaitingApprovalBlocker(summary); blocker != "" && summary != nil && summary.PlanApprovalStatus == ApprovalApproved && summary.PendingApprovals > 0 {
		if next := primaryNonPlanPendingApproval(summary); next != nil {
			label := strings.TrimSpace(next.Title)
			if label == "" {
				label = strings.TrimSpace(next.ApprovalKind)
			}
			if label != "" {
				return fmt.Sprintf("Awaiting approval · %s", label)
			}
		}
		return "Awaiting approval · remaining approvals"
	}
	return "Awaiting approval"
}

func missionPhaseLabel(summary *MissionSummary) string {
	if summary == nil || summary.Mission == nil {
		return ""
	}
	switch summary.Mission.Status {
	case MissionDraft:
		return "Draft"
	case MissionPlanning:
		return "Planning"
	case MissionAwaitingApproval:
		switch {
		case summary.HasApprovalGate():
			return awaitingApprovalPhaseLabel(summary)
		case summary.PlanApprovalStatus == ApprovalApproved && summary.PendingApprovals == 0:
			return "Ready to start"
		case awaitingApprovalBlocker(summary) != "":
			return awaitingApprovalPhaseLabel(summary)
		case summary.PlanApprovalStatus != "":
			return "Awaiting plan resolution"
		default:
			return awaitingApprovalPhaseLabel(summary)
		}
	case MissionRunning:
		if summary.Recovery != nil && summary.Recovery.RepairNeeded && summary.TaskCounts.Ready == 0 && summary.ActiveRuns == 0 {
			return "Running · repair needed"
		}
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
	if summary.HasApprovalGate() {
		return "Review the proposed mission plan and approve start with /mission approve"
	}
	if blocker := awaitingApprovalBlocker(summary); blocker != "" {
		if summary.Mission.Status == MissionAwaitingApproval && summary.PlanApprovalStatus == ApprovalApproved && summary.PendingApprovals > 0 {
			return blocker
		}
	}
	switch summary.Mission.Status {
	case MissionDraft:
		return "Generate a task DAG with /mission plan"
	case MissionPlanning:
		return "Wait for planning to finish, then review the DAG"
	case MissionAwaitingApproval:
		if summary.PlanApprovalStatus == ApprovalApproved {
			return "Plan approved. Start mission execution with /mission start"
		}
		if summary.PlanApprovalStatus != "" {
			return fmt.Sprintf("Mission plan approval is %s; resolve it before starting", summary.PlanApprovalStatus)
		}
		return "Mission is awaiting a durable plan approval record before it can start"
	case MissionPaused:
		return "Resume mission execution with /mission start"
	case MissionBlocked:
		if blocked := summary.PrimaryBlockedTask(); blocked != nil && blocked.BlockingReason != "" {
			return fmt.Sprintf("Unblock %s: %s", blocked.Title, blocked.BlockingReason)
		}
		return "Resolve the blocking issue or cancel the mission"
	case MissionCompleted:
		return "Mission complete"
	case MissionCancelled:
		return "Mission cancelled"
	case MissionFailed:
		return "Inspect failures before retrying or replanning"
	}
	if blocked := summary.PrimaryBlockedTask(); blocked != nil {
		if blocked.BlockingReason != "" {
			return fmt.Sprintf("Unblock %s: %s", blocked.Title, blocked.BlockingReason)
		}
		return fmt.Sprintf("Unblock %s so the DAG can keep moving", blocked.Title)
	}
	if review := summary.PrimaryReviewTask(); review != nil {
		return fmt.Sprintf("Review %s before integration can proceed", review.Title)
	}
	if running := summary.PrimaryRunningTask(); running != nil {
		return fmt.Sprintf("Monitor active work on %s", running.Title)
	}
	if summary.HasPendingApprovals() {
		if next := primaryNonPlanPendingApproval(summary); next != nil {
			if next.BlockingReason != "" {
				return next.BlockingReason
			}
			return fmt.Sprintf("Resolve pending approval: %s", next.Title)
		}
		return fmt.Sprintf("Resolve %d pending approval(s) to keep the DAG moving", summary.PendingApprovals)
	}
	if ready := summary.PrimaryReadyTask(); ready != nil {
		return fmt.Sprintf("Next ready task: %s", ready.Title)
	}
	if summary.NextTask != nil {
		return fmt.Sprintf("Queued after dependencies: %s", summary.NextTask.Title)
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
	if summary.HasApprovalGate() {
		return "Mission plan is waiting for approval"
	}
	if summary.Mission.Status == MissionAwaitingApproval {
		if blocker := awaitingApprovalBlocker(summary); blocker != "" {
			if summary.PlanApprovalStatus == ApprovalApproved && summary.PendingApprovals > 0 {
				return blocker
			}
			switch summary.PlanApprovalStatus {
			case "":
				return blocker
			case ApprovalApproved:
				return blocker
			default:
				return blocker
			}
		}
		switch summary.PlanApprovalStatus {
		case ApprovalApproved:
			return "Plan approved and ready to start"
		case "":
			return "Mission is missing its durable plan approval record"
		default:
			return fmt.Sprintf("Mission plan approval is %s", summary.PlanApprovalStatus)
		}
	}
	if summary.HasPendingApprovals() {
		return fmt.Sprintf("%d approval gate(s) pending", summary.PendingApprovals)
	}
	if summary.HasBlockers() || summary.HasBlockedTasks() {
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
