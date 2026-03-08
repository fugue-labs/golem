package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fugue-labs/golem/internal/ui/chat"
	"github.com/fugue-labs/golem/internal/ui/styles"
)

func (m *Model) cancelActiveRun(asyncCleanup bool) {
	if m.cancel != nil {
		m.cancel()
	}
	m.cancel = nil
	m.runCtx = nil
	m.busy = false
	m.runID++
	m.hookRID.Store(int32(m.runID))
	m.agent = nil

	dropped := 0
	m.pendingMu.Lock()
	dropped = len(m.pendingMsgs)
	m.pendingMsgs = nil
	m.pendingMu.Unlock()

	if asyncCleanup {
		session := m.runtime.Session
		m.runtime.Session = nil
		if session != nil {
			go session.Cleanup()
		}
		message := "Run canceled."
		if dropped > 0 {
			message = fmt.Sprintf("Run canceled. Discarded %d queued follow-up(s).", dropped)
		}
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindAssistant, Content: message})
	}
}

func (m *Model) cleanupSession() {
	session := m.runtime.Session
	m.runtime.Session = nil
	if session != nil {
		session.Cleanup()
	}
}

func (m *Model) pendingCount() int {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	return len(m.pendingMsgs)
}

func (m *Model) hasWorkflowPanel() bool {
	return m.planState.HasTasks() || m.invariantState.HasItems()
}

func (m *Model) renderWorkflowPanel(height, width int) string {
	if height < 1 {
		height = 1
	}
	contentWidth := max(1, width-2)
	borderStr := lipgloss.NewStyle().Foreground(m.sty.BgSubtle).Render(styles.BorderThin) + " "
	sep := m.sty.Panel.Separator.Render(strings.Repeat(styles.Separator, contentWidth))

	headerLeft := m.sty.Panel.Title.Render("Workflow")
	headerRightText := workflowPanelSummary(m)
	headerRight := ""
	if headerRightText != "" {
		headerRight = m.sty.Panel.Progress.Render(headerRightText)
	}
	titleGap := contentWidth - lipgloss.Width(headerLeft) - lipgloss.Width(headerRight)
	if titleGap < 1 {
		titleGap = 1
	}
	titleLine := headerLeft + strings.Repeat(" ", titleGap) + headerRight

	var body []string
	showPlan := m.planState.HasTasks()
	showInv := m.invariantState.HasItems()
	bodyBudget := max(1, height-2)

	switch {
	case showPlan && showInv:
		bodyBudget = max(2, height-3)
		planBudget := max(1, bodyBudget/2)
		invBudget := max(1, bodyBudget-planBudget)
		body = append(body, m.renderPlanPanelLines(planBudget, contentWidth)...)
		body = append(body, sep)
		body = append(body, m.renderInvariantPanelLines(invBudget, contentWidth)...)
	case showPlan:
		body = append(body, m.renderPlanPanelLines(bodyBudget, contentWidth)...)
	case showInv:
		body = append(body, m.renderInvariantPanelLines(bodyBudget, contentWidth)...)
	default:
		body = append(body, m.sty.Muted.Render("No workflow state yet."))
	}

	allLines := append([]string{titleLine, sep}, body...)
	if len(allLines) > height {
		allLines = allLines[:height]
		allLines[len(allLines)-1] = m.sty.Muted.Render("...")
	}
	for len(allLines) < height {
		allLines = append(allLines, "")
	}
	for i, line := range allLines {
		full := borderStr + line
		if w := lipgloss.Width(full); w < width {
			full += strings.Repeat(" ", width-w)
		}
		allLines[i] = full
	}
	return strings.Join(allLines, "\n")
}

func workflowPanelSummary(m *Model) string {
	var parts []string
	if completed, total := m.planState.Progress(); total > 0 {
		parts = append(parts, fmt.Sprintf("plan %d/%d", completed, total))
	}
	if hardTotal, hardPass, hardFail, hardUnresolved, _, _, _ := m.invariantState.Counts(); hardTotal > 0 || m.invariantState.Extracted {
		parts = append(parts, fmt.Sprintf("inv %d✓ %d✗ %d?", hardPass, hardFail, hardUnresolved))
	}
	return strings.Join(parts, " · ")
}

