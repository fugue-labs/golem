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

	// git worktree add -b <branch> <path> <base>
	cmd := exec.CommandContext(ctx, "git", "worktree", "add",
		"-b", branchName,
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
