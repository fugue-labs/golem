package mission

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// LeaseDuration is the default lease duration for a worker run.
const LeaseDuration = 10 * time.Minute

// Controller owns the mission lifecycle: scheduling, leasing, monitoring,
// event logging, and recovery.
type Controller struct {
	mu        sync.Mutex
	store     *Store
	scheduler *Scheduler
	worktrees *WorktreeManager
	repoRoot  string

	// onChange is called (if non-nil) whenever mission state changes,
	// allowing the TUI to refresh.
	onChange func()
}

// NewController creates a mission controller for the given repository.
func NewController(dbPath, repoRoot string) (*Controller, error) {
	store, err := OpenStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open mission store: %w", err)
	}
	return &Controller{
		store:     store,
		scheduler: NewScheduler(store),
		worktrees: NewWorktreeManager(repoRoot),
		repoRoot:  repoRoot,
	}, nil
}

// SetOnChange registers a callback for state change notifications.
func (c *Controller) SetOnChange(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onChange = fn
}

func (c *Controller) notify() {
	if c.onChange != nil {
		c.onChange()
	}
}

// Close closes the controller and its resources.
func (c *Controller) Close() error {
	return c.store.Close()
}

// Store returns the underlying store for direct queries.
func (c *Controller) Store() *Store {
	return c.store
}

// --- Mission lifecycle ---

// CreateMission creates a new mission in draft state.
func (c *Controller) CreateMission(title, goal, baseBranch string) (*Mission, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Get repository state.
	baseCommit, err := gitHeadCommit(c.repoRoot)
	if err != nil {
		return nil, fmt.Errorf("get HEAD commit: %w", err)
	}

	m := &Mission{
		ID:         "m_" + shortID(),
		Title:      title,
		Goal:       goal,
		RepoRoot:   c.repoRoot,
		BaseCommit: baseCommit,
		BaseBranch: baseBranch,
		Status:     MissionDraft,
		Policy:     DefaultPolicy(),
		Budget:     DefaultBudget(),
	}

	if err := c.store.CreateMission(m); err != nil {
		return nil, err
	}

	c.store.LogEvent(&Event{
		MissionID: m.ID,
		Type:      EventMissionCreated,
		Payload:   mustJSON(map[string]string{"title": title, "goal": goal}),
	})

	c.notify()
	return m, nil
}

// StartMission transitions a mission from draft/planning to running.
func (c *Controller) StartMission(missionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	m, err := c.store.GetMission(missionID)
	if err != nil {
		return err
	}
	if m.Status != MissionDraft && m.Status != MissionPlanning && m.Status != MissionAwaitingApproval {
		return fmt.Errorf("cannot start mission in %s state", m.Status)
	}

	now := time.Now().UTC()
	m.StartedAt = &now
	if err := c.store.UpdateMissionStatus(missionID, MissionRunning); err != nil {
		return err
	}

	// Promote any dependency-met pending tasks to ready.
	c.scheduler.PromotePendingToReady(missionID)

	c.store.LogEvent(&Event{
		MissionID: missionID,
		Type:      EventMissionStarted,
		Payload:   []byte(`{}`),
	})

	c.notify()
	return nil
}

// PauseMission stops leasing new tasks and moves mission to paused.
func (c *Controller) PauseMission(missionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.store.UpdateMissionStatus(missionID, MissionPaused); err != nil {
		return err
	}
	c.store.LogEvent(&Event{
		MissionID: missionID,
		Type:      EventMissionPaused,
		Payload:   []byte(`{}`),
	})
	c.notify()
	return nil
}

// ResumeMission moves a paused mission back to running.
func (c *Controller) ResumeMission(missionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	m, err := c.store.GetMission(missionID)
	if err != nil {
		return err
	}
	if m.Status != MissionPaused {
		return fmt.Errorf("cannot resume mission in %s state", m.Status)
	}

	// Recover expired leases first.
	c.recoverExpiredLeases(missionID)

	if err := c.store.UpdateMissionStatus(missionID, MissionRunning); err != nil {
		return err
	}
	c.store.LogEvent(&Event{
		MissionID: missionID,
		Type:      EventMissionResumed,
		Payload:   []byte(`{}`),
	})
	c.notify()
	return nil
}

// CancelMission cancels a mission and cleans up all active runs.
func (c *Controller) CancelMission(missionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Cancel all active runs.
	runs, _ := c.store.ActiveRuns(missionID)
	for _, r := range runs {
		c.store.UpdateRunStatus(r.ID, RunCancelled, "", "mission cancelled")
		c.worktrees.Remove(r.ID)
	}

	if err := c.store.UpdateMissionStatus(missionID, MissionCancelled); err != nil {
		return err
	}
	c.worktrees.CleanupAll(missionID)
	c.store.LogEvent(&Event{
		MissionID: missionID,
		Type:      EventMissionCancelled,
		Payload:   []byte(`{}`),
	})
	c.notify()
	return nil
}

// --- Task management ---

