package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/fugue-labs/golem/internal/mission"
	"github.com/fugue-labs/golem/internal/ui/chat"
	"github.com/fugue-labs/golem/internal/ui/styles"
)

// hasMission returns true if a mission controller is active with a mission.
func (m *Model) hasMission() bool {
	return m.missionCtrl != nil && m.missionSnap != nil
}

// refreshMissionSnapshot updates the cached mission snapshot from the controller.
func (m *Model) refreshMissionSnapshot() {
	if m.missionCtrl == nil || m.activeMissionID == "" {
		m.missionSnap = nil
		return
	}
	snap, err := m.missionCtrl.MissionStatus(m.activeMissionID)
	if err != nil {
		m.missionSnap = nil
		return
	}
	m.missionSnap = snap
}

// renderMissionPanelLines renders the mission status section for the workflow panel.
func (m *Model) renderMissionPanelLines(limit, width int) []string {
	if limit <= 0 || !m.hasMission() {
		return nil
	}

	snap := m.missionSnap
	mi := snap.Mission

	// Header line: Mission status + progress.
	statusIcon := missionStatusIcon(mi.Status)
	header := fmt.Sprintf("Mission %s %s", statusIcon, mi.Title)
	if len(header) > width {
		header = ansi.Truncate(header, max(1, width), "...")
	}
	lines := []string{m.sty.Panel.Progress.Render(header)}

	if limit == 1 {
		return lines
	}

	// Progress summary line.
	progress := fmt.Sprintf("%d/%d done · %d running · %d ready · %d blocked",
		snap.DoneTasks, snap.TotalTasks, snap.RunningTasks, snap.ReadyTasks, snap.BlockedTasks)
	if snap.ReviewTasks > 0 {
		progress += fmt.Sprintf(" · %d review", snap.ReviewTasks)
	}
	progress = ansi.Truncate(progress, max(1, width-2), "...")
	lines = append(lines, " "+m.sty.Muted.Render(progress))

	if limit <= 2 {
		return lines[:limit]
	}

	// Worker cards.
	cardBudget := limit - len(lines)
	if len(snap.WorkerCards) > 0 && cardBudget > 0 {
		maxCards := min(cardBudget, len(snap.WorkerCards))
		for i := range maxCards {
			card := snap.WorkerCards[i]
			icon := m.sty.Panel.IconInProgress.Render(styles.InProgressIcon)
			if card.Status == mission.RunQueued {
				icon = m.sty.Panel.IconPending.Render(styles.PendingIcon)
			}

			elapsed := ""
			if card.StartedAt != nil {
				d := time.Since(*card.StartedAt).Truncate(time.Second)
				elapsed = " " + d.String()
			}

			label := fmt.Sprintf("%s %s%s", card.TaskID, card.TaskTitle, elapsed)
			label = ansi.Truncate(label, max(1, width-4), "...")
			lines = append(lines, fmt.Sprintf(" %s %s", icon, m.sty.Panel.TaskText.Render(label)))
		}
		remaining := len(snap.WorkerCards) - maxCards
		if remaining > 0 && len(lines) < limit {
			lines = append(lines, m.sty.Muted.Render(fmt.Sprintf("  ... +%d workers", remaining)))
		}
	}

	for len(lines) < limit {
		lines = append(lines, "")
	}
	return lines[:limit]
}

// missionStatusIcon returns a compact icon for a mission status.
func missionStatusIcon(status mission.MissionStatus) string {
	switch status {
	case mission.MissionDraft:
		return "○"
	case mission.MissionPlanning:
		return "◐"
	case mission.MissionRunning:
		return "▶"
	case mission.MissionPaused:
		return "⏸"
	case mission.MissionBlocked:
		return "✗"
	case mission.MissionCompleting:
		return "◐"
	case mission.MissionCompleted:
		return "✓"
	case mission.MissionFailed:
		return "✗"
	case mission.MissionCancelled:
		return "×"
	default:
		return "?"
	}
}

// handleMissionCommand processes /mission subcommands.
func (m *Model) handleMissionCommand(text string) *chat.Message {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		return m.renderMissionStatusMessage()
	}

	sub := parts[1]
	switch sub {
	case "status":
		return m.renderMissionStatusMessage()
	case "new":
		if len(parts) < 3 {
			return &chat.Message{
				Kind:    chat.KindError,
				Content: "Usage: /mission new <goal description>",
			}
		}
		goal := strings.Join(parts[2:], " ")
		return m.createMission(goal)
	case "start":
		return m.startMission()
	case "pause":
		return m.pauseMission()
	case "resume":
		return m.resumeMission()
	case "cancel":
		return m.cancelMission()
	default:
		return &chat.Message{
			Kind:    chat.KindError,
			Content: fmt.Sprintf("Unknown mission command: %s. Try: status, new, start, pause, resume, cancel", sub),
		}
	}
}

