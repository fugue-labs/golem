package mission

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ReviewLauncher coordinates the lifecycle of review runs for completed worker
// tasks. It is the Phase C counterpart to WorkerLauncher.
type ReviewLauncher struct {
	store         Store
	benchmarkMode bool
}

// NewReviewLauncher creates a review launcher.
// When benchmarkMode is true, tasks are auto-accepted after maxReviewFailures
// consecutive review failures. In normal mode, tasks are blocked instead.
func NewReviewLauncher(store Store, benchmarkMode bool) *ReviewLauncher {
	return &ReviewLauncher{store: store, benchmarkMode: benchmarkMode}
}

// ReviewSpec describes a provisioned review run ready for execution by the TUI.
type ReviewSpec struct {
	Run          *Run
	Task         *Task
	WorkerRun    *Run
	WorktreePath string
	Prompt       string
	DiffText     string
}

// DispatchPendingReviews creates review runs for tasks in TaskAwaitingReview.
// The TUI spawns a reviewer agent for each returned ReviewSpec.
func (rl *ReviewLauncher) DispatchPendingReviews(ctx context.Context, missionID, repoRoot string) ([]*ReviewSpec, error) {
	mission, err := rl.store.GetMission(ctx, missionID)
	if err != nil {
		return nil, fmt.Errorf("review: get mission: %w", err)
	}
	if mission.Status != MissionRunning {
		return nil, fmt.Errorf("review: mission %s is %s, not running", missionID, mission.Status)
	}

	tasks, err := rl.store.GetTasksByStatus(ctx, missionID, TaskAwaitingReview)
	if err != nil {
		return nil, fmt.Errorf("review: get awaiting tasks: %w", err)
	}
	if len(tasks) == 0 {
		return nil, nil
	}

	baseBranch := mission.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	var specs []*ReviewSpec
	for _, task := range tasks {
		// Advisory fast-path: skip if we already see an active review.
		// The real guard is the atomic CreateRunExclusive in provisionReview.
		if rl.hasActiveReview(ctx, task.ID) {
			continue
		}

		spec, err := rl.provisionReview(ctx, mission, task, repoRoot, baseBranch)
		if err != nil {
			rl.logEvent(ctx, missionID, task.ID, "", "review.provision_failed", map[string]string{
				"error": err.Error(),
			})
			continue
		}
		if spec == nil {
			continue // active review detected atomically
		}
		specs = append(specs, spec)
	}

	return specs, nil
}

// hasActiveReview checks if there's already a running review for a task.
func (rl *ReviewLauncher) hasActiveReview(ctx context.Context, taskID string) bool {
	runs, err := rl.store.GetRunsForTask(ctx, taskID)
	if err != nil {
		return false
	}
	for _, r := range runs {
		if r.Mode == RunModeReview && r.Status == RunRunning {
			return true
		}
	}
	return false
}

// provisionReview creates a review run and builds the review prompt.
func (rl *ReviewLauncher) provisionReview(ctx context.Context, m *Mission, task *Task, repoRoot, baseBranch string) (*ReviewSpec, error) {
	// Find the most recent succeeded worker run.
	workerRun, err := rl.lastSucceededWorkerRun(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("find worker run for %s: %w", task.ID, err)
	}

	// Validate worktree path exists before proceeding.
	if _, err := os.Stat(workerRun.WorktreePath); err != nil {
		return nil, fmt.Errorf("worktree missing for task %s: %w", task.ID, err)
	}

	// Get the diff.
	diff, err := GetWorkerDiff(ctx, repoRoot, baseBranch, task.ID)
	if err != nil {
		return nil, fmt.Errorf("get diff for %s: %w", task.ID, err)
	}

	// Atomically create run record only if no active review exists.
	// This prevents the TOCTOU race between hasActiveReview and CreateRun
	// where concurrent dispatches could both pass the check and spawn
	// duplicate reviews.
	now := time.Now().UTC()
	run := &Run{
		ID:        generateID("r"),
		MissionID: m.ID,
		TaskID:    task.ID,
		Mode:      RunModeReview,
		Status:    RunRunning,
		StartedAt: &now,
	}
	created, err := rl.store.CreateRunExclusive(ctx, run)
	if err != nil {
		return nil, fmt.Errorf("create review run for %s: %w", task.ID, err)
	}
	if !created {
		return nil, nil // another review is already running
	}

	rl.logEvent(ctx, m.ID, task.ID, run.ID, "review.dispatched", nil)

	prompt := BuildReviewPrompt(task, workerRun, diff)

	return &ReviewSpec{
		Run:          run,
		Task:         task,
		WorkerRun:    workerRun,
		WorktreePath: workerRun.WorktreePath,
		Prompt:       prompt,
		DiffText:     diff,
	}, nil
}