// AddTask adds a task to a mission.
func (c *Controller) AddTask(task *Task) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.store.CreateTask(task); err != nil {
		return err
	}
	c.store.LogEvent(&Event{
		MissionID: task.MissionID,
		TaskID:    task.ID,
		Type:      EventTaskCreated,
		Payload:   mustJSON(map[string]string{"title": task.Title, "kind": string(task.Kind)}),
	})
	c.notify()
	return nil
}

// --- Scheduling ---

// ScheduleNext selects and leases the next batch of ready tasks.
// Returns the runs that were created and leased.
func (c *Controller) ScheduleNext(missionID string) ([]*Run, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	m, err := c.store.GetMission(missionID)
	if err != nil {
		return nil, err
	}
	if m.Status != MissionRunning {
		return nil, nil
	}

	// Promote pending tasks first.
	c.scheduler.PromotePendingToReady(missionID)

	// Check for expired leases.
	c.recoverExpiredLeases(missionID)

	// Select schedulable tasks.
	candidates, err := c.scheduler.SelectTasks(missionID, m.Budget.MaxConcurrentWorkers)
	if err != nil {
		return nil, fmt.Errorf("select tasks: %w", err)
	}

	var created []*Run
	for _, candidate := range candidates {
		run, err := c.leaseTask(m, candidate.Task)
		if err != nil {
			// Log but continue with other tasks.
			c.store.LogEvent(&Event{
				MissionID: missionID,
				TaskID:    candidate.Task.ID,
				Type:      EventRunFailed,
				Payload:   mustJSON(map[string]string{"error": err.Error()}),
			})
			continue
		}
		created = append(created, run)
	}

	// Check if mission is blocked (no active runs, no ready tasks).
	if len(created) == 0 {
		activeRuns, _ := c.store.ActiveRuns(missionID)
		readyTasks, _ := c.store.ReadyTasks(missionID)
		if len(activeRuns) == 0 && len(readyTasks) == 0 {
			// Check if all tasks are done.
			allTasks, _ := c.store.ListMissionTasks(missionID)
			allDone := true
			for _, t := range allTasks {
				if t.Status != TaskDone && t.Status != TaskSuperseded {
					allDone = false
					break
				}
			}
			if allDone && len(allTasks) > 0 {
				c.store.UpdateMissionStatus(missionID, MissionCompleted)
				c.store.LogEvent(&Event{
					MissionID: missionID,
					Type:      EventMissionCompleted,
					Payload:   []byte(`{}`),
				})
			} else if len(allTasks) > 0 {
				c.store.UpdateMissionStatus(missionID, MissionBlocked)
				c.store.LogEvent(&Event{
					MissionID: missionID,
					Type:      EventMissionBlocked,
					Payload:   []byte(`{}`),
				})
			}
		}
	}

	c.notify()
	return created, nil
}

// leaseTask creates a worktree, creates a run, and leases a task.
func (c *Controller) leaseTask(m *Mission, task *Task) (*Run, error) {
	runID := "r_" + shortID()

	// Create worktree.
	wtPath, err := c.worktrees.Create(m.ID, runID, m.BaseCommit)
	if err != nil {
		return nil, fmt.Errorf("create worktree: %w", err)
	}

	now := time.Now().UTC()
	leaseExpiry := now.Add(LeaseDuration)

	run := &Run{
		ID:             runID,
		MissionID:      m.ID,
		TaskID:         task.ID,
		Mode:           RunModeWorker,
		Status:         RunRunning,
		LeaseOwner:     "controller",
		LeaseExpiresAt: &leaseExpiry,
		HeartbeatAt:    &now,
		WorktreePath:   wtPath,
		StartedAt:      &now,
	}

	if err := c.store.CreateRun(run); err != nil {
		c.worktrees.Remove(runID)
		return nil, fmt.Errorf("create run: %w", err)
	}

	// Update task status.
	c.store.UpdateTaskStatus(task.ID, TaskRunning, "")
	c.store.IncrementTaskAttempt(task.ID)

	// Log events.
	c.store.LogEvent(&Event{
		MissionID: m.ID,
		TaskID:    task.ID,
		RunID:     runID,
		Type:      EventTaskLeased,
		Payload:   mustJSON(map[string]string{"run_id": runID}),
	})
	c.store.LogEvent(&Event{
		MissionID: m.ID,
		TaskID:    task.ID,
		RunID:     runID,
		Type:      EventRunStarted,
		Payload:   mustJSON(map[string]string{"worktree": wtPath}),
	})

	return run, nil
}

// --- Run lifecycle ---

// HeartbeatRun extends the lease on a running worker.
func (c *Controller) HeartbeatRun(runID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.store.HeartbeatRun(runID, time.Now().UTC().Add(LeaseDuration))
}

