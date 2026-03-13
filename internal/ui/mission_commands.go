package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
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

	ctx := m.appCtx
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

	ctx := m.appCtx
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

	ctx := m.appCtx
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

// handleMissionPlan validates the goal, gathers codebase context, and
// returns a tea.Cmd that triggers the planner agent run. It flows through
// the same dispatch path as all other /mission subcommands.
func (m *Model) handleMissionPlan() (*chat.Message, tea.Cmd) {
	if m.activeMissionID == "" {
		return &chat.Message{Kind: chat.KindAssistant, Content: "No active mission. Use `/mission new <goal>` first."}, nil
	}
	if m.busy {
		return &chat.Message{Kind: chat.KindAssistant, Content: "Cannot plan while the agent is running."}, nil
	}

	ctrl := m.missionController()
	if ctrl == nil {
		return &chat.Message{Kind: chat.KindError, Content: "Mission store not available."}, nil
	}

	ctx := m.appCtx
	ms, err := ctrl.GetMission(ctx, m.activeMissionID)
	if err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to get mission: %v", err)}, nil
	}

	// Build planner and reject vague goals.
	planner := &mission.Planner{GitInfo: m.runtime.Git}
	if planner.IsVagueGoal(ms.Goal) {
		return &chat.Message{
			Kind: chat.KindAssistant,
			Content: fmt.Sprintf("The mission goal is too vague to plan:\n> %s\n\n"+
				"Please update the mission with a specific, actionable goal. For example:\n"+
				"- \"Add pagination to the /api/users endpoint\"\n"+
				"- \"Refactor the auth middleware to support OAuth2\"\n"+
				"- \"Fix the race condition in the worker pool shutdown\"\n\n"+
				"Use `/mission new <specific goal>` to create a new mission with a clearer goal.", ms.Goal),
		}, nil
	}

	switch ms.Status {
	case mission.MissionAwaitingApproval:
		return &chat.Message{Kind: chat.KindAssistant, Content: fmt.Sprintf("Mission `%s` already has a pending plan. Run `/mission approve` to start execution.", ms.ID)}, nil
	case mission.MissionRunning:
		return &chat.Message{Kind: chat.KindAssistant, Content: fmt.Sprintf("Mission `%s` is already running. Re-planning through `/mission plan` is not supported yet.", ms.ID)}, nil
	case mission.MissionPaused, mission.MissionBlocked, mission.MissionCompleting, mission.MissionCompleted, mission.MissionFailed, mission.MissionCancelled:
		return &chat.Message{Kind: chat.KindAssistant, Content: fmt.Sprintf("Mission `%s` is %s. Create a new mission to plan a fresh DAG.", ms.ID, ms.Status)}, nil
	}

	previousStatus := ms.Status
	ms.Status = mission.MissionPlanning
	ms.UpdatedAt = time.Now().UTC()
	if err := ctrl.Store().UpdateMission(ctx, ms); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to mark mission as planning: %v", err)}, nil
	}
	m.missionPlanRun = &missionPlanRun{MissionID: ms.ID, PreviousStatus: previousStatus}

	// Build enriched planning prompt (gathers codebase context internally).
	prompt := planner.BuildPlanPrompt(ms.Goal, m.cfg.WorkingDir)

	// Add planning status message and trigger agent run.
	m.messages = append(m.messages, &chat.Message{
		Kind:    chat.KindSystem,
		Content: fmt.Sprintf("Planning mission `%s`: %s", ms.ID, ms.Goal),
	})
	m.inputHistory = append(m.inputHistory, "/mission plan")

	statusMsg := &chat.Message{Kind: chat.KindUser, Content: "/mission plan"}
	return statusMsg, m.beginRun(prompt, []*chat.Message{statusMsg})
}

func (m *Model) handleMissionApprove() *chat.Message {
	if m.activeMissionID == "" {
		return &chat.Message{Kind: chat.KindAssistant, Content: "No active mission."}
	}

	ctrl := m.missionController()
	if ctrl == nil {
		return &chat.Message{Kind: chat.KindError, Content: "Mission store not available."}
	}

	ctx := m.appCtx
	if err := ctrl.StartMission(ctx, m.activeMissionID); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to approve/start mission: %v", err)}
	}

	m.startOrchestrator()

	return &chat.Message{
		Kind:    chat.KindAssistant,
		Content: fmt.Sprintf("Mission `%s` approved and started. Orchestrator is running.", m.activeMissionID),
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

	ctx := m.appCtx
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

	m.startOrchestrator()

	return &chat.Message{
		Kind:    chat.KindAssistant,
		Content: fmt.Sprintf("Mission `%s` started. Orchestrator is running.", m.activeMissionID),
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

	m.stopOrchestrator()

	ctx := m.appCtx
	if err := ctrl.PauseMission(ctx, m.activeMissionID); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to pause mission: %v", err)}
	}

	return &chat.Message{Kind: chat.KindAssistant, Content: "Mission paused. Orchestrator stopped."}
}

