package mission

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// IntegrationEngine handles deterministic merging of approved worker output
// into the mission's base branch. No agent runs — pure git operations.
type IntegrationEngine struct {
	store    Store
	repoRoot string
}

// NewIntegrationEngine creates an integration engine.
func NewIntegrationEngine(store Store, repoRoot string) *IntegrationEngine {
	return &IntegrationEngine{store: store, repoRoot: repoRoot}
}

// IntegrateTask merges one accepted task's worker branch into the mission's
// integration branch. The task must be in TaskAccepted status and all its
// dependencies must be integrated or done.
func (ie *IntegrationEngine) IntegrateTask(ctx context.Context, missionID, taskID string) (*IntegrationResult, error) {
	m, err := ie.store.GetMission(ctx, missionID)
	if err != nil {
		return nil, fmt.Errorf("integrate: get mission: %w", err)
	}

	task, err := ie.store.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("integrate: get task %s: %w", taskID, err)
	}
	if task.Status != TaskAccepted {
		return &IntegrationResult{
			TaskID:    taskID,
			ErrorText: fmt.Sprintf("task is %s, not accepted", task.Status),
		}, nil
	}

	// Verify all dependencies are satisfied.
	if err := ie.checkDependencies(ctx, missionID, taskID); err != nil {
		return &IntegrationResult{
			TaskID:    taskID,
			ErrorText: err.Error(),
		}, nil
	}

	// Determine the target branch.
	targetBranch := m.BaseBranch
	if targetBranch == "" {
		targetBranch = "main"
	}

	// Merge the worker branch.
	workerBranch := "mission/worker/" + taskID
	mergedCommit, conflicts, err := ie.gitMerge(ctx, targetBranch, workerBranch, task.Title)
	if err != nil {
		if len(conflicts) > 0 {
			// Merge conflict — prepare the worker's worktree for conflict
			// resolution and re-queue the task so a worker can fix it.
			ie.prepareConflictResolution(ctx, taskID, targetBranch)

			now := time.Now().UTC()
			task.Status = TaskReady
			task.BlockingReason = fmt.Sprintf(
				"MERGE CONFLICT: Your branch conflicts with %s. "+
					"Conflicting files: %s\n\n"+
					"The target branch has been merged into your worktree with conflict markers. "+
					"Resolve ALL conflicts (look for <<<<<<< markers), then commit and push.",
				targetBranch, strings.Join(conflicts, ", "))
			task.UpdatedAt = now
			ie.store.UpdateTask(ctx, task) //nolint:errcheck

			ie.logEvent(ctx, missionID, taskID, "", "integration.conflict.requeued", map[string]string{
				"files": strings.Join(conflicts, ","),
			})

			return &IntegrationResult{
				TaskID:        taskID,
				ConflictFiles: conflicts,
				ErrorText:     fmt.Sprintf("merge conflict in: %s", strings.Join(conflicts, ", ")),
			}, nil
		}
		return nil, fmt.Errorf("integrate task %s: %w", taskID, err)
	}

	// Transition task to integrated.
	now := time.Now().UTC()
	task.Status = TaskIntegrated
	task.UpdatedAt = now
	if err := ie.store.UpdateTask(ctx, task); err != nil {
		return nil, fmt.Errorf("update task %s to integrated: %w", taskID, err)
	}

	ie.logEvent(ctx, missionID, taskID, "", "integration.completed", map[string]string{
		"commit": mergedCommit,
	})

	return &IntegrationResult{
		TaskID:       taskID,
		MergedCommit: mergedCommit,
		Success:      true,
	}, nil
}

// IntegrateReady finds all accepted tasks with satisfied dependencies and
// integrates them in dependency order.
func (ie *IntegrationEngine) IntegrateReady(ctx context.Context, missionID string) ([]*IntegrationResult, error) {
	tasks, err := ie.store.GetTasksByStatus(ctx, missionID, TaskAccepted)
	if err != nil {
		return nil, fmt.Errorf("integrate ready: get accepted tasks: %w", err)
	}

	var results []*IntegrationResult
	for _, task := range tasks {
		result, err := ie.IntegrateTask(ctx, missionID, task.ID)
		if err != nil {
			ie.logEvent(ctx, missionID, task.ID, "", "integration.error", map[string]string{
				"error": err.Error(),
			})
			continue
		}
		results = append(results, result)
	}

	// After integration, resolve downstream tasks that may now be ready.
	ie.resolveReadyTasks(ctx, missionID)

	return results, nil
}

// CheckMissionComplete returns true if all tasks are in a terminal success state.
func (ie *IntegrationEngine) CheckMissionComplete(ctx context.Context, missionID string) (bool, error) {
	tasks, err := ie.store.ListTasks(ctx, missionID)
	if err != nil {
		return false, err
	}
	if len(tasks) == 0 {
		return false, nil
	}

	for _, t := range tasks {
		if t.Status != TaskIntegrated && t.Status != TaskDone {
			return false, nil
		}
	}
	return true, nil
}

