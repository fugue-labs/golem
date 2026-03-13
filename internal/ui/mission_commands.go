package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/fugue-labs/golem/internal/mission"
	"github.com/fugue-labs/golem/internal/ui/chat"
)

// handleMissionCommand dispatches /mission subcommands.
func (m *Model) handleMissionCommand(text string) *chat.Message {
	args := strings.TrimSpace(strings.TrimPrefix(text, "/mission"))

	switch {
	case args == "" || args == "help":
		return m.renderMissionHelpMessage()

	case strings.HasPrefix(args, "new "):
		goal := strings.TrimSpace(strings.TrimPrefix(args, "new "))
		return m.handleMissionNew(goal)

	case args == "status":
		return m.handleMissionStatus()

	case args == "tasks":
		return m.handleMissionTasks()

	case args == "plan":
		return m.handleMissionPlan()

	case args == "approve":
		return m.handleMissionApprove()

	case args == "start":
		return m.handleMissionStart()

	case args == "pause":
		return m.handleMissionPause()

	case args == "cancel":
		return m.handleMissionCancel()

	case args == "list":
		return m.handleMissionList()

	default:
		return &chat.Message{
			Kind:    chat.KindError,
			Content: fmt.Sprintf("Unknown mission command: `/mission%s`\nUse `/mission help` for available commands.", args),
		}
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

func (m *Model) handleMissionPlan() *chat.Message {
	if m.activeMissionID == "" {
		return &chat.Message{Kind: chat.KindAssistant, Content: "No active mission. Use `/mission new <goal>` first."}
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

	// Queue a planning prompt for the agent.
	planPrompt := fmt.Sprintf(
		"You are planning a mission. The user's goal is:\n\n%s\n\n"+
			"Analyze the codebase and create a task DAG for this mission. "+
			"For each task, specify: ID, title, kind (code/test/docs/investigation), "+
			"objective, priority (higher=more important), writable scope (file paths), "+
			"acceptance criteria, estimated effort, and risk level.\n"+
			"Also specify task dependencies (which tasks depend on which).\n\n"+
			"Respond with a structured plan that can be reviewed before execution begins.",
		ms.Goal,
	)

	return &chat.Message{
		Kind:    chat.KindAssistant,
		Content: fmt.Sprintf("Planning mission `%s`...\n\nSending goal to agent for task decomposition:\n> %s", ms.ID, planPrompt),
	}
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
	return m.handleMissionApprove() // Same as approve for now.
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
