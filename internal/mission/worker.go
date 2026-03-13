package mission

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// WorkerLauncher coordinates the lifecycle of worker runs for a mission.
// It ties together the Scheduler (task selection), WorktreeManager (isolation),
// and Store (durable state) into a coherent dispatch/complete/fail flow.
//
// Thread safety: The launcher itself is not safe for concurrent use.
// The TUI should call it from the main update loop.
type WorkerLauncher struct {
	scheduler *Scheduler
	worktrees *WorktreeManager
	store     Store
}

// NewWorkerLauncher creates a launcher wired to the given components.
func NewWorkerLauncher(scheduler *Scheduler, worktrees *WorktreeManager, store Store) *WorkerLauncher {
	return &WorkerLauncher{
		scheduler: scheduler,
		worktrees: worktrees,
		store:     store,
	}
}

// WorkerSpec describes a provisioned worker ready for execution by the TUI.
// The TUI layer receives this and spawns an actual agent run with the Prompt.
type WorkerSpec struct {
	Run          *Run
	Task         *Task
	WorktreePath string
	Prompt       string
}

// DispatchReadyTasks selects ready tasks, provisions worktrees, creates Run
// records, and returns WorkerSpecs for the TUI to execute. Each returned spec
// represents a worker that has been leased and is ready to start.
//
// On any provisioning failure, previously provisioned specs in this batch are
// still returned (partial success). The failed task is left in ready state.
func (wl *WorkerLauncher) DispatchReadyTasks(ctx context.Context, missionID string) ([]*WorkerSpec, error) {
	mission, err := wl.store.GetMission(ctx, missionID)
	if err != nil {
		return nil, fmt.Errorf("dispatch: get mission: %w", err)
	}
	if mission.Status != MissionRunning {
		return nil, fmt.Errorf("dispatch: mission %s is %s, not running", missionID, mission.Status)
	}

	tasks, err := wl.scheduler.SelectTasks(ctx, missionID)
	if err != nil {
		return nil, fmt.Errorf("dispatch: select tasks: %w", err)
	}
	if len(tasks) == 0 {
		return nil, nil
	}

	var specs []*WorkerSpec
	for _, task := range tasks {
		spec, err := wl.provisionWorker(ctx, mission, task)
		if err != nil {
			// Log the error as an event but continue with other tasks.
			wl.logEvent(ctx, missionID, task.ID, "", "worker.provision_failed", map[string]string{
				"error": err.Error(),
			})
			continue
		}
		specs = append(specs, spec)
	}

	return specs, nil
}

// provisionWorker creates a worktree, a Run record, transitions the task to
// running, and builds the worker prompt.
func (wl *WorkerLauncher) provisionWorker(ctx context.Context, mission *Mission, task *Task) (*WorkerSpec, error) {
	baseBranch := mission.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	// Create isolated worktree.
	wtPath, err := wl.worktrees.Create(ctx, task.ID, baseBranch)
	if err != nil {
		return nil, fmt.Errorf("create worktree for %s: %w", task.ID, err)
	}

	// Create run record.
	now := time.Now().UTC()
	leaseExpiry := now.Add(30 * time.Minute)
	run := &Run{
		ID:           generateID("r"),
		MissionID:    mission.ID,
		TaskID:       task.ID,
		Mode:         RunModeWorker,
		Status:       RunRunning,
		LeaseOwner:   task.ID,
		LeaseExpires: &leaseExpiry,
		HeartbeatAt:  &now,
		WorktreePath: wtPath,
		StartedAt:    &now,
	}

	if err := wl.store.CreateRun(ctx, run); err != nil {
		// Clean up worktree on failure.
		wl.worktrees.Release(ctx, task.ID) //nolint:errcheck
		return nil, fmt.Errorf("create run for %s: %w", task.ID, err)
	}

	// Transition task to running.
	task.Status = TaskRunning
	task.AttemptCount++
	task.UpdatedAt = now
	if err := wl.store.UpdateTask(ctx, task); err != nil {
		// Run record exists but task update failed — mark run as failed.
		run.Status = RunFailed
		run.ErrorText = "failed to update task status"
		endedAt := time.Now().UTC()
		run.EndedAt = &endedAt
		wl.store.UpdateRun(ctx, run) //nolint:errcheck
		wl.worktrees.Release(ctx, task.ID) //nolint:errcheck
		return nil, fmt.Errorf("update task %s to running: %w", task.ID, err)
	}

	// Log dispatch event.
	wl.logEvent(ctx, mission.ID, task.ID, run.ID, "worker.dispatched", map[string]string{
		"worktree": wtPath,
		"attempt":  fmt.Sprintf("%d", task.AttemptCount),
	})

	prompt := BuildWorkerPrompt(task, wtPath)

	return &WorkerSpec{
		Run:          run,
		Task:         task,
		WorktreePath: wtPath,
		Prompt:       prompt,
	}, nil
}

