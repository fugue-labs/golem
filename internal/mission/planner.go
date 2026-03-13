package mission

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fugue-labs/golem/internal/agent"
)

// Planner constructs enriched planning prompts from mission goals and codebase
// context. It encapsulates the domain logic for goal validation, directory
// scanning, key-file discovery, and prompt assembly — none of which depends on
// any UI types.
type Planner struct {
	GitInfo *agent.GitInfo
}

// BuildPlanPrompt gathers codebase context for repoRoot and returns a
// ready-to-send planning prompt for the given goal.
func (p *Planner) BuildPlanPrompt(goal, repoRoot string) string {
	codebaseCtx := p.GatherCodebaseContext(repoRoot)

	var b strings.Builder

	b.WriteString("# Mission Planning\n\n")
	b.WriteString("You are planning a mission for this codebase. Your job is to analyze the goal,\n")
	b.WriteString("understand the codebase, and produce a detailed task DAG.\n\n")

	b.WriteString("## Mission Goal\n\n")
	fmt.Fprintf(&b, "%s\n\n", goal)

	if codebaseCtx != "" {
		b.WriteString("## Codebase Context\n\n")
		b.WriteString(codebaseCtx)
	}

	b.WriteString("## Instructions\n\n")
	b.WriteString("1. Analyze the codebase to understand its structure, conventions, and relevant code paths.\n")
	b.WriteString("2. Break the goal into concrete, atomic tasks.\n")
	b.WriteString("3. For each task, specify:\n")
	b.WriteString("   - **ID**: Short unique identifier (e.g., t1, t2)\n")
	b.WriteString("   - **Title**: Concise description\n")
	b.WriteString("   - **Kind**: code, test, docs, or investigation\n")
	b.WriteString("   - **Objective**: What this task accomplishes\n")
	b.WriteString("   - **Priority**: 1 (highest) to 5 (lowest)\n")
	b.WriteString("   - **Scope**: File paths this task will modify\n")
	b.WriteString("   - **Acceptance criteria**: How to verify completion\n")
	b.WriteString("   - **Estimated effort**: small, medium, or large\n")
	b.WriteString("   - **Risk level**: low, medium, or high\n")
	b.WriteString("4. Specify task dependencies (which tasks depend on which).\n")
	b.WriteString("5. Present the plan clearly so it can be reviewed before execution.\n\n")
	b.WriteString("Use the codebase tools (glob, grep, view) to examine relevant files before producing your plan.\n")
	b.WriteString("Do NOT guess at file contents — read the actual code.\n\n")
	b.WriteString("## Required Output\n\n")
	b.WriteString("Return exactly one JSON object in a ```json fenced block with this shape:\n\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString("  \"summary\": \"Short overview of the plan\",\n")
	b.WriteString("  \"success_criteria\": [\"criterion 1\", \"criterion 2\"],\n")
	b.WriteString("  \"tasks\": [\n")
	b.WriteString("    {\n")
	b.WriteString("      \"id\": \"t1\",\n")
	b.WriteString("      \"title\": \"Concise task title\",\n")
	b.WriteString("      \"kind\": \"code\",\n")
	b.WriteString("      \"objective\": \"What this task accomplishes\",\n")
	b.WriteString("      \"priority\": 1,\n")
	b.WriteString("      \"scope\": {\n")
	b.WriteString("        \"write_paths\": [\"path/to/file.go\"],\n")
	b.WriteString("        \"read_paths\": [\"internal/...\"]\n")
	b.WriteString("      },\n")
	b.WriteString("      \"acceptance_criteria\": [\"How to verify completion\"],\n")
	b.WriteString("      \"estimated_effort\": \"small\",\n")
	b.WriteString("      \"risk_level\": \"low\"\n")
	b.WriteString("    }\n")
	b.WriteString("  ],\n")
	b.WriteString("  \"dependencies\": [\n")
	b.WriteString("    {\"task_id\": \"t2\", \"depends_on_id\": \"t1\"}\n")
	b.WriteString("  ]\n")
	b.WriteString("}\n")
	b.WriteString("```\n")
	b.WriteString("Do not include commentary before or after the JSON block.\n")

	return b.String()
}

// GatherCodebaseContext collects directory structure, key files, and git info
// for injection into the planning prompt.
func (p *Planner) GatherCodebaseContext(workDir string) string {
	var b strings.Builder

	// Git context.
	if p.GitInfo != nil && p.GitInfo.IsRepo {
		b.WriteString(agent.FormatGitContext(p.GitInfo))
		b.WriteString("\n\n")
	}

	// Directory structure (top 2 levels, skip hidden/vendor dirs).
	if workDir != "" {
		b.WriteString("# Directory Structure\n\n")
		b.WriteString("```\n")
		b.WriteString(p.ScanDirectoryTree(workDir, 2))
		b.WriteString("```\n\n")

		// Key files — look for common entry points and config files.
		keyFiles := p.FindKeyFiles(workDir)
		if len(keyFiles) > 0 {
			b.WriteString("# Key Files\n\n")
			for _, f := range keyFiles {
				fmt.Fprintf(&b, "- `%s`\n", f)
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

// IsVagueGoal returns true if the goal is too short or generic to plan against.
func (p *Planner) IsVagueGoal(goal string) bool {
	g := strings.TrimSpace(goal)
	if len(g) < 10 {
		return true
	}
	// Reject common non-actionable phrases.
	lower := strings.ToLower(g)
	vaguePhrases := []string{
		"do it", "let's do it", "let's go", "make it work",
		"fix it", "just do it", "go ahead", "start", "begin",
		"help me", "do something", "do the thing",
	}
	for _, vp := range vaguePhrases {
		if lower == vp {
			return true
		}
	}
	return false
}

// ScanDirectoryTree returns a textual directory tree up to maxDepth levels.
func (p *Planner) ScanDirectoryTree(root string, maxDepth int) string {
	var b strings.Builder
	walkDir(root, root, 0, maxDepth, &b)
	return b.String()
}

// FindKeyFiles identifies important files in the project root.
func (p *Planner) FindKeyFiles(root string) []string {
	candidates := []string{
		"go.mod", "go.sum", "package.json", "Cargo.toml", "pyproject.toml",
		"Makefile", "Dockerfile", "docker-compose.yml", "docker-compose.yaml",
		"README.md", "CLAUDE.md", "GOLEM.md",
		"main.go", "cmd/main.go",
	}
	var found []string
	for _, c := range candidates {
		p := filepath.Join(root, c)
		if _, err := os.Stat(p); err == nil {
			found = append(found, c)
		}
	}
	return found
}

func walkDir(root, dir string, depth, maxDepth int, b *strings.Builder) {
	if depth > maxDepth {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	skipDirs := map[string]bool{
		".git": true, "node_modules": true, "vendor": true,
		".next": true, "__pycache__": true, ".cache": true,
		"dist": true, "build": true, ".idea": true, ".vscode": true,
	}

	indent := strings.Repeat("  ", depth)
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") && depth == 0 && name != ".github" {
			continue
		}
		if e.IsDir() {
			if skipDirs[name] {
				continue
			}
			fmt.Fprintf(b, "%s%s/\n", indent, name)
			walkDir(root, filepath.Join(dir, name), depth+1, maxDepth, b)
		} else if depth < maxDepth {
			fmt.Fprintf(b, "%s%s\n", indent, name)
		}
	}
}