func (m *Model) handleMissionCancel() *chat.Message {
	if m.activeMissionID == "" {
		return &chat.Message{Kind: chat.KindAssistant, Content: "No active mission."}
	}

	ctrl := m.missionController()
	if ctrl == nil {
		return &chat.Message{Kind: chat.KindError, Content: "Mission store not available."}
	}

	m.stopOrchestrator()

	ctx := m.appCtx
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

	ctx := m.appCtx
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

// startOrchestrator creates and starts the mission orchestrator.
// It wires orchestrator events back to the TUI via tea.Program.Send.
func (m *Model) startOrchestrator() {
	m.stopOrchestrator() // stop any existing orchestrator

	ctrl := m.missionController()
	if ctrl == nil {
		return
	}

	worktrees := mission.NewWorktreeManager(m.cfg.WorkingDir)
	spawner := newGollemSpawner(m.cfg, m.activeSkills)

	cfg := mission.OrchestratorConfig{
		MissionID: m.activeMissionID,
		RepoRoot:  m.cfg.WorkingDir,
	}

	m.orchestrator = mission.NewOrchestrator(cfg, ctrl.Store(), spawner, worktrees, func(e mission.OrchestratorEvent) {
		if m.prog != nil {
			m.prog.Send(orchestratorEventMsg(e))
		}
	})
	m.orchestrator.Start(m.appCtx)
}

// stopOrchestrator stops the orchestrator if running.
func (m *Model) stopOrchestrator() {
	if m.orchestrator != nil {
		m.orchestrator.Stop()
		m.orchestrator = nil
	}
}

// orchestratorEventMsg wraps an OrchestratorEvent as a BubbleTea message.
type orchestratorEventMsg mission.OrchestratorEvent

// handleOrchestratorEvent displays orchestrator lifecycle events in the chat.
func (m *Model) handleOrchestratorEvent(e mission.OrchestratorEvent) *Model {
	var content string
	kind := chat.KindSystem

	switch e.Type {
	case "worker.started":
		content = fmt.Sprintf("Worker started for task `%s`: %s", e.TaskID, e.Message)
	case "worker.completed":
		content = fmt.Sprintf("Worker completed task `%s`", e.TaskID)
	case "worker.failed":
		content = fmt.Sprintf("Worker failed for task `%s`: %v", e.TaskID, e.Error)
		kind = chat.KindError
	case "worker.spawn_failed":
		content = fmt.Sprintf("Failed to spawn worker for task `%s`: %v", e.TaskID, e.Error)
		kind = chat.KindError
	case "review.started":
		content = fmt.Sprintf("Review started for task `%s`", e.TaskID)
	case "review.pass":
		content = fmt.Sprintf("Review passed for task `%s`: %s", e.TaskID, e.Message)
	case "review.reject":
		content = fmt.Sprintf("Review rejected task `%s`: %s", e.TaskID, e.Message)
	case "review.request_changes":
		content = fmt.Sprintf("Review requested changes for task `%s`: %s", e.TaskID, e.Message)
	case "review.failed", "review.parse_failed":
		content = fmt.Sprintf("Review failed for task `%s`: %v", e.TaskID, e.Error)
		kind = chat.KindError
	case "integration.completed":
		content = fmt.Sprintf("Task `%s` integrated: %s", e.TaskID, e.Message)
	case "integration.failed":
		content = fmt.Sprintf("Integration failed for task `%s`: %s", e.TaskID, e.Message)
		kind = chat.KindError
	case "mission.completed":
		content = fmt.Sprintf("Mission `%s` completed! %s", e.MissionID, e.Message)
		m.stopOrchestrator()
	default:
		return m // ignore unknown events
	}

	m.messages = append(m.messages, &chat.Message{Kind: kind, Content: content})
	m.scroll = 0
	return m
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