// CompleteMission transitions a running mission to completed.
func (ie *IntegrationEngine) CompleteMission(ctx context.Context, missionID string) error {
	m, err := ie.store.GetMission(ctx, missionID)
	if err != nil {
		return err
	}
	if m.Status != MissionRunning {
		return fmt.Errorf("cannot complete mission in %s state", m.Status)
	}

	now := time.Now().UTC()
	m.Status = MissionCompleted
	m.UpdatedAt = now
	m.EndedAt = &now
	if err := ie.store.UpdateMission(ctx, m); err != nil {
		return err
	}

	ie.logEvent(ctx, missionID, "", "", "mission.completed", nil)
	return nil
}

// prepareConflictResolution merges the target branch into the worker's worktree
// branch, leaving conflict markers for the worker to resolve. After the worker
// resolves and commits, the next integration attempt will succeed because the
// worker branch will include all of the target branch's changes.
func (ie *IntegrationEngine) prepareConflictResolution(ctx context.Context, taskID, targetBranch string) {
	wtPath := filepath.Join(ie.repoRoot, ".mission-worktrees", "worker-"+taskID)

	// Merge the target branch INTO the worker branch (in the worktree).
	// This will fail with conflicts — that's intentional. The conflict
	// markers will be left in the working tree for the worker to resolve.
	merge := exec.CommandContext(ctx, "git", "merge", "--no-commit", targetBranch)
	merge.Dir = wtPath
	merge.CombinedOutput() //nolint:errcheck // expected to fail with conflicts
}

// checkDependencies verifies all dependencies of a task are integrated or done.
func (ie *IntegrationEngine) checkDependencies(ctx context.Context, missionID, taskID string) error {
	deps, err := ie.store.ListDependencies(ctx, missionID)
	if err != nil {
		return fmt.Errorf("list dependencies: %w", err)
	}

	tasks, err := ie.store.ListTasks(ctx, missionID)
	if err != nil {
		return fmt.Errorf("list tasks: %w", err)
	}

	statusMap := make(map[string]TaskStatus, len(tasks))
	for _, t := range tasks {
		statusMap[t.ID] = t.Status
	}

	for _, dep := range deps {
		if dep.TaskID != taskID {
			continue
		}
		depStatus := statusMap[dep.DependsOnID]
		if depStatus != TaskIntegrated && depStatus != TaskDone {
			return fmt.Errorf("dependency %s is %s, not integrated/done", dep.DependsOnID, depStatus)
		}
	}

	return nil
}

// gitMerge performs a git merge of srcBranch into targetBranch.
// Returns the merge commit hash, any conflict file paths, and an error.
func (ie *IntegrationEngine) gitMerge(ctx context.Context, targetBranch, srcBranch, title string) (string, []string, error) {
	// Ensure we're on the target branch.
	checkout := exec.CommandContext(ctx, "git", "checkout", targetBranch)
	checkout.Dir = ie.repoRoot
	if out, err := checkout.CombinedOutput(); err != nil {
		return "", nil, fmt.Errorf("checkout %s: %w\n%s", targetBranch, err, out)
	}

	// Attempt merge.
	msg := fmt.Sprintf("Integrate task: %s", title)
	merge := exec.CommandContext(ctx, "git", "merge", "--no-ff", "-m", msg, srcBranch)
	merge.Dir = ie.repoRoot
	out, err := merge.CombinedOutput()
	if err != nil {
		// Check for merge conflicts.
		conflicts := ie.detectConflicts(ctx)
		if len(conflicts) > 0 {
			// Abort the merge.
			abort := exec.CommandContext(ctx, "git", "merge", "--abort")
			abort.Dir = ie.repoRoot
			abort.CombinedOutput() //nolint:errcheck
			return "", conflicts, fmt.Errorf("merge conflict")
		}
		return "", nil, fmt.Errorf("merge %s into %s: %w\n%s", srcBranch, targetBranch, err, out)
	}

	// Get the merge commit hash.
	rev := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	rev.Dir = ie.repoRoot
	commitOut, err := rev.Output()
	if err != nil {
		return "", nil, fmt.Errorf("rev-parse HEAD: %w", err)
	}

	return strings.TrimSpace(string(commitOut)), nil, nil
}

// detectConflicts returns the list of conflicted files during a merge.
func (ie *IntegrationEngine) detectConflicts(ctx context.Context) []string {
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = ie.repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

// resolveReadyTasks transitions pending tasks whose dependencies are all done/integrated to ready.
func (ie *IntegrationEngine) resolveReadyTasks(ctx context.Context, missionID string) {
	tasks, err := ie.store.ListTasks(ctx, missionID)
	if err != nil {
		return
	}

	deps, err := ie.store.ListDependencies(ctx, missionID)
	if err != nil {
		return
	}

	doneSet := make(map[string]bool)
	for _, t := range tasks {
		if t.Status == TaskDone || t.Status == TaskIntegrated {
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
			ie.store.UpdateTask(ctx, t) //nolint:errcheck
		}
	}
}

// logEvent appends a structured event to the mission event log.
func (ie *IntegrationEngine) logEvent(ctx context.Context, missionID, taskID, runID, eventType string, payload map[string]string) {
	var payloadJSON json.RawMessage
	if payload != nil {
		payloadJSON, _ = json.Marshal(payload)
	}
	ie.store.AppendEvent(ctx, &Event{ //nolint:errcheck
		MissionID:   missionID,
		TaskID:      taskID,
		RunID:       runID,
		Type:        eventType,
		PayloadJSON: payloadJSON,
		CreatedAt:   time.Now().UTC(),
	})
}
