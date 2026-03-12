package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// InstructionFile records a discovered project instruction file.
type InstructionFile struct {
	Path    string // absolute path
	Content string // file contents
}

// DiscoverInstructions searches for project instruction files following a
// precedence similar to Claude Code's CLAUDE.md system. It searches:
//
//  1. ~/.golem/instructions.md (user-level)
//  2. Walk from workingDir up to git root (or home), checking each directory for:
//     - GOLEM.md
//     - CLAUDE.md
//     - .golem/instructions.md
//  3. .golem/instructions.md in workingDir (project-level, highest precedence)
//
// Files are returned in precedence order (user-level first, project-level last)
// so they can be concatenated with later entries overriding earlier ones.
func DiscoverInstructions(workingDir string) []InstructionFile {
	var files []InstructionFile
	seen := make(map[string]bool)

	addIfExists := func(path string) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		if seen[abs] {
			return
		}
		content, err := os.ReadFile(abs)
		if err != nil {
			return
		}
		trimmed := strings.TrimSpace(string(content))
		if trimmed == "" {
			return
		}
		seen[abs] = true
		files = append(files, InstructionFile{Path: abs, Content: trimmed})
	}

	// 1. User-level instructions.
	if home, err := os.UserHomeDir(); err == nil {
		addIfExists(filepath.Join(home, ".golem", "instructions.md"))
	}

	// 2. Walk parent directories up to git root or filesystem root.
	gitRoot := findGitRoot(workingDir)
	stopAt := gitRoot
	if stopAt == "" {
		if home, err := os.UserHomeDir(); err == nil {
			stopAt = home
		} else {
			stopAt = "/"
		}
	}

	// Collect directories from stopAt down to workingDir (ancestors first).
	var dirs []string
	dir := workingDir
	for {
		dirs = append(dirs, dir)
		if dir == stopAt {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // filesystem root
		}
		dir = parent
	}
	// Reverse so ancestors come first (lower precedence).
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}

	for _, d := range dirs {
		addIfExists(filepath.Join(d, "GOLEM.md"))
		addIfExists(filepath.Join(d, "CLAUDE.md"))
		addIfExists(filepath.Join(d, ".golem", "instructions.md"))
	}

	return files
}

// FormatInstructions formats discovered instruction files for injection into
// the system prompt.
func FormatInstructions(files []InstructionFile) string {
	if len(files) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Project Instructions\n\n")
	b.WriteString("The following project-level instructions were discovered and should be followed:\n\n")
	for _, f := range files {
		shortPath := shortFilePath(f.Path)
		b.WriteString("## From: ")
		b.WriteString(shortPath)
		b.WriteString("\n\n")
		b.WriteString(f.Content)
		b.WriteString("\n\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// findGitRoot walks up from dir looking for a .git directory.
func findGitRoot(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
			return abs
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return ""
		}
		abs = parent
	}
}

// shortFilePath replaces the home directory prefix with ~.
func shortFilePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}
