package mission

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// WorktreeManager manages git worktrees for parallel task execution.
// Each task gets an isolated worktree with its own branch, enabling
// concurrent workers to modify files without interference.
//
// Thread-safe: all methods are safe for concurrent use.
type WorktreeManager struct {
	mu       sync.Mutex
	repoRoot string
	baseDir  string
	active   map[string]string // taskID → worktree path
}

// NewWorktreeManager creates a manager rooted at the given repository.
// Worktrees are created under {repoRoot}/.mission-worktrees.
func NewWorktreeManager(repoRoot string) *WorktreeManager {
	return &WorktreeManager{
		repoRoot: repoRoot,
		baseDir:  filepath.Join(repoRoot, ".mission-worktrees"),
		active:   make(map[string]string),
	}
}

// Create provisions an isolated git worktree for the given task.
// The worktree is placed at {baseDir}/worker-{taskID} on a new branch
// named mission/worker/{taskID}, branching from baseBranch.
//
// Returns an error if a worktree already exists for the task.
func (wm *WorktreeManager) Create(ctx context.Context, taskID, baseBranch string) (string, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if _, exists := wm.active[taskID]; exists {
		return "", fmt.Errorf("worktree already exists for task %s", taskID)
	}

	worktreePath := filepath.Join(wm.baseDir, "worker-"+taskID)
	branchName := "mission/worker/" + taskID

	// Ensure base directory exists.
	if err := os.MkdirAll(wm.baseDir, 0o755); err != nil {
		return "", fmt.Errorf("create worktree base dir: %w", err)
	}

	// git worktree add -B <branch> <path> <base>
	// -B (not -b) allows re-creating the branch on task retry after
	// review rejection — the branch may already exist from a prior attempt.
	cmd := exec.CommandContext(ctx, "git", "worktree", "add",
		"-B", branchName,
		worktreePath,
		baseBranch,
	)
	cmd.Dir = wm.repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add: %w\n%s", err, out)
	}

	wm.active[taskID] = worktreePath
	return worktreePath, nil
}

// Release removes the worktree associated with the given task.
// Uses --force to ensure removal even if the worktree has modifications.
func (wm *WorktreeManager) Release(ctx context.Context, taskID string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wtPath, exists := wm.active[taskID]
	if !exists {
		return fmt.Errorf("no worktree found for task %s", taskID)
	}

	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", wtPath)
	cmd.Dir = wm.repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove: %w\n%s", err, out)
	}

	delete(wm.active, taskID)
	return nil
}

// Get returns the worktree path for a task and whether it exists.
func (wm *WorktreeManager) Get(taskID string) (string, bool) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	path, ok := wm.active[taskID]
	return path, ok
}

// List returns a copy of all active taskID-to-worktree-path mappings.
func (wm *WorktreeManager) List() map[string]string {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	result := make(map[string]string, len(wm.active))
	for k, v := range wm.active {
		result[k] = v
	}
	return result
}

// RecoverFromRemote fetches a previously pushed branch from the remote and
// resets the worktree to match it. This handles the edge case where a worktree
// was lost (e.g., cleaned up) but the worker had pushed their changes.
// Returns true if recovery was successful, false if the remote branch doesn't
// exist or the recovery failed (in which case the worktree starts fresh).
func (wm *WorktreeManager) RecoverFromRemote(ctx context.Context, taskID string) bool {
	wm.mu.Lock()
	wtPath, exists := wm.active[taskID]
	wm.mu.Unlock()
	if !exists {
		return false
	}

	branchName := "mission/worker/" + taskID

	// Fetch the remote branch (may not exist if worker never pushed).
	fetch := exec.CommandContext(ctx, "git", "fetch", "origin", branchName)
	fetch.Dir = wm.repoRoot
	if _, err := fetch.CombinedOutput(); err != nil {
		return false
	}

	// Reset the local branch to match the remote.
	reset := exec.CommandContext(ctx, "git", "reset", "--hard", "origin/"+branchName)
	reset.Dir = wtPath
	if _, err := reset.CombinedOutput(); err != nil {
		return false
	}

	return true
}

// CleanupOrphans prunes worktrees that are no longer tracked by the manager.
// This handles stale worktrees left behind by crashed workers or unclean shutdowns.
func (wm *WorktreeManager) CleanupOrphans(ctx context.Context) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	cmd := exec.CommandContext(ctx, "git", "worktree", "prune")
	cmd.Dir = wm.repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree prune: %w\n%s", err, out)
	}
	return nil
}