// CompleteWorker handles successful worker completion. It transitions the run
// to succeeded, the task to awaiting_review, and releases the worktree.
func (wl *WorkerLauncher) CompleteWorker(ctx context.Context, spec *WorkerSpec, summary string) error {
	now := time.Now().UTC()

	// Update run.
	spec.Run.Status = RunSucceeded
	spec.Run.EndedAt = &now
	spec.Run.Summary = summary
	if err := wl.store.UpdateRun(ctx, spec.Run); err != nil {
		return fmt.Errorf("complete run %s: %w", spec.Run.ID, err)
	}

	// Transition task to awaiting_review.
	spec.Task.Status = TaskAwaitingReview
	spec.Task.UpdatedAt = now
	if err := wl.store.UpdateTask(ctx, spec.Task); err != nil {
		return fmt.Errorf("complete task %s: %w", spec.Task.ID, err)
	}

	wl.logEvent(ctx, spec.Run.MissionID, spec.Task.ID, spec.Run.ID, "worker.completed", map[string]string{
		"summary": summary,
	})

	// NOTE: Worktree is NOT released here. The reviewer and integrator
	// need access to the worker's branch. Release happens after
	// integration or rejection via ReleaseWorkerWorktree.

	return nil
}

// FailWorker handles worker failure. It transitions the run to failed and
// the task back to ready (for retry) or to failed (if max attempts exceeded).
func (wl *WorkerLauncher) FailWorker(ctx context.Context, spec *WorkerSpec, errText string, maxAttempts int) error {
	now := time.Now().UTC()

	// Update run.
	spec.Run.Status = RunFailed
	spec.Run.EndedAt = &now
	spec.Run.ErrorText = errText
	if err := wl.store.UpdateRun(ctx, spec.Run); err != nil {
		return fmt.Errorf("fail run %s: %w", spec.Run.ID, err)
	}

	// Decide whether to retry or mark as permanently failed.
	if maxAttempts > 0 && spec.Task.AttemptCount >= maxAttempts {
		spec.Task.Status = TaskFailed
		spec.Task.BlockingReason = fmt.Sprintf("exceeded max attempts (%d): %s", maxAttempts, errText)
	} else {
		spec.Task.Status = TaskReady // back in the queue for retry
	}
	spec.Task.UpdatedAt = now
	if err := wl.store.UpdateTask(ctx, spec.Task); err != nil {
		return fmt.Errorf("update task %s after failure: %w", spec.Task.ID, err)
	}

	wl.logEvent(ctx, spec.Run.MissionID, spec.Task.ID, spec.Run.ID, "worker.failed", map[string]string{
		"error":    errText,
		"attempts": fmt.Sprintf("%d", spec.Task.AttemptCount),
		"retrying": fmt.Sprintf("%t", spec.Task.Status == TaskReady),
	})

	// Release worktree.
	if err := wl.worktrees.Release(ctx, spec.Task.ID); err != nil {
		wl.logEvent(ctx, spec.Run.MissionID, spec.Task.ID, spec.Run.ID, "worktree.release_failed", map[string]string{
			"error": err.Error(),
		})
	}

	return nil
}

// CancelWorker handles worker cancellation (e.g., mission paused/cancelled).
func (wl *WorkerLauncher) CancelWorker(ctx context.Context, spec *WorkerSpec) error {
	now := time.Now().UTC()

	spec.Run.Status = RunCancelled
	spec.Run.EndedAt = &now
	if err := wl.store.UpdateRun(ctx, spec.Run); err != nil {
		return fmt.Errorf("cancel run %s: %w", spec.Run.ID, err)
	}

	// Put task back to ready so it can be retried.
	spec.Task.Status = TaskReady
	spec.Task.UpdatedAt = now
	if err := wl.store.UpdateTask(ctx, spec.Task); err != nil {
		return fmt.Errorf("cancel task %s: %w", spec.Task.ID, err)
	}

	wl.logEvent(ctx, spec.Run.MissionID, spec.Task.ID, spec.Run.ID, "worker.cancelled", nil)

	if err := wl.worktrees.Release(ctx, spec.Task.ID); err != nil {
		wl.logEvent(ctx, spec.Run.MissionID, spec.Task.ID, spec.Run.ID, "worktree.release_failed", map[string]string{
			"error": err.Error(),
		})
	}

	return nil
}

// ReleaseWorkerWorktree releases the worktree for a task after review +
// integration or rejection. This is separate from CompleteWorker because
// the reviewer and integrator need the worktree to remain available.
func (wl *WorkerLauncher) ReleaseWorkerWorktree(ctx context.Context, missionID, taskID string) {
	if err := wl.worktrees.Release(ctx, taskID); err != nil {
		wl.logEvent(ctx, missionID, taskID, "", "worktree.release_failed", map[string]string{
			"error": err.Error(),
		})
	}
}

