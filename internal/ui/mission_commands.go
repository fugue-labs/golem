package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/mission"
	"github.com/fugue-labs/golem/internal/ui/chat"
)

// handleMissionCommand dispatches /mission subcommands.
func (m *Model) handleMissionCommand(text string) (*chat.Message, tea.Cmd) {
	args := strings.TrimSpace(strings.TrimPrefix(text, "/mission"))

	switch {
	case args == "" || args == "help":
		return m.renderMissionHelpMessage(), nil

	case strings.HasPrefix(args, "new "):
		goal := strings.TrimSpace(strings.TrimPrefix(args, "new "))
		return m.handleMissionNew(goal), nil

	case args == "status":
		return m.handleMissionStatus(), nil

	case args == "tasks":
		return m.handleMissionTasks(), nil

	case args == "plan":
		return m.handleMissionPlan()

	case args == "approve":
		return m.handleMissionApprove(), nil

	case args == "start":
		return m.handleMissionStart(), nil

	case args == "pause":
		return m.handleMissionPause(), nil

	case args == "cancel":
		return m.handleMissionCancel(), nil

	case args == "list":
		return m.handleMissionList(), nil

	default:
		return &chat.Message{
			Kind:    chat.KindError,
			Content: fmt.Sprintf("Unknown mission command: `/mission%s`\nUse `/mission help` for available commands.", args),
		}, nil
	}
}

func (m *Model) renderMissionHelpMessage() *chat.Message {
	var b strings.Builder
	b.WriteString("**Mission commands**\n\n")
	b.WriteString("- `/mission new <goal>` — create a new mission from a goal\n")
	b.WriteString("- `/mission status` — show current mission status\n")
	b.WriteString("- `/mission tasks` — list tasks in the current mission\n")
	b.WriteString("- `/mission plan` — invoke planner to create task DAG\n")
	b.WriteString("- `/mission approve` — approve the pending plan\n")
	b.WriteString("- `/mission start` — start executing the mission\n")
	b.WriteString("- `/mission pause` — pause the active mission\n")
	b.WriteString("- `/mission cancel` — cancel the mission\n")
	b.WriteString("- `/mission list` — list all missions\n")
	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

func (m *Model) handleMissionNew(goal string) *chat.Message {
	if goal == "" {
		return &chat.Message{
			Kind:    chat.KindError,
			Content: "Usage: `/mission new <goal description>`",
		}
	}

	ctrl := m.missionController()
	if ctrl == nil {
		return &chat.Message{Kind: chat.KindError, Content: "Mission store not available."}
	}

	ctx := context.Background()
	title := goal
	if len(title) > 80 {
		title = title[:77] + "..."
	}

	created, err := ctrl.CreateMission(ctx, mission.CreateMissionRequest{
		Title:      title,
		Goal:       goal,
		RepoRoot:   m.cfg.WorkingDir,
		BaseBranch: m.gitBranch(),
		BaseCommit: m.gitCommit(),
		Budget: mission.Budget{
			MaxConcurrentWorkers: 3,
		},
	})
	if err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to create mission: %v", err)}
	}

	m.activeMissionID = created.ID

	var b strings.Builder
	fmt.Fprintf(&b, "**Mission created**: `%s`\n\n", created.ID)
	fmt.Fprintf(&b, "- **Title**: %s\n", created.Title)
	fmt.Fprintf(&b, "- **Status**: %s\n", created.Status)
	fmt.Fprintf(&b, "- **Goal**: %s\n\n", created.Goal)
	b.WriteString("Next: Use `/mission plan` to generate a task DAG, or send a message to have the agent plan for you.\n")

	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

func (m *Model) handleMissionStatus() *chat.Message {
	ctrl := m.missionController()
	if ctrl == nil {
		return &chat.Message{Kind: chat.KindError, Content: "Mission store not available."}
	}
	if m.activeMissionID == "" {
		return &chat.Message{Kind: chat.KindAssistant, Content: "No active mission. Use `/mission new <goal>` to create one."}
	}

	ctx := context.Background()
	summary, err := ctrl.GetMissionSummary(ctx, m.activeMissionID)
	if err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to get mission status: %v", err)}
	}

	return renderMissionSummaryMessage(summary)
}

