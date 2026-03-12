package agent

import (
	"fmt"
	"os/exec"
	"strings"
)

// GitInfo captures git repository state for the working directory.
type GitInfo struct {
	IsRepo        bool
	Branch        string
	IsDirty       bool
	RecentCommits []string // last 5 one-line commit summaries
}

// GatherGitInfo collects git state from the given directory.
// Returns nil if the directory is not a git repository.
func GatherGitInfo(dir string) *GitInfo {
	if !isGitRepo(dir) {
		return nil
	}

	info := &GitInfo{IsRepo: true}

	// Current branch.
	if out, err := gitCmd(dir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		info.Branch = strings.TrimSpace(out)
	}

	// Dirty status.
	if out, err := gitCmd(dir, "status", "--porcelain", "--untracked-files=normal"); err == nil {
		info.IsDirty = strings.TrimSpace(out) != ""
	}

	// Recent commits (last 5).
	if out, err := gitCmd(dir, "log", "--oneline", "-5", "--no-decorate"); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				info.RecentCommits = append(info.RecentCommits, trimmed)
			}
		}
	}

	return info
}

// FormatGitContext formats git info for injection into the system prompt.
func FormatGitContext(info *GitInfo) string {
	if info == nil || !info.IsRepo {
		return ""
	}

	var b strings.Builder
	b.WriteString("# Git Context\n\n")
	fmt.Fprintf(&b, "- Branch: `%s`\n", info.Branch)
	if info.IsDirty {
		b.WriteString("- Status: uncommitted changes\n")
	} else {
		b.WriteString("- Status: clean\n")
	}

	if len(info.RecentCommits) > 0 {
		b.WriteString("\nRecent commits:\n")
		for _, c := range info.RecentCommits {
			fmt.Fprintf(&b, "- %s\n", c)
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// BranchDisplay returns a short display string for the header bar.
func (g *GitInfo) BranchDisplay() string {
	if g == nil || !g.IsRepo || g.Branch == "" {
		return ""
	}
	if g.IsDirty {
		return g.Branch + "*"
	}
	return g.Branch
}

func gitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}