func (m *Model) renderMissionStatusMessage() *chat.Message {
	if !m.hasMission() {
		return &chat.Message{
			Kind:    chat.KindAssistant,
			Content: "No active mission. Use `/mission new <goal>` to create one.",
		}
	}

	m.refreshMissionSnapshot()
	snap := m.missionSnap
	mi := snap.Mission

	var b strings.Builder
	fmt.Fprintf(&b, "**Mission: %s** (%s)\n\n", mi.Title, mi.Status)
	fmt.Fprintf(&b, "- Goal: %s\n", mi.Goal)
	fmt.Fprintf(&b, "- Tasks: %d total, %d done, %d running, %d ready, %d blocked\n",
		snap.TotalTasks, snap.DoneTasks, snap.RunningTasks, snap.ReadyTasks, snap.BlockedTasks)
	if snap.ReviewTasks > 0 {
		fmt.Fprintf(&b, "- Awaiting review: %d\n", snap.ReviewTasks)
	}

	if len(snap.WorkerCards) > 0 {
		b.WriteString("\n**Active Workers**\n\n")
		for _, card := range snap.WorkerCards {
			elapsed := ""
			if card.StartedAt != nil {
				d := time.Since(*card.StartedAt).Truncate(time.Second)
				elapsed = " (" + d.String() + ")"
			}
			icon := "◐"
			if card.Status == mission.RunQueued {
				icon = "●"
			}
			fmt.Fprintf(&b, "- %s `%s` — %s%s\n", icon, card.RunID, card.TaskTitle, elapsed)
		}
	}

	if len(snap.Tasks) > 0 {
		b.WriteString("\n**Tasks**\n\n")
		for _, t := range snap.Tasks {
			icon := taskStatusIcon(t.Status)
			fmt.Fprintf(&b, "- %s `%s` [%s] %s\n", icon, t.ID, t.Status, t.Title)
		}
	}

	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

func (m *Model) createMission(goal string) *chat.Message {
	if m.missionCtrl == nil {
		// Initialize controller on first use.
		dbPath := mission.MissionStoreDir(m.cfg.WorkingDir)
		ctrl, err := mission.NewController(dbPath, m.cfg.WorkingDir)
		if err != nil {
			return &chat.Message{
				Kind:    chat.KindError,
				Content: fmt.Sprintf("Failed to initialize mission controller: %v", err),
			}
		}
		m.missionCtrl = ctrl
		// Register change callback.
		if m.prog != nil {
			ctrl.SetOnChange(func() {
				m.prog.Send(missionChangedMsg{})
			})
		}
	}

	// Use goal as title (truncated) and full goal.
	title := goal
	if len(title) > 60 {
		title = title[:57] + "..."
	}

	baseBranch := "HEAD"
	if m.runtime.Git != nil && m.runtime.Git.Branch != "" {
		baseBranch = m.runtime.Git.Branch
	}

	mi, err := m.missionCtrl.CreateMission(title, goal, baseBranch)
	if err != nil {
		return &chat.Message{
			Kind:    chat.KindError,
			Content: fmt.Sprintf("Failed to create mission: %v", err),
		}
	}
	m.activeMissionID = mi.ID
	m.refreshMissionSnapshot()

	return &chat.Message{
		Kind:    chat.KindAssistant,
		Content: fmt.Sprintf("Mission created: `%s` — %s\nUse the planner to add tasks, then `/mission start` to begin execution.", mi.ID, mi.Title),
	}
}

func (m *Model) startMission() *chat.Message {
	if m.missionCtrl == nil || m.activeMissionID == "" {
		return &chat.Message{Kind: chat.KindError, Content: "No active mission to start."}
	}
	if err := m.missionCtrl.StartMission(m.activeMissionID); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to start mission: %v", err)}
	}
	m.refreshMissionSnapshot()
	return &chat.Message{Kind: chat.KindAssistant, Content: "Mission started. Scheduler is selecting ready tasks."}
}

func (m *Model) pauseMission() *chat.Message {
	if m.missionCtrl == nil || m.activeMissionID == "" {
		return &chat.Message{Kind: chat.KindError, Content: "No active mission to pause."}
	}
	if err := m.missionCtrl.PauseMission(m.activeMissionID); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to pause mission: %v", err)}
	}
	m.refreshMissionSnapshot()
	return &chat.Message{Kind: chat.KindAssistant, Content: "Mission paused. Active workers will finish but no new tasks will be scheduled."}
}

func (m *Model) resumeMission() *chat.Message {
	if m.missionCtrl == nil || m.activeMissionID == "" {
		return &chat.Message{Kind: chat.KindError, Content: "No active mission to resume."}
	}
	if err := m.missionCtrl.ResumeMission(m.activeMissionID); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to resume: %v", err)}
	}
	m.refreshMissionSnapshot()
	return &chat.Message{Kind: chat.KindAssistant, Content: "Mission resumed. Scheduler is selecting ready tasks."}
}

func (m *Model) cancelMission() *chat.Message {
	if m.missionCtrl == nil || m.activeMissionID == "" {
		return &chat.Message{Kind: chat.KindError, Content: "No active mission to cancel."}
	}
	if err := m.missionCtrl.CancelMission(m.activeMissionID); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to cancel mission: %v", err)}
	}
	m.activeMissionID = ""
	m.missionSnap = nil
	return &chat.Message{Kind: chat.KindAssistant, Content: "Mission cancelled. All active workers stopped and worktrees cleaned up."}
}

func taskStatusIcon(status mission.TaskStatus) string {
	switch status {
	case mission.TaskPending:
		return "○"
	case mission.TaskReady:
		return "●"
	case mission.TaskLeased, mission.TaskRunning:
		return "◐"
	case mission.TaskAwaitingReview:
		return "◑"
	case mission.TaskAccepted, mission.TaskIntegrated, mission.TaskDone:
		return "✓"
	case mission.TaskRejected, mission.TaskFailed:
		return "✗"
	case mission.TaskBlocked:
		return "⊘"
	case mission.TaskSuperseded:
		return "×"
	default:
		return "?"
	}
}
