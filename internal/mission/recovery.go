package mission

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// RecoveryReport summarizes what was recovered during a mission restart.
type RecoveryReport struct {
	MissionID      string
	StaleRecovered int
	StuckReset     int
	NewlyReady     int
}

// ReplanRequest represents a pending request to revise the task graph.
type ReplanRequest struct {
	ID              string
	MissionID       string
	AffectedTaskIDs []string
	Reason          string
	CreatedAt       time.Time
}

// MissionRecoveryManager handles restart recovery, replanning, and
// blocked-task resolution for a mission.
type MissionRecoveryManager struct {
	store     Store
	worktrees *WorktreeManager
	workers   *WorkerLauncher
}

// NewMissionRecoveryManager creates a recovery manager.
func NewMissionRecoveryManager(store Store, worktrees *WorktreeManager, workers *WorkerLauncher) *MissionRecoveryManager {
	return &MissionRecoveryManager{
		store:     store,
		worktrees: worktrees,
		workers:   workers,
	}
}

// RecoverMission is the main recovery entry point after a restart or crash.
// It recovers stale workers, resets stuck tasks, and resolves newly ready tasks.
func (rm *MissionRecoveryManager) RecoverMission(ctx context.Context, missionID string) (*RecoveryReport, error) {
	m, err := rm.store.GetMission(ctx, missionID)
	if err != nil {
		return nil, fmt.Errorf("recover: get mission: %w", err)
	}
	if m.Status.IsTerminal() {
		return nil, fmt.Errorf("recover: mission %s is in terminal state %s", missionID, m.Status)
	}

	report := &RecoveryReport{MissionID: missionID}

	// 1. Recover stale workers (delegates to WorkerLauncher.RecoverStaleWorkers).
	staleRecovered, err := rm.workers.RecoverStaleWorkers(ctx, missionID)
	if err != nil {
		return nil, fmt.Errorf("recover: stale workers: %w", err)
	}
	report.StaleRecovered = staleRecovered

	// 2. Check for stuck tasks — running tasks with no active run.
	stuck, err := rm.resetStuckTasks(ctx, missionID)
	if err != nil {
		return nil, fmt.Errorf("recover: stuck tasks: %w", err)
	}
	report.StuckReset = stuck

	// 3. Resolve newly ready tasks (tasks whose deps completed while offline).
	newlyReady, err := rm.resolveNewlyReady(ctx, missionID)
	if err != nil {
		return nil, fmt.Errorf("recover: resolve ready: %w", err)
	}
	report.NewlyReady = newlyReady

	rm.logEvent(ctx, missionID, "", "", "recovery.completed", map[string]string{
		"stale_recovered": fmt.Sprintf("%d", report.StaleRecovered),
		"stuck_reset":     fmt.Sprintf("%d", report.StuckReset),
		"newly_ready":     fmt.Sprintf("%d", report.NewlyReady),
	})

	return report, nil
}

// resetStuckTasks finds tasks in TaskRunning state that have no active
// (running) run and resets them to TaskReady.
func (rm *MissionRecoveryManager) resetStuckTasks(ctx context.Context, missionID string) (int, error) {
	tasks, err := rm.store.ListTasks(ctx, missionID)
	if err != nil {
		return 0, err
	}

	runs, err := rm.store.ListRuns(ctx, missionID)
	if err != nil {
		return 0, err
	}

	// Build set of task IDs that have an active (running) run.
	activeTaskIDs := make(map[string]bool)
	for _, r := range runs {
		if r.Status == RunRunning && r.TaskID != "" {
			activeTaskIDs[r.TaskID] = true
		}
	}

	now := time.Now().UTC()
	stuck := 0
	for _, t := range tasks {
		if t.Status != TaskRunning {
			continue
		}
		if activeTaskIDs[t.ID] {
			continue // has an active run, not stuck
		}

		// Stuck: running state but no active run.
		t.Status = TaskReady
		t.UpdatedAt = now
		if err := rm.store.UpdateTask(ctx, t); err != nil {
			continue
		}

		rm.logEvent(ctx, missionID, t.ID, "", "recovery.stuck_task_reset", nil)
		stuck++
	}

	return stuck, nil
}

// resolveNewlyReady transitions pending tasks whose dependencies are all
// done/integrated/accepted to ready.
func (rm *MissionRecoveryManager) resolveNewlyReady(ctx context.Context, missionID string) (int, error) {
	tasks, err := rm.store.ListTasks(ctx, missionID)
	if err != nil {
		return 0, err
	}

	deps, err := rm.store.ListDependencies(ctx, missionID)
	if err != nil {
		return 0, err
	}

	// Build set of task IDs that are done/integrated/accepted.
	doneSet := make(map[string]bool)
	for _, t := range tasks {
		if t.Status == TaskDone || t.Status == TaskIntegrated || t.Status == TaskAccepted {
			doneSet[t.ID] = true
		}
	}

	// Build dependency map: taskID -> count of unsatisfied deps.
	unsatisfied := make(map[string]int)
	for _, d := range deps {
		if !doneSet[d.DependsOnID] {
			unsatisfied[d.TaskID]++
		}
	}

	now := time.Now().UTC()
	promoted := 0
	for _, t := range tasks {
		if t.Status != TaskPending {
			continue
		}
		if unsatisfied[t.ID] == 0 {
			t.Status = TaskReady
			t.UpdatedAt = now
			if err := rm.store.UpdateTask(ctx, t); err != nil {
				continue
			}
			promoted++
		}
	}

	return promoted, nil
}