func renderMissionSummaryMessage(summary *mission.MissionSummary) *chat.Message {
	m := summary.Mission
	tc := summary.TaskCounts

	var b strings.Builder
	fmt.Fprintf(&b, "**Mission**: %s\n", m.Title)
	fmt.Fprintf(&b, "**Status**: %s\n", m.Status)
	fmt.Fprintf(&b, "**ID**: `%s`\n\n", m.ID)

	if m.Goal != m.Title {
		fmt.Fprintf(&b, "**Goal**: %s\n\n", m.Goal)
	}

	if tc.Total > 0 {
		b.WriteString("**Tasks**\n\n")
		b.WriteString("| Status | Count |\n")
		b.WriteString("|--------|-------|\n")
		if tc.Ready > 0 {
			fmt.Fprintf(&b, "| Ready | %d |\n", tc.Ready)
		}
		if tc.Running > 0 {
			fmt.Fprintf(&b, "| Running | %d |\n", tc.Running)
		}
		if tc.AwaitingReview > 0 {
			fmt.Fprintf(&b, "| Awaiting Review | %d |\n", tc.AwaitingReview)
		}
		if tc.Done > 0 {
			fmt.Fprintf(&b, "| Done | %d |\n", tc.Done)
		}
		if tc.Blocked > 0 {
			fmt.Fprintf(&b, "| Blocked | %d |\n", tc.Blocked)
		}
		if tc.Failed > 0 {
			fmt.Fprintf(&b, "| Failed | %d |\n", tc.Failed)
		}
		if tc.Pending > 0 {
			fmt.Fprintf(&b, "| Pending | %d |\n", tc.Pending)
		}
		fmt.Fprintf(&b, "| **Total** | **%d** |\n\n", tc.Total)
	}

	if summary.ActiveRuns > 0 {
		fmt.Fprintf(&b, "**Active runs**: %d\n", summary.ActiveRuns)
	}
	if summary.PendingApprovals > 0 {
		fmt.Fprintf(&b, "**Pending approvals**: %d\n", summary.PendingApprovals)
	}

	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

func (m *Model) handleMissionTasks() *chat.Message {
	ctrl := m.missionController()
	if ctrl == nil {
		return &chat.Message{Kind: chat.KindError, Content: "Mission store not available."}
	}
	if m.activeMissionID == "" {
		return &chat.Message{Kind: chat.KindAssistant, Content: "No active mission."}
	}

	ctx := context.Background()
	tasks, err := ctrl.Store().ListTasks(ctx, m.activeMissionID)
	if err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to list tasks: %v", err)}
	}
	if len(tasks) == 0 {
		return &chat.Message{Kind: chat.KindAssistant, Content: "No tasks yet. Use `/mission plan` to generate a task DAG."}
	}

	deps, _ := ctrl.Store().ListDependencies(ctx, m.activeMissionID)
	depMap := make(map[string][]string) // task -> depends_on
	for _, d := range deps {
		depMap[d.TaskID] = append(depMap[d.TaskID], d.DependsOnID)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "**Mission tasks** (%d total)\n\n", len(tasks))

	for _, t := range tasks {
		icon := taskStatusIcon(t.Status)
		fmt.Fprintf(&b, "- %s `%s` [%s] **%s**", icon, t.ID, t.Status, t.Title)
		if t.Objective != "" && t.Objective != t.Title {
			fmt.Fprintf(&b, "\n  %s", t.Objective)
		}
		if depsOn := depMap[t.ID]; len(depsOn) > 0 {
			fmt.Fprintf(&b, "\n  depends on: %s", strings.Join(depsOn, ", "))
		}
		b.WriteString("\n")
	}

	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

func taskStatusIcon(s mission.TaskStatus) string {
	switch s {
	case mission.TaskDone, mission.TaskIntegrated, mission.TaskAccepted:
		return "✓"
	case mission.TaskRunning, mission.TaskLeased:
		return "◐"
	case mission.TaskBlocked, mission.TaskFailed, mission.TaskRejected:
		return "✗"
	case mission.TaskReady:
		return "●"
	case mission.TaskAwaitingReview:
		return "◎"
	default:
		return "○"
	}
}

func (m *Model) handleMissionPlan() (*chat.Message, tea.Cmd) {
	// This is a fallback for the old dispatch path; real work is in handleMissionPlanRun.
	return &chat.Message{Kind: chat.KindAssistant, Content: "Use `/mission plan` to invoke the planner."}, nil
}

// handleMissionPlanRun validates the goal, gathers codebase context, and
// triggers the agent to produce a streaming task DAG.
func (m *Model) handleMissionPlanRun() (tea.Model, tea.Cmd) {
	if m.activeMissionID == "" {
		m.messages = append(m.messages, &chat.Message{
			Kind: chat.KindAssistant, Content: "No active mission. Use `/mission new <goal>` first.",
		})
		m.scroll = 0
		return m, m.input.Focus()
	}
	if m.busy {
		m.messages = append(m.messages, &chat.Message{
			Kind: chat.KindAssistant, Content: "Cannot plan while the agent is running.",
		})
		m.scroll = 0
		return m, m.input.Focus()
	}

	ctrl := m.missionController()
	if ctrl == nil {
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindError, Content: "Mission store not available."})
		m.scroll = 0
		return m, m.input.Focus()
	}

	ctx := context.Background()
	ms, err := ctrl.GetMission(ctx, m.activeMissionID)
	if err != nil {
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to get mission: %v", err)})
		m.scroll = 0
		return m, m.input.Focus()
	}

	// (1) Reject vague goals.
	if isVagueGoal(ms.Goal) {
		m.messages = append(m.messages, &chat.Message{
			Kind: chat.KindAssistant,
			Content: fmt.Sprintf("The mission goal is too vague to plan:\n> %s\n\n"+
				"Please update the mission with a specific, actionable goal. For example:\n"+
				"- \"Add pagination to the /api/users endpoint\"\n"+
				"- \"Refactor the auth middleware to support OAuth2\"\n"+
				"- \"Fix the race condition in the worker pool shutdown\"\n\n"+
				"Use `/mission new <specific goal>` to create a new mission with a clearer goal.", ms.Goal),
		})
		m.scroll = 0
		return m, m.input.Focus()
	}

	// (2) Gather codebase context.
	codebaseCtx := gatherCodebaseContext(m.cfg.WorkingDir, m.runtime.Git)

	// (3) Build enriched planning prompt.
	prompt := buildPlanPrompt(ms, codebaseCtx)

	// (4) Trigger agent execution — same pattern as /spec.
	userMsg := &chat.Message{Kind: chat.KindUser, Content: "/mission plan"}
	m.messages = append(m.messages, userMsg)
	m.messages = append(m.messages, &chat.Message{
		Kind:    chat.KindSystem,
		Content: fmt.Sprintf("Planning mission `%s`: %s", ms.ID, ms.Goal),
	})
	m.inputHistory = append(m.inputHistory, "/mission plan")
	m.busy = true
	m.startTime = time.Now()
	m.lastPrompt = prompt
	m.currentRunMessages = []*chat.Message{userMsg}
	m.runID++
	m.hookRID.Store(int64(m.runID))
	m.runCtx, m.cancel = context.WithCancel(context.Background())
	m.scroll = 0
	return m, m.prepareRun(prompt)
}

