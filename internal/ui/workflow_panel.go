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
	showMission := m.hasMissionState()
	showSpec := m.specState.IsActive()
	showPlan := m.planState.HasTasks()
	showInv := m.invariantState.HasItems()
	showVerify := m.verificationState.HasEntries()
	showTeam := m.hasTeamMembers()
	bodyBudget := max(1, height-2)

	// Count how many sections we need to render.
	sections := 0
	if showMission {
		sections++
	}
	if showSpec {
		sections++
	}
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

		if showMission {
			budget := perSection
			if remainder > 0 {
				budget++
				remainder--
			}
			body = append(body, m.renderMissionPanelLines(budget, contentWidth)...)
		}
		if showSpec {
			if len(body) > 0 {
				body = append(body, sep)
			}
			budget := perSection
			if remainder > 0 {
				budget++
				remainder--
			}
			body = append(body, m.renderSpecPanelLines(budget, contentWidth)...)
		}
		if showTeam {
			if len(body) > 0 {
				body = append(body, sep)
			}
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
	if missionSummary := m.missionPanelSummary(); missionSummary != "" {
		parts = append(parts, missionSummary)
	}
	if m.specState.IsActive() {
		parts = append(parts, fmt.Sprintf("spec %s · %s", m.specState.PhaseLabel(), m.specState.GateSummary()))
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
	if m.planState.HasTasks() {
		parts = append(parts, "plan "+m.planState.Summary(28))
	}
	if m.invariantState.HasItems() {
		parts = append(parts, "Inv "+m.invariantState.Summary(16))
	}
	if m.verificationState.HasEntries() {
		parts = append(parts, "verify "+m.verificationState.Summary(28))
	}
	return strings.Join(parts, " · ")
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
	return m.sty.Muted.Render(fmt.Sprintf("… +%d %s", remaining, label))
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
	lines := []string{m.renderPanelSectionHeader("Team", summary, width)}
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
	lines := []string{m.renderPanelSectionHeader("Plan", m.planState.Summary(width/2), width)}
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
	lines := []string{m.renderPanelSectionHeader("Invariants", m.invariantState.Summary(width/2), width)}
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
	lines := []string{m.renderPanelSectionHeader("Verification", m.verificationState.Summary(width/2), width)}
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
	lines := []string{m.renderPanelSectionHeader("Spec", m.specState.PanelSummary(max(12, width/2)), width)}
	if limit == 1 {
		return lines
	}
	if limit > 2 {
		lines = append(lines, m.renderPanelDetail("file", m.specState.FileLabel(max(12, width/2)), width))
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