// RequestReplan creates a replan request after task failures or when follow-up
// work is needed. It checks the mission's MaxReplans budget before proceeding.
func (rm *MissionRecoveryManager) RequestReplan(ctx context.Context, missionID string, affectedTaskIDs []string, reason string) (*ReplanRequest, error) {
	m, err := rm.store.GetMission(ctx, missionID)
	if err != nil {
		return nil, fmt.Errorf("replan: get mission: %w", err)
	}

	// Count existing replans by counting replan.applied events.
	replanCount, err := rm.countReplans(ctx, missionID)
	if err != nil {
		return nil, fmt.Errorf("replan: count replans: %w", err)
	}

	if m.Budget.MaxReplans > 0 && replanCount >= m.Budget.MaxReplans {
		return nil, fmt.Errorf("replan budget exceeded: %d of %d replans used", replanCount, m.Budget.MaxReplans)
	}

	req := &ReplanRequest{
		ID:              generateID("rp"),
		MissionID:       missionID,
		AffectedTaskIDs: affectedTaskIDs,
		Reason:          reason,
		CreatedAt:       time.Now().UTC(),
	}

	rm.logEvent(ctx, missionID, "", "", "replan.requested", map[string]string{
		"reason":        reason,
		"affected_tasks": strings.Join(affectedTaskIDs, ","),
	})

	return req, nil
}

// countReplans counts how many replan.applied events exist for this mission.
func (rm *MissionRecoveryManager) countReplans(ctx context.Context, missionID string) (int, error) {
	events, err := rm.store.ListEvents(ctx, missionID, 0)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, e := range events {
		if e.Type == "replan.applied" {
			count++
		}
	}
	return count, nil
}

// BuildReplanPrompt constructs a prompt for the planner to revise the task
// graph based on failed tasks and current mission state.
func (rm *MissionRecoveryManager) BuildReplanPrompt(ctx context.Context, missionID string, failedTasks []*Task, reason string) (string, error) {
	m, err := rm.store.GetMission(ctx, missionID)
	if err != nil {
		return "", fmt.Errorf("replan prompt: get mission: %w", err)
	}

	tasks, err := rm.store.ListTasks(ctx, missionID)
	if err != nil {
		return "", fmt.Errorf("replan prompt: list tasks: %w", err)
	}

	deps, err := rm.store.ListDependencies(ctx, missionID)
	if err != nil {
		return "", fmt.Errorf("replan prompt: list deps: %w", err)
	}

	var b strings.Builder

	b.WriteString("# Partial Replan Request\n\n")
	b.WriteString("The mission has encountered failures requiring a partial revision of the task graph.\n")
	b.WriteString("You must produce a **partial plan update** — only add, modify, or remove tasks in\n")
	b.WriteString("the affected branch of the DAG. Do NOT re-plan tasks that are already completed.\n\n")

	// Mission goal.
	b.WriteString("## Mission Goal\n\n")
	b.WriteString(m.Goal)
	b.WriteString("\n\n")

	// Current task graph state.
	b.WriteString("## Current Task Graph\n\n")
	for _, t := range tasks {
		b.WriteString(fmt.Sprintf("- **%s** (%s): %s [%s]\n", t.ID, t.Kind, t.Title, t.Status))
	}
	b.WriteString("\n")

	// Dependencies.
	if len(deps) > 0 {
		b.WriteString("## Dependencies\n\n")
		for _, d := range deps {
			b.WriteString(fmt.Sprintf("- %s depends on %s\n", d.TaskID, d.DependsOnID))
		}
		b.WriteString("\n")
	}

	// Failed tasks detail.
	b.WriteString("## Failed Tasks\n\n")
	for _, ft := range failedTasks {
		b.WriteString(fmt.Sprintf("### %s: %s\n\n", ft.ID, ft.Title))
		b.WriteString(fmt.Sprintf("- **Status:** %s\n", ft.Status))
		b.WriteString(fmt.Sprintf("- **Attempts:** %d\n", ft.AttemptCount))
		if ft.BlockingReason != "" {
			b.WriteString(fmt.Sprintf("- **Blocking Reason:** %s\n", ft.BlockingReason))
		}
		b.WriteString(fmt.Sprintf("- **Objective:** %s\n\n", ft.Objective))
	}

	// Reason for replan.
	b.WriteString("## Reason for Replan\n\n")
	b.WriteString(reason)
	b.WriteString("\n\n")

	// Instructions.
	b.WriteString("## Instructions\n\n")
	b.WriteString("1. Analyze why the failed tasks could not be completed.\n")
	b.WriteString("2. Produce replacement or additional tasks to achieve the same objectives.\n")
	b.WriteString("3. Only modify the affected branch of the DAG — leave completed tasks untouched.\n")
	b.WriteString("4. New tasks should have unique IDs that do not collide with existing task IDs.\n")
	b.WriteString("5. Specify any new dependencies.\n")
	b.WriteString("6. Output the partial plan in the same JSON format as the original plan.\n")

	return b.String(), nil
}