// lastSucceededWorkerRun finds the most recent succeeded worker run for a task.
func (rl *ReviewLauncher) lastSucceededWorkerRun(ctx context.Context, taskID string) (*Run, error) {
	runs, err := rl.store.GetRunsForTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	for i := len(runs) - 1; i >= 0; i-- {
		if runs[i].Mode == RunModeWorker && runs[i].Status == RunSucceeded {
			return runs[i], nil
		}
	}
	return nil, fmt.Errorf("no succeeded worker run found for task %s", taskID)
}

// CompleteReview handles the reviewer's verdict.
func (rl *ReviewLauncher) CompleteReview(ctx context.Context, spec *ReviewSpec, result *ReviewResult) error {
	now := time.Now().UTC()

	// Mark review run as succeeded.
	spec.Run.Status = RunSucceeded
	spec.Run.EndedAt = &now
	spec.Run.Summary = result.Summary
	if err := rl.store.UpdateRun(ctx, spec.Run); err != nil {
		return fmt.Errorf("update review run %s: %w", spec.Run.ID, err)
	}

	// Store the review result as an artifact.
	resultJSON, _ := json.Marshal(result)
	rl.store.CreateArtifact(ctx, &Artifact{ //nolint:errcheck
		ID:           generateID("a"),
		MissionID:    spec.Run.MissionID,
		TaskID:       spec.Task.ID,
		RunID:        spec.Run.ID,
		Type:         "review_result",
		RelativePath: fmt.Sprintf("reviews/%s.json", spec.Task.ID),
		CreatedAt:    now,
	})

	switch result.Verdict {
	case ReviewPass:
		return rl.handlePass(ctx, spec, result, resultJSON, now)
	case ReviewReject:
		return rl.handleReject(ctx, spec, result, resultJSON, now)
	case ReviewRequestChanges:
		return rl.handleRequestChanges(ctx, spec, result, resultJSON, now)
	default:
		return fmt.Errorf("unknown review verdict: %s", result.Verdict)
	}
}

func (rl *ReviewLauncher) handlePass(ctx context.Context, spec *ReviewSpec, result *ReviewResult, resultJSON json.RawMessage, now time.Time) error {
	// Transition task to accepted.
	spec.Task.Status = TaskAccepted
	spec.Task.UpdatedAt = now
	if err := rl.store.UpdateTask(ctx, spec.Task); err != nil {
		return fmt.Errorf("accept task %s: %w", spec.Task.ID, err)
	}

	// Create approval record.
	resolvedAt := now
	rl.store.CreateApproval(ctx, &Approval{ //nolint:errcheck
		ID:           generateID("ap"),
		MissionID:    spec.Run.MissionID,
		TaskID:       spec.Task.ID,
		RunID:        spec.Run.ID,
		Kind:         "review",
		Status:       ApprovalApproved,
		ResponseJSON: resultJSON,
		CreatedAt:    now,
		ResolvedAt:   &resolvedAt,
	})

	rl.logEvent(ctx, spec.Run.MissionID, spec.Task.ID, spec.Run.ID, "review.passed", map[string]string{
		"summary": result.Summary,
	})

	return nil
}

func (rl *ReviewLauncher) handleReject(ctx context.Context, spec *ReviewSpec, result *ReviewResult, resultJSON json.RawMessage, now time.Time) error {
	// Put task back to ready with feedback so the worker can address
	// the reviewer's concerns without starting from scratch.
	spec.Task.Status = TaskReady
	spec.Task.BlockingReason = result.Summary
	if result.Suggestion != "" {
		spec.Task.BlockingReason = result.Suggestion
	}
	spec.Task.UpdatedAt = now
	if err := rl.store.UpdateTask(ctx, spec.Task); err != nil {
		return fmt.Errorf("reject task %s: %w", spec.Task.ID, err)
	}

	resolvedAt := now
	rl.store.CreateApproval(ctx, &Approval{ //nolint:errcheck
		ID:           generateID("ap"),
		MissionID:    spec.Run.MissionID,
		TaskID:       spec.Task.ID,
		RunID:        spec.Run.ID,
		Kind:         "review",
		Status:       ApprovalRejected,
		ResponseJSON: resultJSON,
		CreatedAt:    now,
		ResolvedAt:   &resolvedAt,
	})

	rl.logEvent(ctx, spec.Run.MissionID, spec.Task.ID, spec.Run.ID, "review.rejected", map[string]string{
		"summary": result.Summary,
	})

	return nil
}

