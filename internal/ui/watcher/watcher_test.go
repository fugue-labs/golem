package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExternalChange(t *testing.T) {
	dir := t.TempDir()
	w, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// Write a file externally.
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case events := <-w.Events():
		if len(events) == 0 {
			t.Fatal("expected at least one event")
		}
		found := false
		for _, ev := range events {
			if ev.Path == "test.txt" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected event for test.txt, got %v", events)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for file change event")
	}
}

func TestAgentFilesSuppressed(t *testing.T) {
	dir := t.TempDir()
	w, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// Mark a file as agent-modified before creating it.
	path := filepath.Join(dir, "agent.txt")
	w.MarkAgentFile(path)
	if err := os.WriteFile(path, []byte("agent wrote this"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Should NOT get an event for the agent file.
	select {
	case events := <-w.Events():
		for _, ev := range events {
			if ev.Path == "agent.txt" {
				t.Errorf("agent file should have been suppressed, got event: %+v", ev)
			}
		}
	case <-time.After(time.Second):
		// No event — correct behavior.
	}
}

func TestIgnoredPaths(t *testing.T) {
	dir := t.TempDir()
	w, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// Create a .git directory and write a file inside it.
	gitDir := filepath.Join(dir, ".git")
	os.MkdirAll(gitDir, 0o755)
	os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)

	// Also write a swap file.
	os.WriteFile(filepath.Join(dir, ".test.swp"), []byte("swap"), 0o644)

	// Should not get events for ignored paths.
	select {
	case events := <-w.Events():
		for _, ev := range events {
			if ev.Path == ".git/HEAD" || ev.Path == ".test.swp" {
				t.Errorf("ignored path should not generate event: %+v", ev)
			}
		}
	case <-time.After(time.Second):
		// No event — correct behavior.
	}
}

func TestSubdirectoryWatching(t *testing.T) {
	dir := t.TempDir()
	w, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// Create a new subdirectory and write a file inside it.
	subDir := filepath.Join(dir, "subdir")
	os.MkdirAll(subDir, 0o755)
	// Small delay for the watcher to pick up the new directory.
	time.Sleep(100 * time.Millisecond)
	os.WriteFile(filepath.Join(subDir, "file.go"), []byte("package main"), 0o644)

	select {
	case events := <-w.Events():
		if len(events) == 0 {
			t.Fatal("expected at least one event")
		}
		found := false
		for _, ev := range events {
			if ev.Path == filepath.Join("subdir", "file.go") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected event for subdir/file.go, got %v", events)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for file change event")
	}
}
