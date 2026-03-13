package mission

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Scheduler selects the next batch of tasks eligible for worker dispatch,
// respecting concurrency limits and write-scope conflicts.
type Scheduler struct {
	store Store
}

// NewScheduler creates a Scheduler backed by the given store.
func NewScheduler(store Store) *Scheduler {
	return &Scheduler{store: store}
}

// SelectTasks returns tasks that are ready for dispatch, up to the mission's
// MaxConcurrentWorkers limit, excluding tasks whose WritePaths overlap with
// any currently running worker's worktree path. Tasks are returned in
// priority order (lower Priority value = higher priority).
func (s *Scheduler) SelectTasks(ctx context.Context, missionID string) ([]*Task, error) {
	// 1. Get the mission to read budget constraints.
	m, err := s.store.GetMission(ctx, missionID)
	if err != nil {
		return nil, fmt.Errorf("scheduler: get mission: %w", err)
	}

	// 2. Find tasks whose dependencies are all satisfied.
	ready, err := s.store.GetReadyTasks(ctx, missionID)
	if err != nil {
		return nil, fmt.Errorf("scheduler: get ready tasks: %w", err)
	}
	if len(ready) == 0 {
		return nil, nil
	}

	// 3. Count currently running workers and collect their worktree paths.
	runs, err := s.store.ListRuns(ctx, missionID)
	if err != nil {
		return nil, fmt.Errorf("scheduler: list runs: %w", err)
	}

	var runningCount int
	var activeWorktrees []string
	for _, r := range runs {
		if r.Status == RunRunning {
			runningCount++
			if r.WorktreePath != "" {
				activeWorktrees = append(activeWorktrees, r.WorktreePath)
			}
		}
	}

	// 4. Determine how many more workers we can launch.
	maxWorkers := m.Budget.MaxConcurrentWorkers
	if maxWorkers <= 0 {
		maxWorkers = 1 // default: serial execution
	}
	slots := maxWorkers - runningCount
	if slots <= 0 {
		return nil, nil
	}

	// 5. Sort by priority (lower number = higher priority).
	sort.Slice(ready, func(i, j int) bool {
		return ready[i].Priority < ready[j].Priority
	})

	// 6. Filter out tasks with write-scope conflicts and fill available slots.
	var selected []*Task
	// Track write paths claimed by tasks we've already selected in this batch
	// to avoid intra-batch conflicts.
	var selectedWritePaths []string

	for _, task := range ready {
		if len(selected) >= slots {
			break
		}

		if hasWriteConflict(task.Scope.WritePaths, activeWorktrees) {
			continue
		}
		if hasWriteConflict(task.Scope.WritePaths, selectedWritePaths) {
			continue
		}

		selected = append(selected, task)
		selectedWritePaths = append(selectedWritePaths, task.Scope.WritePaths...)
	}

	return selected, nil
}

// hasWriteConflict returns true if any path in candidate overlaps with any
// path in active. Overlap means one path is a prefix of the other (i.e. they
// refer to the same directory or a parent/child relationship).
func hasWriteConflict(candidate, active []string) bool {
	for _, cp := range candidate {
		cp = normalizePath(cp)
		for _, ap := range active {
			ap = normalizePath(ap)
			if pathsOverlap(cp, ap) {
				return true
			}
		}
	}
	return false
}

// pathsOverlap returns true if a is a prefix of b or b is a prefix of a,
// using directory-boundary-aware comparison.
func pathsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	// Ensure prefix checks respect directory boundaries.
	aSlash := a + "/"
	bSlash := b + "/"
	return strings.HasPrefix(bSlash, aSlash) || strings.HasPrefix(aSlash, bSlash)
}

// normalizePath trims trailing slashes for consistent comparison.
func normalizePath(p string) string {
	return strings.TrimRight(p, "/")
}