// HeartbeatWorker updates the lease heartbeat for a running worker.
func (wl *WorkerLauncher) HeartbeatWorker(ctx context.Context, spec *WorkerSpec) error {
	now := time.Now().UTC()
	spec.Run.HeartbeatAt = &now
	leaseExpiry := now.Add(30 * time.Minute)
	spec.Run.LeaseExpires = &leaseExpiry
	return wl.store.UpdateRun(ctx, spec.Run)
}

// RecoverStaleWorkers finds runs whose leases have expired and marks them as
// lease_lost. Their tasks are returned to ready for retry.
func (wl *WorkerLauncher) RecoverStaleWorkers(ctx context.Context, missionID string) (int, error) {
	runs, err := wl.store.ListRuns(ctx, missionID)
	if err != nil {
		return 0, fmt.Errorf("recover: list runs: %w", err)
	}

	now := time.Now().UTC()
	recovered := 0

	for _, run := range runs {
		if run.Status != RunRunning {
			continue
		}
		if run.LeaseExpires == nil || run.LeaseExpires.After(now) {
			continue
		}

		// Lease expired — mark as lost.
		run.Status = RunLeaseLost
		run.EndedAt = &now
		run.ErrorText = "lease expired"
		if err := wl.store.UpdateRun(ctx, run); err != nil {
			continue
		}

		// Return task to ready.
		if run.TaskID != "" {
			task, err := wl.store.GetTask(ctx, run.TaskID)
			if err == nil && task.Status == TaskRunning {
				task.Status = TaskReady
				task.UpdatedAt = now
				wl.store.UpdateTask(ctx, task) //nolint:errcheck
			}
		}

		wl.logEvent(ctx, missionID, run.TaskID, run.ID, "worker.lease_lost", map[string]string{
			"lease_expired_at": run.LeaseExpires.Format(time.RFC3339),
		})

		// Release worktree if we can.
		if run.TaskID != "" {
			wl.worktrees.Release(ctx, run.TaskID) //nolint:errcheck
		}

		recovered++
	}

	return recovered, nil
}

// logEvent appends a structured event to the mission event log.
func (wl *WorkerLauncher) logEvent(ctx context.Context, missionID, taskID, runID, eventType string, payload map[string]string) {
	var payloadJSON json.RawMessage
	if payload != nil {
		payloadJSON, _ = json.Marshal(payload)
	}
	wl.store.AppendEvent(ctx, &Event{ //nolint:errcheck
		MissionID:   missionID,
		TaskID:      taskID,
		RunID:       runID,
		Type:        eventType,
		PayloadJSON: payloadJSON,
		CreatedAt:   time.Now().UTC(),
	})
}

// BuildWorkerPrompt constructs the prompt given to a worker agent for a task.
func BuildWorkerPrompt(task *Task, worktreePath string) string {
	var b strings.Builder

	b.WriteString("# Worker Task\n\n")
	b.WriteString(fmt.Sprintf("**Task ID:** %s\n", task.ID))
	b.WriteString(fmt.Sprintf("**Title:** %s\n", task.Title))
	b.WriteString(fmt.Sprintf("**Kind:** %s\n", task.Kind))
	b.WriteString(fmt.Sprintf("**Working Directory:** %s\n\n", worktreePath))

	b.WriteString("## Objective\n\n")
	b.WriteString(task.Objective)
	b.WriteString("\n\n")

	if len(task.Scope.WritePaths) > 0 {
		b.WriteString("## Writable Scope\n\n")
		b.WriteString("You may only modify files under these paths:\n")
		for _, p := range task.Scope.WritePaths {
			b.WriteString(fmt.Sprintf("- `%s`\n", p))
		}
		b.WriteString("\n")
	}

	if len(task.Scope.ReadPaths) > 0 {
		b.WriteString("## Read Scope\n\n")
		b.WriteString("These paths provide useful context:\n")
		for _, p := range task.Scope.ReadPaths {
			b.WriteString(fmt.Sprintf("- `%s`\n", p))
		}
		b.WriteString("\n")
	}

	if len(task.AcceptanceCriteria) > 0 {
		b.WriteString("## Acceptance Criteria\n\n")
		for _, c := range task.AcceptanceCriteria {
			b.WriteString(fmt.Sprintf("- %s\n", c))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Rules\n\n")
	b.WriteString("1. Work ONLY within your worktree — do not modify the main branch directly.\n")
	b.WriteString("2. Commit your changes with clear, descriptive commit messages.\n")
	b.WriteString("3. Stay within the writable scope defined above.\n")
	b.WriteString("4. Produce a brief summary of what you changed and why when done.\n")

	return b.String()
}
