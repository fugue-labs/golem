package mission

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// WorktreeManager manages isolated git worktrees for concurrent worker runs.
// Each active task gets its own worktree. Worktrees are lease-owned and
// cleaned up when the associated run completes or expires.
type WorktreeManager struct {
	mu       sync.Mutex
	repoRoot string // original repository root
	baseDir  string // directory where worktrees are created
	active   map[string]string // runID -> worktree path
}

// NewWorktreeManager creates a worktree manager for the given repository.
func NewWorktreeManager(repoRoot string) *WorktreeManager {
	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, ".golem", "worktrees")
	return &WorktreeManager{
		repoRoot: repoRoot,
		baseDir:  baseDir,
		active:   make(map[string]string),
	}
}

// Create creates a new git worktree for a run, branching from the given base ref.
func (wm *WorktreeManager) Create(missionID, runID, baseRef string) (string, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	// Construct worktree path: ~/.golem/worktrees/<mission-id>/<run-id>/
	wtDir := filepath.Join(wm.baseDir, missionID, runID)
	if err := os.MkdirAll(filepath.Dir(wtDir), 0o755); err != nil {
		return "", fmt.Errorf("create worktree parent: %w", err)
	}

	// Create a branch name for this run.
	branchName := fmt.Sprintf("mission/%s/%s", missionID, runID)

	// Create the worktree.
	cmd := exec.Command("git", "worktree", "add", "-b", branchName, wtDir, baseRef)
	cmd.Dir = wm.repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}

	wm.active[runID] = wtDir
	return wtDir, nil
}

// Remove removes a worktree for a completed/failed run and cleans up the branch.
func (wm *WorktreeManager) Remove(runID string) error {
	wm.mu.Lock()
	wtDir, ok := wm.active[runID]
	if ok {
		delete(wm.active, runID)
	}
	wm.mu.Unlock()

	if !ok {
		return nil // nothing to clean up
	}

	// Remove the worktree.
	cmd := exec.Command("git", "worktree", "remove", "--force", wtDir)
	cmd.Dir = wm.repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		// If the directory is already gone, that's fine.
		if !os.IsNotExist(err) {
			return fmt.Errorf("git worktree remove: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}

	// Prune stale worktree references.
	pruneCmd := exec.Command("git", "worktree", "prune")
	pruneCmd.Dir = wm.repoRoot
	pruneCmd.Run() // best-effort

	return nil
}

// Path returns the worktree path for a run, if it exists.
func (wm *WorktreeManager) Path(runID string) (string, bool) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	p, ok := wm.active[runID]
	return p, ok
}

// ActiveCount returns the number of active worktrees.
func (wm *WorktreeManager) ActiveCount() int {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	return len(wm.active)
}

// CleanupAll removes all worktrees for a mission. Used during mission
// cancellation or completion.
func (wm *WorktreeManager) CleanupAll(missionID string) error {
	wm.mu.Lock()
	toRemove := make(map[string]string)
	for runID, path := range wm.active {
		if strings.Contains(path, missionID) {
			toRemove[runID] = path
		}
	}
	for runID := range toRemove {
		delete(wm.active, runID)
	}
	wm.mu.Unlock()

	var firstErr error
	for _, path := range toRemove {
		cmd := exec.Command("git", "worktree", "remove", "--force", path)
		cmd.Dir = wm.repoRoot
		if out, err := cmd.CombinedOutput(); err != nil && firstErr == nil {
			if !os.IsNotExist(err) {
				firstErr = fmt.Errorf("cleanup worktree %s: %s: %w", path, strings.TrimSpace(string(out)), err)
			}
		}
	}

	// Clean up the mission directory.
	missionDir := filepath.Join(wm.baseDir, missionID)
	os.RemoveAll(missionDir) // best-effort

	// Prune stale references.
	pruneCmd := exec.Command("git", "worktree", "prune")
	pruneCmd.Dir = wm.repoRoot
	pruneCmd.Run()

	return firstErr
}

// Diff returns the git diff of changes in a worktree against its base.
func (wm *WorktreeManager) Diff(runID string) (string, error) {
	wm.mu.Lock()
	wtDir, ok := wm.active[runID]
	wm.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("no worktree for run %s", runID)
	}

	cmd := exec.Command("git", "diff", "HEAD")
	cmd.Dir = wtDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return string(out), nil
}
