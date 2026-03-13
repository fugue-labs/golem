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
	m.resetAskState()
	m.runID++
	m.hookRID.Store(int64(m.runID))
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
	if m.runtime.MCPManager != nil {
		m.runtime.MCPManager.Close()
		m.runtime.MCPManager = nil
	}
	if closer, ok := m.runtime.MemoryStore.(interface{ Close() error }); ok {
		closer.Close()
		m.runtime.MemoryStore = nil
	}
}

func (m *Model) pendingCount() int {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	return len(m.pendingMsgs)
}

func (m *Model) hasWorkflowPanel() bool {
	return m.planState.HasTasks() || m.invariantState.HasItems() || m.verificationState.HasEntries() || m.hasTeamMembers()
}

func (m *Model) hasTeamMembers() bool {
	if session := m.runtime.Session; session != nil && session.Team != nil {
		return len(session.Team.Members()) > 1 // >1 because leader is always a member
	}
	return false
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
		// Truncate summary if it would overflow the content area.
		maxRight := contentWidth - lipgloss.Width(headerLeft) - 1
		if maxRight > 0 {
			headerRightText = ansi.Truncate(headerRightText, maxRight, "…")
		}
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
	showVerify := m.verificationState.HasEntries()
	showTeam := m.hasTeamMembers()
	bodyBudget := max(1, height-2)

	// Count how many sections we need to render.
	sections := 0
	if showPlan {
		sections++
	}
	if showInv {
		sections++
	}
	if showVerify {
		sections++
	}
	if showTeam {
		sections++
	}

	if sections == 0 {
		body = append(body, m.sty.Muted.Render("No workflow state yet."))
	} else {
		// Subtract separator lines between sections.
		if sections > 1 {
			bodyBudget = max(sections, height-2-(sections-1))
		}
		perSection := max(1, bodyBudget/sections)
		remainder := bodyBudget - perSection*sections

		if showTeam {
			budget := perSection
			if remainder > 0 {
				budget++
				remainder--
			}
			body = append(body, m.renderTeamPanelLines(budget, contentWidth)...)
		}
		if showPlan {
			if len(body) > 0 {
				body = append(body, sep)
			}
			budget := perSection
			if remainder > 0 {
				budget++
				remainder--
			}
			body = append(body, m.renderPlanPanelLines(budget, contentWidth)...)
		}
		if showInv {
			if len(body) > 0 {
				body = append(body, sep)
			}
			budget := perSection
			if remainder > 0 {
				budget++
				remainder--
			}
			body = append(body, m.renderInvariantPanelLines(budget, contentWidth)...)
		}
		if showVerify {
			if len(body) > 0 {
				body = append(body, sep)
			}
			budget := perSection
			if remainder > 0 {
				budget++
			}
			body = append(body, m.renderVerificationPanelLines(budget, contentWidth)...)
		}
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
	if m.hasTeamMembers() {
		members := m.runtime.Session.Team.Members()
		running := 0
		for _, mi := range members {
			if mi.State.String() == "running" {
				running++
			}
		}
		parts = append(parts, fmt.Sprintf("team %d/%d", running, len(members)))
	}
	if completed, total := m.planState.Progress(); total > 0 {
		parts = append(parts, fmt.Sprintf("plan %d/%d", completed, total))
	}
	if hardTotal, hardPass, hardFail, hardUnresolved, _, _, _ := m.invariantState.Counts(); hardTotal > 0 || m.invariantState.Extracted {
		parts = append(parts, fmt.Sprintf("inv %d✓ %d✗ %d?", hardPass, hardFail, hardUnresolved))
	}
	if total, pass, fail, stale, inProgress := m.verificationState.Counts(); total > 0 {
		parts = append(parts, fmt.Sprintf("verify %d✓ %d✗ %d◐ %d*", pass, fail, inProgress, stale))
	}
	return strings.Join(parts, " · ")
}

func (m *Model) renderTeamPanelLines(limit, width int) []string {
	if limit <= 0 || !m.hasTeamMembers() {
		return nil
	}
	members := m.runtime.Session.Team.Members()
	running, idle, stopped := 0, 0, 0
	for _, mi := range members {
		switch mi.State.String() {
		case "running":
			running++
		case "idle":
			idle++
		case "stopped":
			stopped++
		}
	}
	header := fmt.Sprintf("Team %d↑ %d○ %d×", running, idle, stopped)
	lines := []string{m.sty.Panel.Progress.Render(header)}
	if limit == 1 {
		return lines
	}
	itemBudget := limit - 1
	maxItems := min(itemBudget, len(members))
	for i := range maxItems {
		mi := members[i]
		icon := m.sty.Panel.IconPending.Render(styles.HollowIcon)
		switch mi.State.String() {
		case "running":
			icon = m.sty.Panel.IconInProgress.Render(styles.InProgressIcon)
		case "idle":
			icon = m.sty.Panel.IconCompleted.Render(styles.CheckIcon)
		case "stopped":
			icon = m.sty.Panel.IconBlocked.Render(styles.BlockedIcon)
		case "starting":
			icon = m.sty.Panel.IconPending.Render(styles.PendingIcon)
		}
		label := ansi.Truncate(mi.Name+" ("+mi.State.String()+")", max(1, width-4), "...")
		lines = append(lines, fmt.Sprintf(" %s %s", icon, m.sty.Panel.TaskText.Render(label)))
	}
	for len(lines) < limit {
		lines = append(lines, "")
	}
	return lines[:limit]
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
	for i := range maxItems {
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
	for i := range maxItems {
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

func (m *Model) renderVerificationPanelLines(limit, width int) []string {
	if limit <= 0 {
		return nil
	}
	_, pass, fail, stale, inProgress := m.verificationState.Counts()
	header := fmt.Sprintf("Verify %d✓ %d✗ %d◐ %d*", pass, fail, inProgress, stale)
	lines := []string{m.sty.Panel.Progress.Render(header)}
	if limit == 1 {
		return lines
	}
	itemBudget := limit - 1
	maxItems := min(itemBudget, len(m.verificationState.Entries))
	if len(m.verificationState.Entries) > itemBudget && itemBudget > 0 {
		maxItems = itemBudget - 1
	}
	for i := range maxItems {
		entry := m.verificationState.Entries[i]
		icon := m.sty.Panel.IconPending.Render(styles.HollowIcon)
		switch entry.Status {
		case "pass":
			icon = m.sty.Panel.IconCompleted.Render(styles.CheckIcon)
		case "fail":
			icon = m.sty.Panel.IconBlocked.Render(styles.BlockedIcon)
		case "in_progress":
			icon = m.sty.Panel.IconInProgress.Render(styles.InProgressIcon)
		}
		label := entry.Command
		if entry.Freshness == "stale" {
			label += " *"
		}
		label = ansi.Truncate(strings.TrimSpace(label), max(1, width-4), "...")
		if entry.Status == "pass" && entry.Freshness != "stale" {
			label = m.sty.Panel.TaskDone.Render(label)
		} else {
			label = m.sty.Panel.TaskText.Render(label)
		}
		lines = append(lines, fmt.Sprintf(" %s %s", icon, label))
	}
	remaining := len(m.verificationState.Entries) - maxItems
	if remaining > 0 && len(lines) < limit {
		lines = append(lines, m.sty.Muted.Render(fmt.Sprintf("... +%d verifications", remaining)))
	}
	for len(lines) < limit {
		lines = append(lines, "")
	}
	return lines[:limit]
}