func (m *Model) renderPlanPanelLines(limit, width int) []string {
	if limit <= 0 {
		return nil
	}
	completed, total := m.planState.Progress()
	lines := []string{m.sty.Panel.Progress.Render(fmt.Sprintf("Plan %d/%d completed", completed, total))}
	if limit == 1 {
		return lines
	}
	itemBudget := limit - 1
	maxItems := min(itemBudget, len(m.planState.Tasks))
	if len(m.planState.Tasks) > itemBudget && itemBudget > 0 {
		maxItems = itemBudget - 1
	}
	for i := 0; i < maxItems; i++ {
		t := m.planState.Tasks[i]
		icon := m.sty.Panel.IconPending.Render(styles.HollowIcon)
		switch t.Status {
		case "completed":
			icon = m.sty.Panel.IconCompleted.Render(styles.CheckIcon)
		case "in_progress":
			icon = m.sty.Panel.IconInProgress.Render(styles.InProgressIcon)
		case "blocked":
			icon = m.sty.Panel.IconBlocked.Render(styles.BlockedIcon)
		}
		desc := ansi.Truncate(t.Description, max(1, width-4), "...")
		if t.Status == "completed" {
			desc = m.sty.Panel.TaskDone.Render(desc)
		} else {
			desc = m.sty.Panel.TaskText.Render(desc)
		}
		lines = append(lines, fmt.Sprintf(" %s %s", icon, desc))
	}
	remaining := len(m.planState.Tasks) - maxItems
	if remaining > 0 && len(lines) < limit {
		lines = append(lines, m.sty.Muted.Render(fmt.Sprintf("... +%d plan tasks", remaining)))
	}
	for len(lines) < limit {
		lines = append(lines, "")
	}
	return lines[:limit]
}

func (m *Model) renderInvariantPanelLines(limit, width int) []string {
	if limit <= 0 {
		return nil
	}
	hardTotal, hardPass, hardFail, hardUnresolved, _, _, _ := m.invariantState.Counts()
	lines := []string{m.sty.Panel.Progress.Render(fmt.Sprintf("Inv %d✓ %d✗ %d?", hardPass, hardFail, hardUnresolved))}
	if hardTotal == 0 && !m.invariantState.Extracted {
		lines[0] = m.sty.Panel.Progress.Render("Inv pending")
	}
	if limit == 1 {
		return lines
	}
	itemBudget := limit - 1
	maxItems := min(itemBudget, len(m.invariantState.Items))
	if len(m.invariantState.Items) > itemBudget && itemBudget > 0 {
		maxItems = itemBudget - 1
	}
	for i := 0; i < maxItems; i++ {
		item := m.invariantState.Items[i]
		icon := m.sty.Panel.IconPending.Render(styles.HollowIcon)
		switch item.Status {
		case "pass":
			icon = m.sty.Panel.IconCompleted.Render(styles.CheckIcon)
		case "fail":
			icon = m.sty.Panel.IconBlocked.Render(styles.BlockedIcon)
		case "in_progress":
			icon = m.sty.Panel.IconInProgress.Render(styles.InProgressIcon)
		}
		kind := strings.ToUpper(strings.TrimSpace(item.Kind))
		if kind == "" {
			kind = "H"
		} else {
			kind = kind[:1]
		}
		label := fmt.Sprintf("%s %s %s", item.ID, kind, item.Description)
		label = ansi.Truncate(strings.TrimSpace(label), max(1, width-4), "...")
		if item.Status == "pass" {
			label = m.sty.Panel.TaskDone.Render(label)
		} else {
			label = m.sty.Panel.TaskText.Render(label)
		}
		lines = append(lines, fmt.Sprintf(" %s %s", icon, label))
	}
	remaining := len(m.invariantState.Items) - maxItems
	if remaining > 0 && len(lines) < limit {
		lines = append(lines, m.sty.Muted.Render(fmt.Sprintf("... +%d invariants", remaining)))
	}
	for len(lines) < limit {
		lines = append(lines, "")
	}
	return lines[:limit]
}
