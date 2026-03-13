package mission

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fugue-labs/golem/internal/agent"
)

func TestIsVagueGoal(t *testing.T) {
	p := &Planner{}

	vague := []string{
		"do it", "fix it", "start", "help me",
		"short", "ab", "",
		"  do it  ", // whitespace-padded
	}
	for _, g := range vague {
		if !p.IsVagueGoal(g) {
			t.Errorf("expected %q to be vague", g)
		}
	}

	specific := []string{
		"Add pagination to the /api/users endpoint",
		"Refactor the auth middleware to support OAuth2",
		"Fix the race condition in the worker pool shutdown",
	}
	for _, g := range specific {
		if p.IsVagueGoal(g) {
			t.Errorf("expected %q to NOT be vague", g)
		}
	}
}

func TestFindKeyFiles(t *testing.T) {
	dir := t.TempDir()

	// Create some key files.
	for _, name := range []string{"go.mod", "README.md", "Makefile"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	p := &Planner{}
	found := p.FindKeyFiles(dir)

	want := map[string]bool{"go.mod": true, "README.md": true, "Makefile": true}
	for _, f := range found {
		if !want[f] {
			t.Errorf("unexpected key file: %s", f)
		}
		delete(want, f)
	}
	for missing := range want {
		t.Errorf("missing key file: %s", missing)
	}
}

func TestFindKeyFilesEmpty(t *testing.T) {
	dir := t.TempDir()
	p := &Planner{}
	found := p.FindKeyFiles(dir)
	if len(found) != 0 {
		t.Errorf("expected no key files, got %v", found)
	}
}

func TestScanDirectoryTree(t *testing.T) {
	dir := t.TempDir()

	// Create a simple tree: dir/sub/file.go, dir/top.txt
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0755)
	os.WriteFile(filepath.Join(sub, "file.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "top.txt"), []byte("x"), 0644)

	p := &Planner{}
	tree := p.ScanDirectoryTree(dir, 2)

	if !strings.Contains(tree, "sub/") {
		t.Errorf("tree missing sub/\n%s", tree)
	}
	if !strings.Contains(tree, "file.go") {
		t.Errorf("tree missing file.go\n%s", tree)
	}
	if !strings.Contains(tree, "top.txt") {
		t.Errorf("tree missing top.txt\n%s", tree)
	}
}

func TestScanDirectoryTreeSkipsHiddenAndVendor(t *testing.T) {
	dir := t.TempDir()

	for _, d := range []string{".git", "node_modules", "vendor", ".hidden"} {
		os.Mkdir(filepath.Join(dir, d), 0755)
		os.WriteFile(filepath.Join(dir, d, "inner.txt"), []byte("x"), 0644)
	}
	os.Mkdir(filepath.Join(dir, "src"), 0755)

	p := &Planner{}
	tree := p.ScanDirectoryTree(dir, 2)

	for _, skip := range []string{".git", "node_modules", "vendor", ".hidden"} {
		if strings.Contains(tree, skip) {
			t.Errorf("tree should skip %s\n%s", skip, tree)
		}
	}
	if !strings.Contains(tree, "src/") {
		t.Errorf("tree missing src/\n%s", tree)
	}
}

func TestScanDirectoryTreeRespectsMaxDepth(t *testing.T) {
	dir := t.TempDir()

	// Create dir/a/b/c/deep.txt
	deep := filepath.Join(dir, "a", "b", "c")
	os.MkdirAll(deep, 0755)
	os.WriteFile(filepath.Join(deep, "deep.txt"), []byte("x"), 0644)

	p := &Planner{}
	tree := p.ScanDirectoryTree(dir, 1)

	// depth 1 should show a/ and b/ but NOT c/ or deep.txt
	if !strings.Contains(tree, "a/") {
		t.Errorf("tree missing a/\n%s", tree)
	}
	if strings.Contains(tree, "deep.txt") {
		t.Errorf("tree should not show deep.txt at depth 1\n%s", tree)
	}
}

func TestGatherCodebaseContext(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)

	p := &Planner{
		GitInfo: &agent.GitInfo{
			IsRepo: true,
			Branch: "main",
		},
	}

	ctx := p.GatherCodebaseContext(dir)

	if !strings.Contains(ctx, "Branch: `main`") {
		t.Errorf("missing git branch in context\n%s", ctx)
	}
	if !strings.Contains(ctx, "Directory Structure") {
		t.Errorf("missing directory structure\n%s", ctx)
	}
	if !strings.Contains(ctx, "go.mod") {
		t.Errorf("missing key file go.mod\n%s", ctx)
	}
}

func TestGatherCodebaseContextNoGit(t *testing.T) {
	dir := t.TempDir()

	p := &Planner{} // nil GitInfo
	ctx := p.GatherCodebaseContext(dir)

	if strings.Contains(ctx, "Git Context") {
		t.Errorf("should not include git context when GitInfo is nil\n%s", ctx)
	}
	if !strings.Contains(ctx, "Directory Structure") {
		t.Errorf("should still include directory structure\n%s", ctx)
	}
}

func TestBuildPlanPrompt(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)

	p := &Planner{
		GitInfo: &agent.GitInfo{
			IsRepo: true,
			Branch: "feature",
		},
	}

	prompt := p.BuildPlanPrompt("Add pagination to API", dir)

	if !strings.Contains(prompt, "Mission Planning") {
		t.Error("prompt missing Mission Planning header")
	}
	if !strings.Contains(prompt, "Add pagination to API") {
		t.Error("prompt missing goal text")
	}
	if !strings.Contains(prompt, "Codebase Context") {
		t.Error("prompt missing codebase context section")
	}
	if !strings.Contains(prompt, "Instructions") {
		t.Error("prompt missing instructions section")
	}
	if !strings.Contains(prompt, "Branch: `feature`") {
		t.Error("prompt missing git branch info")
	}
}

func TestBuildPlanPromptEmptyDir(t *testing.T) {
	p := &Planner{}
	prompt := p.BuildPlanPrompt("Do something specific enough", "")

	if !strings.Contains(prompt, "Mission Planning") {
		t.Error("prompt missing Mission Planning header")
	}
	if !strings.Contains(prompt, "Do something specific enough") {
		t.Error("prompt missing goal text")
	}
	// With empty workDir, codebase context should be empty.
	if strings.Contains(prompt, "Directory Structure") {
		t.Error("should not have directory structure with empty workDir")
	}
}
