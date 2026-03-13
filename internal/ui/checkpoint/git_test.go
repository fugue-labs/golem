package checkpoint

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitSnapshotAndRestore(t *testing.T) {
	// Create a temporary git repo.
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	// Init repo with a file.
	run("init")
	writeFile(t, dir, "hello.txt", "original content\n")
	run("add", ".")
	run("commit", "-m", "initial")

	s := NewStore(dir)

	// Turn 1: modify the file.
	writeFile(t, dir, "hello.txt", "modified at turn 1\n")
	s.Save(Checkpoint{Turn: 1, Prompt: "turn 1"})

	// Verify snapshot was captured.
	cp1 := s.Get(1)
	if cp1.GitStash == "" {
		t.Fatal("expected non-empty git stash for dirty working tree")
	}

	// Turn 2: modify the file again.
	writeFile(t, dir, "hello.txt", "modified at turn 2\n")
	writeFile(t, dir, "new-file.txt", "new file content\n")
	s.Save(Checkpoint{Turn: 2, Prompt: "turn 2"})

	cp2 := s.Get(2)
	if cp2.GitStash == "" {
		t.Fatal("expected non-empty git stash for turn 2")
	}

	// Rewind to turn 1.
	_, err := s.RewindTo(1)
	if err != nil {
		t.Fatalf("rewind to turn 1 failed: %v", err)
	}

	// Verify file content matches turn 1 state.
	content := readFile(t, dir, "hello.txt")
	if content != "modified at turn 1\n" {
		t.Fatalf("expected turn 1 content, got %q", content)
	}

	// Verify new-file.txt was removed (it didn't exist at turn 1's git state + stash).
	if _, err := os.Stat(filepath.Join(dir, "new-file.txt")); err == nil {
		t.Fatal("expected new-file.txt to be cleaned up after rewind to turn 1")
	}

	// Verify checkpoint history was truncated.
	if s.Len() != 1 {
		t.Fatalf("expected 1 checkpoint after rewind, got %d", s.Len())
	}
}

func TestGitSnapshotCleanWorkingTree(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init")
	writeFile(t, dir, "file.txt", "committed content\n")
	run("add", ".")
	run("commit", "-m", "initial")

	// No modifications — clean working tree.
	s := NewStore(dir)
	s.Save(Checkpoint{Turn: 1, Prompt: "clean"})

	cp := s.Get(1)
	if cp.GitStash != "" {
		t.Fatalf("expected empty stash for clean working tree, got %q", cp.GitStash)
	}

	// Modify after checkpoint, then rewind.
	writeFile(t, dir, "file.txt", "dirty changes\n")
	_, err := s.RewindTo(1)
	if err != nil {
		t.Fatalf("rewind failed: %v", err)
	}

	content := readFile(t, dir, "file.txt")
	if content != "committed content\n" {
		t.Fatalf("expected original content after rewind to clean checkpoint, got %q", content)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}
