package mission

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initTestRepo creates a temporary git repository with one commit so that
// worktrees can be created from HEAD.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}

	// Create a file and commit so HEAD exists.
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial commit"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}

	return dir
}

func TestCreateAndGet(t *testing.T) {
	repo := initTestRepo(t)
	wm := NewWorktreeManager(repo)
	ctx := context.Background()

	wtPath, err := wm.Create(ctx, "task-1", "HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Worktree path should be under the base directory.
	expectedPath := filepath.Join(repo, ".mission-worktrees", "worker-task-1")
	if wtPath != expectedPath {
		t.Errorf("path = %q, want %q", wtPath, expectedPath)
	}

	// The directory should exist on disk.
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Error("worktree directory does not exist")
	}

	// Get should return the same path.
	gotPath, ok := wm.Get("task-1")
	if !ok {
		t.Fatal("Get returned ok=false for existing task")
	}
	if gotPath != wtPath {
		t.Errorf("Get path = %q, want %q", gotPath, wtPath)
	}
}

func TestRelease(t *testing.T) {
	repo := initTestRepo(t)
	wm := NewWorktreeManager(repo)
	ctx := context.Background()

	wtPath, err := wm.Create(ctx, "task-2", "HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := wm.Release(ctx, "task-2"); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// Worktree directory should be gone.
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree directory still exists after Release")
	}

	// Get should return false.
	if _, ok := wm.Get("task-2"); ok {
		t.Error("Get returned ok=true after Release")
	}
}

func TestGetUnknownTask(t *testing.T) {
	repo := initTestRepo(t)
	wm := NewWorktreeManager(repo)

	_, ok := wm.Get("nonexistent")
	if ok {
		t.Error("Get returned ok=true for unknown task")
	}
}

func TestDoubleCreateErrors(t *testing.T) {
	repo := initTestRepo(t)
	wm := NewWorktreeManager(repo)
	ctx := context.Background()

	if _, err := wm.Create(ctx, "task-dup", "HEAD"); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err := wm.Create(ctx, "task-dup", "HEAD")
	if err == nil {
		t.Fatal("second Create should have failed")
	}
}

func TestList(t *testing.T) {
	repo := initTestRepo(t)
	wm := NewWorktreeManager(repo)
	ctx := context.Background()

	// Empty initially.
	if m := wm.List(); len(m) != 0 {
		t.Fatalf("List should be empty, got %d entries", len(m))
	}

	ids := []string{"alpha", "beta", "gamma"}
	for _, id := range ids {
		if _, err := wm.Create(ctx, id, "HEAD"); err != nil {
			t.Fatalf("Create(%s): %v", id, err)
		}
	}

	listed := wm.List()
	if len(listed) != len(ids) {
		t.Fatalf("List returned %d entries, want %d", len(listed), len(ids))
	}

	for _, id := range ids {
		expected := filepath.Join(repo, ".mission-worktrees", "worker-"+id)
		if listed[id] != expected {
			t.Errorf("List[%s] = %q, want %q", id, listed[id], expected)
		}
	}

	// Verify List returns a copy — mutating it should not affect the manager.
	listed["extra"] = "/tmp/fake"
	if _, ok := wm.Get("extra"); ok {
		t.Error("mutating List result should not affect manager state")
	}
}

func TestCleanupOrphans(t *testing.T) {
	repo := initTestRepo(t)
	wm := NewWorktreeManager(repo)
	ctx := context.Background()

	// CleanupOrphans on a clean repo should succeed.
	if err := wm.CleanupOrphans(ctx); err != nil {
		t.Fatalf("CleanupOrphans: %v", err)
	}
}

func TestReleaseUnknownTaskErrors(t *testing.T) {
	repo := initTestRepo(t)
	wm := NewWorktreeManager(repo)
	ctx := context.Background()

	if err := wm.Release(ctx, "ghost"); err == nil {
		t.Error("Release of unknown task should error")
	}
}
