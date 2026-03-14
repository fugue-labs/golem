package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fugue-labs/golem/internal/ui/chat"
	"github.com/fugue-labs/golem/internal/ui/styles"
	"github.com/fugue-labs/gollem/ext/codetool"
	"github.com/fugue-labs/gollem/ext/team"
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
	if m.appCancel != nil {
		m.appCancel()
	}
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
	if m.orchestrator != nil {
		m.orchestrator.Stop()
		m.orchestrator = nil
	}
	if m.missionCtrl != nil {
		m.missionCtrl.Close()
		m.missionCtrl = nil
	}
	if m.fileWatcher != nil {
		m.fileWatcher.Close()
		m.fileWatcher = nil
	}
}

func (m *Model) pendingCount() int {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	return len(m.pendingMsgs)
}

func (m *Model) hasWorkflowPanel() bool {
	return m.planState.HasTasks() || m.invariantState.HasItems() || m.verificationState.HasEntries() || m.hasTeamMembers() || m.hasMissionState() || m.specState.IsActive()
}

func (m *Model) hasTeamMembers() bool {
	if session := m.runtime.Session; session != nil && session.Team != nil {
		return len(activeTeamMembers(session.Team.Members())) > 1 // >1 because leader is always a member
	}
	return false
}

// activeTeamMembers filters out stopped teammates so that stale entries
// from previous runs don't clutter the display.
func activeTeamMembers(members []team.TeammateInfo) []team.TeammateInfo {
	result := make([]team.TeammateInfo, 0, len(members))
	for _, mi := range members {
		if mi.State != team.TeammateStopped {
			result = append(result, mi)
		}
	}
	return result
}

// purgeStaleTeam nils the team on the session when all non-leader
// teammates have stopped. This allows a fresh team to be created on
// the next run, preventing stopped members from blocking name reuse
// and accumulating across mission restarts or re-plans.
func (m *Model) purgeStaleTeam(sess *codetool.Session) {
	members := sess.Team.Members()
	if len(members) <= 1 {
		return // only leader or empty — nothing to purge
	}
	for _, mi := range members {
		if mi.Name == "leader" {
			continue
		}
		if mi.State != team.TeammateStopped {
			return // at least one non-leader still active — keep team
		}
	}
	// All non-leader members are stopped — reset for fresh start.
	sess.Team = nil
}

