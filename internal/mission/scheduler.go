package mission

import (
	"fmt"
	"sort"
)

// Scheduler selects ready tasks for execution, enforcing dependency completion
// and scope overlap constraints.
type Scheduler struct {
	store *Store
}

// NewScheduler creates a new task scheduler.
func NewScheduler(store *Store) *Scheduler {
	return &Scheduler{store: store}
}

// SchedulableTask is a task that has been validated as ready to schedule,
// along with its priority score for ordering.
type SchedulableTask struct {
	Task  *Task
	Score int // higher is better
}

// SelectTasks returns tasks that can be scheduled right now, respecting:
//   - dependency completion
//   - scope overlap with currently active tasks
//   - concurrency limits from the mission budget
//
// Returns up to maxWorkers tasks, ordered by scheduling priority.
func (s *Scheduler) SelectTasks(missionID string, maxWorkers int) ([]SchedulableTask, error) {
	// Get currently active runs to determine occupied scopes.
	activeRuns, err := s.store.ActiveRuns(missionID)
	if err != nil {
		return nil, fmt.Errorf("get active runs: %w", err)
	}
	availableSlots := maxWorkers - len(activeRuns)
	if availableSlots <= 0 {
		return nil, nil
	}

	// Get active task scopes.
	activeScopes, err := s.activeTaskScopes(activeRuns)
	if err != nil {
		return nil, fmt.Errorf("get active scopes: %w", err)
	}

	// Get ready tasks.
	readyTasks, err := s.store.ReadyTasks(missionID)
	if err != nil {
		return nil, fmt.Errorf("get ready tasks: %w", err)
	}

	// Filter: dependency completion check.
	allTasks, err := s.store.ListMissionTasks(missionID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	taskStatus := make(map[string]TaskStatus, len(allTasks))
	for _, t := range allTasks {
		taskStatus[t.ID] = t.Status
	}

	var candidates []SchedulableTask
	for _, task := range readyTasks {
		if !s.dependenciesMet(task, taskStatus) {
			continue
		}
		if s.scopeConflict(task, activeScopes) {
			continue
		}
		candidates = append(candidates, SchedulableTask{
			Task:  task,
			Score: s.priorityScore(task),
		})
	}

	// Sort by priority score descending.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	// Greedily select non-overlapping tasks up to the available slot count.
	var selected []SchedulableTask
	var selectedScopes []TaskScope
	for _, c := range candidates {
		if len(selected) >= availableSlots {
			break
		}
		// Check that this candidate doesn't overlap with already-selected tasks.
		conflict := false
		for _, scope := range selectedScopes {
			if c.Task.Scope.Overlaps(scope) {
				conflict = true
				break
			}
		}
		if conflict {
			continue
		}
		selected = append(selected, c)
		selectedScopes = append(selectedScopes, c.Task.Scope)
	}

	return selected, nil
}

// PromotePendingToReady scans tasks in "pending" status and promotes them to
// "ready" if all their dependencies are satisfied (done/integrated/accepted).
func (s *Scheduler) PromotePendingToReady(missionID string) (int, error) {
	allTasks, err := s.store.ListMissionTasks(missionID)
	if err != nil {
		return 0, err
	}
	taskStatus := make(map[string]TaskStatus, len(allTasks))
	for _, t := range allTasks {
		taskStatus[t.ID] = t.Status
	}

	promoted := 0
	for _, t := range allTasks {
		if t.Status != TaskPending {
			continue
		}
		if s.dependenciesMet(t, taskStatus) {
			if err := s.store.UpdateTaskStatus(t.ID, TaskReady, ""); err != nil {
				return promoted, err
			}
			s.store.LogEvent(&Event{
				MissionID: missionID,
				TaskID:    t.ID,
				Type:      EventTaskReady,
				Payload:   []byte(`{}`),
			})
			promoted++
		}
	}
	return promoted, nil
}

// dependenciesMet returns true if all dependencies of a task are in a terminal
// successful state (done, integrated, or accepted).
func (s *Scheduler) dependenciesMet(task *Task, taskStatus map[string]TaskStatus) bool {
	for _, depID := range task.Dependencies {
		st, ok := taskStatus[depID]
		if !ok {
			return false
		}
		switch st {
		case TaskDone, TaskIntegrated, TaskAccepted:
			continue
		default:
			return false
		}
	}
	return true
}

// scopeConflict returns true if the task's writable scope overlaps with any
// currently active task scope.
func (s *Scheduler) scopeConflict(task *Task, activeScopes []TaskScope) bool {
	// Investigation-type tasks with no writable paths don't conflict.
	if task.Kind == TaskKindInvestigation && len(task.Scope.Paths) == 0 && !task.Scope.RepoWide {
		return false
	}
	for _, scope := range activeScopes {
		if task.Scope.Overlaps(scope) {
			return true
		}
	}
	return false
}

// priorityScore computes a scheduling priority score for a task.
// Higher score = schedule first.
func (s *Scheduler) priorityScore(task *Task) int {
	score := task.Priority * 100

	// Prefer narrower scope.
	if !task.Scope.RepoWide {
		score += 50
	}
	if len(task.Scope.Paths) <= 2 {
		score += 25
	}

	// Prefer lower risk.
	switch task.RiskLevel {
	case RiskLow:
		score += 30
	case RiskMedium:
		score += 15
	}

	// Penalize retries.
	score -= task.AttemptCount * 20

	return score
}

// activeTaskScopes returns the scopes of all tasks that have active runs.
func (s *Scheduler) activeTaskScopes(runs []*Run) ([]TaskScope, error) {
	var scopes []TaskScope
	seen := make(map[string]bool)
	for _, r := range runs {
		if seen[r.TaskID] {
			continue
		}
		seen[r.TaskID] = true
		task, err := s.store.GetTask(r.TaskID)
		if err != nil {
			return nil, err
		}
		scopes = append(scopes, task.Scope)
	}
	return scopes, nil
}