// isVagueGoal returns true if the goal is too short or generic to plan against.
func isVagueGoal(goal string) bool {
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

// gatherCodebaseContext collects directory structure, key files, and git info
// for injection into the planning prompt.
func gatherCodebaseContext(workDir string, gitInfo *agent.GitInfo) string {
	var b strings.Builder

	// Git context.
	if gitInfo != nil && gitInfo.IsRepo {
		b.WriteString(agent.FormatGitContext(gitInfo))
		b.WriteString("\n\n")
	}

	// Directory structure (top 2 levels, skip hidden/vendor dirs).
	if workDir != "" {
		b.WriteString("# Directory Structure\n\n")
		b.WriteString("```\n")
		b.WriteString(scanDirectoryTree(workDir, 2))
		b.WriteString("```\n\n")

		// Key files — look for common entry points and config files.
		keyFiles := findKeyFiles(workDir)
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

// scanDirectoryTree returns a textual directory tree up to maxDepth levels.
func scanDirectoryTree(root string, maxDepth int) string {
	var b strings.Builder
	walkDir(root, root, 0, maxDepth, &b)
	return b.String()
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

// findKeyFiles identifies important files in the project root.
func findKeyFiles(root string) []string {
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

func buildPlanPrompt(ms *mission.Mission, codebaseCtx string) string {
	var b strings.Builder

	b.WriteString("# Mission Planning\n\n")
	b.WriteString("You are planning a mission for this codebase. Your job is to analyze the goal,\n")
	b.WriteString("understand the codebase, and produce a detailed task DAG.\n\n")

	b.WriteString("## Mission Goal\n\n")
	fmt.Fprintf(&b, "%s\n\n", ms.Goal)

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
	b.WriteString("Do NOT guess at file contents — read the actual code.\n")

	return b.String()
}

func (m *Model) handleMissionApprove() *chat.Message {
	if m.activeMissionID == "" {
		return &chat.Message{Kind: chat.KindAssistant, Content: "No active mission."}
	}

	ctrl := m.missionController()
	if ctrl == nil {
		return &chat.Message{Kind: chat.KindError, Content: "Mission store not available."}
	}

	ctx := context.Background()
	if err := ctrl.StartMission(ctx, m.activeMissionID); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to approve/start mission: %v", err)}
	}

	return &chat.Message{
		Kind:    chat.KindAssistant,
		Content: fmt.Sprintf("Mission `%s` approved and started.", m.activeMissionID),
	}
}

func (m *Model) handleMissionStart() *chat.Message {
	if m.activeMissionID == "" {
		return &chat.Message{Kind: chat.KindAssistant, Content: "No active mission. Use `/mission new <goal>` to create one."}
	}

	ctrl := m.missionController()
	if ctrl == nil {
		return &chat.Message{Kind: chat.KindError, Content: "Mission store not available."}
	}

	ctx := context.Background()
	ms, err := ctrl.GetMission(ctx, m.activeMissionID)
	if err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to get mission: %v", err)}
	}

	switch ms.Status {
	case mission.MissionDraft:
		return &chat.Message{
			Kind: chat.KindAssistant,
			Content: fmt.Sprintf("Mission `%s` is in **draft** state.\n\n"+
				"To start a mission, follow these steps:\n"+
				"1. `/mission plan` — generate a task DAG\n"+
				"2. `/mission approve` — approve the plan and start execution\n\n"+
				"Run `/mission plan` to continue.",
				ms.ID),
		}
	case mission.MissionPlanning:
		return &chat.Message{
			Kind:    chat.KindAssistant,
			Content: fmt.Sprintf("Mission `%s` is still being **planned**. Wait for planning to complete, then run `/mission approve`.", ms.ID),
		}
	case mission.MissionRunning:
		return &chat.Message{
			Kind:    chat.KindAssistant,
			Content: fmt.Sprintf("Mission `%s` is already **running**.", ms.ID),
		}
	}

	// For awaiting_approval or paused, proceed with start.
	if err := ctrl.StartMission(ctx, m.activeMissionID); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to start mission: %v", err)}
	}

	return &chat.Message{
		Kind:    chat.KindAssistant,
		Content: fmt.Sprintf("Mission `%s` started.", m.activeMissionID),
	}
}

func (m *Model) handleMissionPause() *chat.Message {
	if m.activeMissionID == "" {
		return &chat.Message{Kind: chat.KindAssistant, Content: "No active mission."}
	}

	ctrl := m.missionController()
	if ctrl == nil {
		return &chat.Message{Kind: chat.KindError, Content: "Mission store not available."}
	}

	ctx := context.Background()
	if err := ctrl.PauseMission(ctx, m.activeMissionID); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to pause mission: %v", err)}
	}

	return &chat.Message{Kind: chat.KindAssistant, Content: "Mission paused."}
}

func (m *Model) handleMissionCancel() *chat.Message {
	if m.activeMissionID == "" {
		return &chat.Message{Kind: chat.KindAssistant, Content: "No active mission."}
	}

	ctrl := m.missionController()
	if ctrl == nil {
		return &chat.Message{Kind: chat.KindError, Content: "Mission store not available."}
	}

	ctx := context.Background()
	if err := ctrl.CancelMission(ctx, m.activeMissionID); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to cancel mission: %v", err)}
	}

	m.activeMissionID = ""
	return &chat.Message{Kind: chat.KindAssistant, Content: "Mission cancelled."}
}

func (m *Model) handleMissionList() *chat.Message {
	ctrl := m.missionController()
	if ctrl == nil {
		return &chat.Message{Kind: chat.KindError, Content: "Mission store not available."}
	}

	ctx := context.Background()
	missions, err := ctrl.Store().ListMissions(ctx)
	if err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to list missions: %v", err)}
	}
	if len(missions) == 0 {
		return &chat.Message{Kind: chat.KindAssistant, Content: "No missions. Use `/mission new <goal>` to create one."}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "**Missions** (%d)\n\n", len(missions))
	for _, ms := range missions {
		active := ""
		if ms.ID == m.activeMissionID {
			active = " ← active"
		}
		fmt.Fprintf(&b, "- `%s` [%s] %s%s\n", ms.ID, ms.Status, ms.Title, active)
	}
	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

// gitBranch returns the current git branch or empty string.
func (m *Model) gitBranch() string {
	if m.runtime.Git != nil {
		return m.runtime.Git.BranchDisplay()
	}
	return ""
}

// gitCommit returns the current git HEAD commit or empty string.
func (m *Model) gitCommit() string {
	// GitInfo doesn't track HEAD commit hash; return empty for now.
	return ""
}