// ApplyReplan applies a partial replan to an existing mission. It adds new
// tasks, wires up dependencies, resolves ready tasks, and increments the
// mission's replan counter.
func (rm *MissionRecoveryManager) ApplyReplan(ctx context.Context, missionID string, plan *PlanResult) error {
	m, err := rm.store.GetMission(ctx, missionID)
	if err != nil {
		return fmt.Errorf("apply replan: get mission: %w", err)
	}

	now := time.Now().UTC()

	// Add new tasks from the plan.
	for _, pt := range plan.Tasks {
		t := &Task{
			ID:                 pt.ID,
			MissionID:          missionID,
			Title:              pt.Title,
			Kind:               pt.Kind,
			Objective:          pt.Objective,
			Status:             TaskPending,
			Priority:           pt.Priority,
			Scope:              pt.Scope,
			AcceptanceCriteria: pt.AcceptanceCriteria,
			EstimatedEffort:    pt.EstimatedEffort,
			RiskLevel:          pt.RiskLevel,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := rm.store.CreateTask(ctx, t); err != nil {
			return fmt.Errorf("create replan task %s: %w", t.ID, err)
		}
	}

	// Add new dependencies.
	for _, dep := range plan.Dependencies {
		if err := rm.store.AddDependency(ctx, dep); err != nil {
			return fmt.Errorf("add replan dependency %s->%s: %w", dep.TaskID, dep.DependsOnID, err)
		}
	}

	// Resolve newly ready tasks.
	if err := rm.resolveReadyTasks(ctx, missionID); err != nil {
		return fmt.Errorf("resolve ready tasks after replan: %w", err)
	}

	// Update mission's replan timestamp.
	m.LastReplanAt = &now
	m.UpdatedAt = now
	if err := rm.store.UpdateMission(ctx, m); err != nil {
		return fmt.Errorf("update mission after replan: %w", err)
	}

	rm.logEvent(ctx, missionID, "", "", "replan.applied", map[string]string{
		"new_tasks": fmt.Sprintf("%d", len(plan.Tasks)),
		"new_deps":  fmt.Sprintf("%d", len(plan.Dependencies)),
	})

	return nil
}

// resolveReadyTasks transitions pending tasks whose dependencies are all
// done/integrated/accepted to ready.
func (rm *MissionRecoveryManager) resolveReadyTasks(ctx context.Context, missionID string) error {
	tasks, err := rm.store.ListTasks(ctx, missionID)
	if err != nil {
		return err
	}

	deps, err := rm.store.ListDependencies(ctx, missionID)
	if err != nil {
		return err
	}

	doneSet := make(map[string]bool)
	for _, t := range tasks {
		if t.Status == TaskDone || t.Status == TaskIntegrated || t.Status == TaskAccepted {
			doneSet[t.ID] = true
		}
	}

	unsatisfied := make(map[string]int)
	for _, d := range deps {
		if !doneSet[d.DependsOnID] {
			unsatisfied[d.TaskID]++
		}
	}

	now := time.Now().UTC()
	for _, t := range tasks {
		if t.Status != TaskPending {
			continue
		}
		if unsatisfied[t.ID] == 0 {
			t.Status = TaskReady
			t.UpdatedAt = now
			rm.store.UpdateTask(ctx, t) //nolint:errcheck
		}
	}

	return nil
}

// ResolveBlockedTask allows an operator to unblock a blocked task, clearing
// its blocking reason and transitioning it to ready.
func (rm *MissionRecoveryManager) ResolveBlockedTask(ctx context.Context, missionID, taskID string, resolution string) error {
	task, err := rm.store.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("resolve blocked: get task %s: %w", taskID, err)
	}
	if task == nil {
		return fmt.Errorf("resolve blocked: task %s not found", taskID)
	}

	if task.Status != TaskBlocked {
		return fmt.Errorf("resolve blocked: task %s is %s, not blocked", taskID, task.Status)
	}

	now := time.Now().UTC()
	task.Status = TaskReady
	task.BlockingReason = ""
	task.UpdatedAt = now
	if err := rm.store.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("resolve blocked: update task %s: %w", taskID, err)
	}

	rm.logEvent(ctx, missionID, taskID, "", "task.unblocked", map[string]string{
		"resolution": resolution,
	})

	return nil
}

// logEvent appends a structured event to the mission event log.
func (rm *MissionRecoveryManager) logEvent(ctx context.Context, missionID, taskID, runID, eventType string, payload map[string]string) {
	var payloadJSON json.RawMessage
	if payload != nil {
		payloadJSON, _ = json.Marshal(payload)
	}
	rm.store.AppendEvent(ctx, &Event{ //nolint:errcheck
		MissionID:   missionID,
		TaskID:      taskID,
		RunID:       runID,
		Type:        eventType,
		PayloadJSON: payloadJSON,
		CreatedAt:   time.Now().UTC(),
	})
}