func (m *Model) renderWorkflowPanel(height, width int) string {
	if height < 1 {
		height = 1
	}
	contentWidth := max(1, width-2)
	borderStr := lipgloss.NewStyle().Foreground(m.sty.BgSubtle).Render(styles.BorderThin) + " "
	sep := m.sty.Panel.Separator.Render(strings.Repeat(styles.Separator, contentWidth))

	headerLeft := m.sty.Panel.Title.Render("Workflow")
	headerRightText := workflowPanelSummaryWidth(m, max(1, contentWidth-lipgloss.Width(headerLeft)-1))
	headerRight := ""
	if headerRightText != "" {
		headerRight = m.sty.Panel.Progress.Render(headerRightText)
	}
	titleGap := contentWidth - lipgloss.Width(headerLeft) - lipgloss.Width(headerRight)
	if titleGap < 1 {
		titleGap = 1
	}
	titleLine := headerLeft + strings.Repeat(" ", titleGap) + headerRight

	sections := []func(int, int) []string{}
	if m.hasMissionState() {
		sections = append(sections, m.renderMissionPanelLines)
	}
	if m.specState.IsActive() {
		sections = append(sections, m.renderSpecPanelLines)
	}
	if m.planState.HasTasks() {
		sections = append(sections, m.renderPlanPanelLines)
	}
	if m.invariantState.HasItems() {
		sections = append(sections, m.renderInvariantPanelLines)
	}
	if m.verificationState.HasEntries() {
		sections = append(sections, m.renderVerificationPanelLines)
	}
	if m.hasTeamMembers() {
		sections = append(sections, m.renderTeamPanelLines)
	}

	var body []string
	if len(sections) == 0 {
		body = append(body, m.sty.Muted.Render("No workflow state yet."))
	} else {
		bodyBudget := max(1, height-2)
		separatorCount := max(0, len(sections)-1)
		lineBudget := max(len(sections), bodyBudget-separatorCount)
		perSection := max(1, lineBudget/len(sections))
		extra := lineBudget - perSection*len(sections)
		for i, render := range sections {
			if i > 0 {
				body = append(body, sep)
			}
			budget := perSection
			if extra > 0 {
				budget++
				extra--
			}
			body = append(body, render(budget, contentWidth)...)
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

func workflowPanelSummaryWidth(m *Model, width int) string {
	if width <= 0 {
		return ""
	}
	compact := width < 44
	var parts []string
	if missionSummary := m.missionPanelSummaryWidth(summaryWidth(width, compact, 12, 28)); missionSummary != "" {
		parts = append(parts, "mission "+missionSummary)
	}
	if m.specState.IsActive() {
		summary := m.specState.PanelSummary(summaryWidth(width, compact, 16, 28))
		if compact {
			summary = m.specState.GateSummary()
		}
		parts = append(parts, "spec "+summary)
	}
	if m.planState.HasTasks() {
		parts = append(parts, "plan "+m.planState.Summary(summaryWidth(width, compact, 12, 28)))
	}
	if m.invariantState.HasItems() {
		if len(m.invariantState.Items) == 0 {
			parts = append(parts, "Inv 0✓ 0✗ 0?")
		} else {
			parts = append(parts, "Inv "+m.invariantState.Summary(summaryWidth(width, compact, 16, 28)))
		}
	}
	if m.verificationState.HasEntries() {
		parts = append(parts, "verify "+m.verificationState.Summary(summaryWidth(width, compact, 14, 28)))
	}
	if m.hasTeamMembers() {
		members := activeTeamMembers(m.runtime.Session.Team.Members())
		running := 0
		for _, mi := range members {
			if mi.State.String() == "running" {
				running++
			}
		}
		parts = append(parts, fmt.Sprintf("team %d/%d", running, len(members)))
	}
	return ansi.Truncate(strings.Join(parts, " · "), width, "…")
}

func summaryWidth(total int, compact bool, narrow, wide int) int {
	if compact {
		return narrow
	}
	return min(wide, max(narrow, total))
}

func (m *Model) panelSectionTitle(title string) string {
	switch title {
	case "Mission":
		return "◎ " + title
	case "Spec":
		return styles.ModelIcon + " " + title
	case "Plan":
		return styles.ArrowRight + " " + title
	case "Invariants":
		return "◫ " + title
	case "Verification":
		return styles.CheckIcon + " " + title
	case "Team":
		return "◌ " + title
	default:
		return title
	}
}

func (m *Model) renderPanelSectionHeader(title, summary string, width int) string {
	left := m.sty.Panel.Title.Render(title)
	if summary == "" {
		return ansi.Truncate(left, max(1, width), "…")
	}
	maxRight := width - lipgloss.Width(left) - 1
	if maxRight <= 0 {
		return ansi.Truncate(left, max(1, width), "…")
	}
	right := m.sty.Panel.Progress.Render(ansi.Truncate(summary, maxRight, "…"))
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) renderPanelDetail(label, value string, width int) string {
	prefix := m.sty.Muted.Render(label + " ")
	available := max(1, width-lipgloss.Width(prefix))
	return prefix + m.sty.Panel.TaskText.Render(ansi.Truncate(strings.TrimSpace(value), available, "…"))
}

func (m *Model) renderPanelOverflow(label string, remaining int) string {
	return m.sty.Muted.Render(fmt.Sprintf("… +%d more %s", remaining, label))
}

func (m *Model) renderTeamPanelLines(limit, width int) []string {
	if limit <= 0 || !m.hasTeamMembers() {
		return nil
	}
	members := activeTeamMembers(m.runtime.Session.Team.Members())
	running, idle, starting := 0, 0, 0
	for _, mi := range members {
		switch mi.State.String() {
		case "running":
			running++
		case "idle":
			idle++
		case "starting":
			starting++
		}
	}
	summary := fmt.Sprintf("%d running · %d idle", running, idle)
	if starting > 0 {
		summary += fmt.Sprintf(" · %d starting", starting)
	}
	lines := []string{m.renderPanelSectionHeader(m.panelSectionTitle("Team"), summary, width)}
	if limit == 1 {
		return lines
	}
	itemBudget := limit - 1
	maxItems := min(itemBudget, len(members))
	if len(members) > itemBudget && itemBudget > 0 {
		maxItems = itemBudget - 1
	}
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
	remaining := len(members) - maxItems
	if remaining > 0 && len(lines) < limit {
		lines = append(lines, m.renderPanelOverflow("members", remaining))
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
	lines := []string{m.renderPanelSectionHeader(m.panelSectionTitle("Plan"), m.planState.Summary(max(12, width-12)), width)}
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
		lines = append(lines, m.renderPanelOverflow("plan tasks", remaining))
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
	lines := []string{m.renderPanelSectionHeader(m.panelSectionTitle("Invariants"), m.invariantState.Summary(max(16, width-16)), width)}
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
		lines = append(lines, m.renderPanelOverflow("invariants", remaining))
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
	lines := []string{m.renderPanelSectionHeader(m.panelSectionTitle("Verification"), m.verificationState.Summary(max(14, width-14)), width)}
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
		lines = append(lines, m.renderPanelOverflow("checks", remaining))
	}
	for len(lines) < limit {
		lines = append(lines, "")
	}
	return lines[:limit]
}

func (m *Model) renderSpecPanelLines(limit, width int) []string {
	if limit <= 0 || !m.specState.IsActive() {
		return nil
	}
	lines := []string{m.renderPanelSectionHeader(m.panelSectionTitle("Spec"), m.specState.PanelSummary(max(12, width-12)), width)}
	if limit == 1 {
		return lines
	}
	if limit > 2 {
		lines = append(lines, m.renderPanelDetail("file", m.specState.FileLabel(max(12, width-10)), width))
	}
	itemBudget := limit - len(lines)
	maxItems := min(itemBudget, len(m.specState.Gates))
	if len(m.specState.Gates) > itemBudget && itemBudget > 0 {
		maxItems = itemBudget - 1
	}
	for i := range maxItems {
		g := m.specState.Gates[i]
		icon := m.sty.Panel.IconPending.Render(styles.HollowIcon)
		if g.Status == "passed" {
			icon = m.sty.Panel.IconCompleted.Render(styles.CheckIcon)
		}
		label := ansi.Truncate(g.Name, max(1, width-4), "...")
		if g.Status == "passed" {
			label = m.sty.Panel.TaskDone.Render(label)
		} else {
			label = m.sty.Panel.TaskText.Render(label)
		}
		lines = append(lines, fmt.Sprintf(" %s %s", icon, label))
	}
	remaining := len(m.specState.Gates) - maxItems
	if remaining > 0 && len(lines) < limit {
		lines = append(lines, m.renderPanelOverflow("gates", remaining))
	}
	for len(lines) < limit {
		lines = append(lines, "")
	}
	return lines[:limit]
}