func (rl *ReviewLauncher) handleRequestChanges(ctx context.Context, spec *ReviewSpec, result *ReviewResult, resultJSON json.RawMessage, now time.Time) error {
	// Set back to ready so the orchestrator auto-retries with the feedback.
	spec.Task.Status = TaskReady
	spec.Task.BlockingReason = result.Suggestion
	if spec.Task.BlockingReason == "" {
		spec.Task.BlockingReason = result.Summary
	}
	spec.Task.UpdatedAt = now
	if err := rl.store.UpdateTask(ctx, spec.Task); err != nil {
		return fmt.Errorf("request changes for task %s: %w", spec.Task.ID, err)
	}

	resolvedAt := now
	rl.store.CreateApproval(ctx, &Approval{ //nolint:errcheck
		ID:           generateID("ap"),
		MissionID:    spec.Run.MissionID,
		TaskID:       spec.Task.ID,
		RunID:        spec.Run.ID,
		Kind:         "review",
		Status:       ApprovalRejected,
		ResponseJSON: resultJSON,
		CreatedAt:    now,
		ResolvedAt:   &resolvedAt,
	})

	rl.logEvent(ctx, spec.Run.MissionID, spec.Task.ID, spec.Run.ID, "review.changes_requested", map[string]string{
		"summary":    result.Summary,
		"suggestion": result.Suggestion,
	})

	return nil
}

// maxReviewFailures is the number of consecutive review failures before
// auto-accepting a task. This prevents infinite retry loops when the
// reviewer agent has a persistent infrastructure error.
const maxReviewFailures = 3

// FailReview handles reviewer crash/timeout. Keeps task in TaskAwaitingReview
// so a new review can be attempted, unless too many consecutive failures have
// occurred — in which case the task is auto-accepted.
func (rl *ReviewLauncher) FailReview(ctx context.Context, spec *ReviewSpec, errText string) error {
	now := time.Now().UTC()
	spec.Run.Status = RunFailed
	spec.Run.EndedAt = &now
	spec.Run.ErrorText = errText
	if err := rl.store.UpdateRun(ctx, spec.Run); err != nil {
		return fmt.Errorf("fail review run %s: %w", spec.Run.ID, err)
	}

	rl.logEvent(ctx, spec.Run.MissionID, spec.Task.ID, spec.Run.ID, "review.failed", map[string]string{
		"error": errText,
	})

	// Count consecutive review failures for this task.
	failures := rl.countConsecutiveReviewFailures(ctx, spec.Task.ID)
	if failures >= maxReviewFailures {
		if rl.benchmarkMode {
			// Benchmark mode: auto-accept since the worker completed successfully
			// but the reviewer keeps failing (infra issue). Better to let the work through.
			spec.Task.Status = TaskAccepted
			spec.Task.UpdatedAt = now
			if err := rl.store.UpdateTask(ctx, spec.Task); err != nil {
				return fmt.Errorf("auto-accept task %s after review failures: %w", spec.Task.ID, err)
			}
			rl.logEvent(ctx, spec.Run.MissionID, spec.Task.ID, spec.Run.ID, "review.auto_accepted", map[string]string{
				"reason":   fmt.Sprintf("%d consecutive review failures", failures),
				"last_err": errText,
			})
		} else {
			// Normal mode: block the task so a human can investigate.
			spec.Task.Status = TaskBlocked
			spec.Task.BlockingReason = fmt.Sprintf("review infrastructure failure (%d consecutive failures, last: %s)", failures, errText)
			spec.Task.UpdatedAt = now
			if err := rl.store.UpdateTask(ctx, spec.Task); err != nil {
				return fmt.Errorf("block task %s after review failures: %w", spec.Task.ID, err)
			}
			rl.logEvent(ctx, spec.Run.MissionID, spec.Task.ID, spec.Run.ID, "review.blocked", map[string]string{
				"reason":   fmt.Sprintf("%d consecutive review failures", failures),
				"last_err": errText,
			})
		}
		return nil
	}

	// Task stays in TaskAwaitingReview for re-review.
	return nil
}