// CompleteRun marks a run as succeeded and transitions the task.
func (c *Controller) CompleteRun(runID, summary string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.store.UpdateRunStatus(runID, RunSucceeded, summary, ""); err != nil {
		return err
	}

	// Get the run to find the task.
	rows, err := c.store.db.Query(`SELECT mission_id, task_id FROM runs WHERE id = ?`, runID)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		return fmt.Errorf("run not found: %s", runID)
	}
	var missionID, taskID string
	rows.Scan(&missionID, &taskID)
	rows.Close()

	// Transition task to awaiting_review or done depending on policy.
	mission, err := c.store.GetMission(missionID)
	if err != nil {
		return err
	}

	nextStatus := TaskDone
	if mission.Policy.RequireReviewBeforeIntegration {
		nextStatus = TaskAwaitingReview
	}
	c.store.UpdateTaskStatus(taskID, nextStatus, "")

	eventType := EventRunSucceeded
	c.store.LogEvent(&Event{
		MissionID: missionID,
		TaskID:    taskID,
		RunID:     runID,
		Type:      eventType,
		Payload:   mustJSON(map[string]string{"summary": summary}),
	})

	// Clean up worktree (keep it for review if needed).
	if nextStatus == TaskDone {
		c.worktrees.Remove(runID)
	}

	c.notify()
	return nil
}

// FailRun marks a run as failed and transitions the task appropriately.
func (c *Controller) FailRun(runID, errorText string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.store.UpdateRunStatus(runID, RunFailed, "", errorText); err != nil {
		return err
	}

	rows, err := c.store.db.Query(`SELECT mission_id, task_id FROM runs WHERE id = ?`, runID)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		return fmt.Errorf("run not found: %s", runID)
	}
	var missionID, taskID string
	rows.Scan(&missionID, &taskID)
	rows.Close()

	// Re-queue the task as ready for retry.
	c.store.UpdateTaskStatus(taskID, TaskReady, "")

	c.store.LogEvent(&Event{
		MissionID: missionID,
		TaskID:    taskID,
		RunID:     runID,
		Type:      EventRunFailed,
		Payload:   mustJSON(map[string]string{"error": errorText}),
	})

	c.worktrees.Remove(runID)
	c.notify()
	return nil
}

// --- Recovery ---

// RecoverOnStartup reconciles expired leases and dead runs.
func (c *Controller) RecoverOnStartup() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	missions, err := c.store.ListActiveMissions()
	if err != nil {
		return err
	}

	for _, m := range missions {
		c.store.LogEvent(&Event{
			MissionID: m.ID,
			Type:      EventRecoveryStarted,
			Payload:   []byte(`{}`),
		})
		c.recoverExpiredLeases(m.ID)
		c.store.LogEvent(&Event{
			MissionID: m.ID,
			Type:      EventRecoveryCompleted,
			Payload:   []byte(`{}`),
		})
	}
	return nil
}

func (c *Controller) recoverExpiredLeases(missionID string) {
	expired, err := c.store.ExpiredLeaseRuns(missionID)
	if err != nil {
		return
	}
	for _, r := range expired {
		c.store.UpdateRunStatus(r.ID, RunLeaseLost, "", "lease expired")
		c.store.UpdateTaskStatus(r.TaskID, TaskReady, "")
		c.worktrees.Remove(r.ID)
		c.store.LogEvent(&Event{
			MissionID: missionID,
			TaskID:    r.TaskID,
			RunID:     r.ID,
			Type:      EventRunLeaseLost,
			Payload:   []byte(`{}`),
		})
		c.store.LogEvent(&Event{
			MissionID: missionID,
			TaskID:    r.TaskID,
			Type:      EventRecoveryReconciledRun,
			Payload:   mustJSON(map[string]string{"run_id": r.ID, "action": "requeued"}),
		})
	}
}

// --- Status queries ---

// MissionStatus returns a snapshot of mission state for TUI display.
func (c *Controller) MissionStatus(missionID string) (*MissionSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	m, err := c.store.GetMission(missionID)
	if err != nil {
		return nil, err
	}
	tasks, err := c.store.ListMissionTasks(missionID)
	if err != nil {
		return nil, err
	}
	cards, err := c.store.WorkerCards(missionID)
	if err != nil {
		return nil, err
	}
	events, err := c.store.RecentEvents(missionID, 10)
	if err != nil {
		return nil, err
	}

	snap := &MissionSnapshot{
		Mission:      m,
		Tasks:        tasks,
		WorkerCards:  cards,
		RecentEvents: events,
	}

	// Compute task counts.
	for _, t := range tasks {
		snap.TotalTasks++
		switch t.Status {
		case TaskReady:
			snap.ReadyTasks++
		case TaskRunning, TaskLeased:
			snap.RunningTasks++
		case TaskDone, TaskIntegrated, TaskAccepted:
			snap.DoneTasks++
		case TaskBlocked:
			snap.BlockedTasks++
		case TaskAwaitingReview:
			snap.ReviewTasks++
		}
	}

	return snap, nil
}

// MissionSnapshot is a point-in-time view of a mission for TUI rendering.
type MissionSnapshot struct {
	Mission      *Mission
	Tasks        []*Task
	WorkerCards  []WorkerCard
	RecentEvents []*Event
	TotalTasks   int
	ReadyTasks   int
	RunningTasks int
	DoneTasks    int
	BlockedTasks int
	ReviewTasks  int
}

// --- Helpers ---

func shortID() string {
	return uuid.New().String()[:8]
}

func gitHeadCommit(repoRoot string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// MissionStoreDir returns the default mission store path for a project.
func MissionStoreDir(workDir string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".golem", "missions", "missions.db")
}