// countConsecutiveReviewFailures counts how many of the most recent review
// runs for a task have failed consecutively.
func (rl *ReviewLauncher) countConsecutiveReviewFailures(ctx context.Context, taskID string) int {
	runs, err := rl.store.GetRunsForTask(ctx, taskID)
	if err != nil {
		return 0
	}
	count := 0
	for i := len(runs) - 1; i >= 0; i-- {
		if runs[i].Mode != RunModeReview {
			continue
		}
		if runs[i].Status == RunFailed {
			count++
		} else {
			break // non-failure breaks the streak
		}
	}
	return count
}

// logEvent appends a structured event to the mission event log.
func (rl *ReviewLauncher) logEvent(ctx context.Context, missionID, taskID, runID, eventType string, payload map[string]string) {
	var payloadJSON json.RawMessage
	if payload != nil {
		payloadJSON, _ = json.Marshal(payload)
	}
	rl.store.AppendEvent(ctx, &Event{ //nolint:errcheck
		MissionID:   missionID,
		TaskID:      taskID,
		RunID:       runID,
		Type:        eventType,
		PayloadJSON: payloadJSON,
		CreatedAt:   time.Now().UTC(),
	})
}

// GetWorkerDiff returns the diff between baseBranch and the worker's branch.
func GetWorkerDiff(ctx context.Context, repoRoot, baseBranch, taskID string) (string, error) {
	branchName := "mission/worker/" + taskID
	cmd := exec.CommandContext(ctx, "git", "diff", baseBranch+"..."+branchName)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff %s...%s: %w\n%s", baseBranch, branchName, err, out)
	}
	return string(out), nil
}

// BuildReviewPrompt constructs the prompt given to a reviewer agent.
func BuildReviewPrompt(task *Task, workerRun *Run, diff string) string {
	var b strings.Builder

	b.WriteString("# Code Review\n\n")
	b.WriteString(fmt.Sprintf("**Task ID:** %s\n", task.ID))
	b.WriteString(fmt.Sprintf("**Title:** %s\n", task.Title))
	b.WriteString(fmt.Sprintf("**Kind:** %s\n\n", task.Kind))

	b.WriteString("## Task Objective\n\n")
	b.WriteString(task.Objective)
	b.WriteString("\n\n")

	if len(task.AcceptanceCriteria) > 0 {
		b.WriteString("## Acceptance Criteria\n\n")
		for _, c := range task.AcceptanceCriteria {
			b.WriteString(fmt.Sprintf("- %s\n", c))
		}
		b.WriteString("\n")
	}

	if workerRun != nil && workerRun.Summary != "" {
		b.WriteString("## Worker Summary\n\n")
		b.WriteString(workerRun.Summary)
		b.WriteString("\n\n")
	}

	b.WriteString("## Diff to Review\n\n")
	b.WriteString("```diff\n")
	if diff != "" {
		b.WriteString(diff)
	} else {
		b.WriteString("(no changes)\n")
	}
	b.WriteString("```\n\n")

	b.WriteString("## Review Instructions\n\n")
	b.WriteString("1. Treat all worker claims as untrusted. Verify by inspecting the diff.\n")
	b.WriteString("2. Check each acceptance criterion against the actual changes.\n")
	b.WriteString("3. Look for bugs, security issues, and correctness problems.\n")
	b.WriteString("4. Produce your verdict as a JSON block in this exact format:\n\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString("  \"verdict\": \"pass\" | \"reject\" | \"request_changes\",\n")
	b.WriteString("  \"summary\": \"Brief explanation of your verdict\",\n")
	b.WriteString("  \"issues\": [\n")
	b.WriteString("    {\"severity\": \"error|warning|info\", \"file\": \"path\", \"line\": 0, \"description\": \"...\"}\n")
	b.WriteString("  ],\n")
	b.WriteString("  \"suggestion\": \"What the worker should change (for request_changes only)\"\n")
	b.WriteString("}\n")
	b.WriteString("```\n")

	return b.String()
}
