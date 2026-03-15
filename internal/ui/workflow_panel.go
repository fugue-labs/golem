package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fugue-labs/golem/internal/ui/chat"
	"github.com/fugue-labs/golem/internal/ui/plan"
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

	type sectionSpec struct {
		priority int
		target   int
		render   func(limit int) []string
	}

	sections := make([]sectionSpec, 0, 6)
	if m.hasMissionState() {
		priority := 0
		if summary := m.missionPanelSummary(); strings.Contains(summary, "blocked") || strings.Contains(summary, "approval") {
			priority = -2
		}
		sections = append(sections, sectionSpec{priority: priority, target: 6, render: func(limit int) []string {
			return m.renderMissionPanelLines(limit, contentWidth)
		}})
	}
	if m.specState.IsActive() {
		priority := 1
		if m.specState.WaitingGateName() != "" {
			priority = -1
		}
		sections = append(sections, sectionSpec{priority: priority, target: 5, render: func(limit int) []string {
			return m.renderSpecPanelLines(limit, contentWidth)
		}})
	}
	if m.planState.HasTasks() {
		priority := 2
		if focus := m.planState.Focus(); focus != nil {
			switch focus.Status {
			case "blocked":
				priority = -1
			case "in_progress":
				priority = 0
			}
		}
		sections = append(sections, sectionSpec{priority: priority, target: 5, render: func(limit int) []string {
			return m.renderPlanPanelLines(limit, contentWidth)
		}})
	}
	if m.verificationState.HasEntries() {
		priority := 3
		if focus := m.verificationState.Focus(); focus != nil {
			switch {
			case focus.Status == "fail":
				priority = -1
			case focus.Status == "in_progress", focus.Freshness == "stale":
				priority = 1
			}
		}
		sections = append(sections, sectionSpec{priority: priority, target: 5, render: func(limit int) []string {
			return m.renderVerificationPanelLines(limit, contentWidth)
		}})
	}
	if m.invariantState.HasItems() {
		priority := 4
		if focus := m.invariantState.Focus(); focus != nil {
			switch focus.Status {
			case "fail":
				priority = 0
			case "in_progress":
				priority = 2
			}
		}
		sections = append(sections, sectionSpec{priority: priority, target: 4, render: func(limit int) []string {
			return m.renderInvariantPanelLines(limit, contentWidth)
		}})
	}
	if m.hasTeamMembers() {
		sections = append(sections, sectionSpec{priority: 5, target: 3, render: func(limit int) []string {
			return m.renderTeamPanelLines(limit, contentWidth)
		}})
	}
	if len(sections) > 1 {
		slices.SortStableFunc(sections, func(a, b sectionSpec) int {
			return cmp.Compare(a.priority, b.priority)
		})
	}

	body := make([]string, 0, max(1, height-2))
	if len(sections) == 0 {
		body = append(body, m.sty.Muted.Render("No workflow state yet."))
	} else {
		usableLines := max(1, height-2)
		if len(sections) > 1 {
			usableLines = max(len(sections), usableLines-(len(sections)-1))
		}
		budgets := make([]int, len(sections))
		for i := range budgets {
			budgets[i] = 1
		}
		extra := usableLines - len(sections)
		for extra > 0 {
			progressed := false
			for i, sec := range sections {
				if extra == 0 {
					break
				}
				if budgets[i] >= sec.target {
					continue
				}
				budgets[i]++
				extra--
				progressed = true
			}
			if progressed {
				continue
			}
			for i := range sections {
				if extra == 0 {
					break
				}
				budgets[i]++
				extra--
			}
		}

		for i, sec := range sections {
			if i > 0 {
				body = append(body, sep)
			}
			body = append(body, sec.render(budgets[i])...)
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
		parts = append(parts, "spec "+strings.ToLower(m.specState.Headline()))
	}
	if focus := m.planState.Focus(); focus != nil {
		switch focus.Status {
		case "blocked":
			parts = append(parts, "plan blocked")
		case "in_progress":
			parts = append(parts, "plan active")
		default:
			completed, total := m.planState.Progress()
			parts = append(parts, fmt.Sprintf("plan %d/%d", completed, total))
		}
	}
	if focus := m.verificationState.Focus(); focus != nil {
		switch {
		case focus.Status == "fail":
			parts = append(parts, "verify failed")
		case focus.Status == "in_progress":
			parts = append(parts, "verify running")
		case focus.Freshness == "stale":
			parts = append(parts, "verify stale")
		default:
			parts = append(parts, "verify ok")
		}
	}
	if focus := m.invariantState.Focus(); focus != nil {
		switch focus.Status {
		case "fail":
			parts = append(parts, "inv failing")
		case "in_progress":
			parts = append(parts, "inv checking")
		case "unknown":
			parts = append(parts, "inv open")
		default:
			parts = append(parts, "inv pass")
		}
	} else if m.invariantState.Extracted {
		parts = append(parts, "inv pending")
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
	return strings.Join(parts, " · ")
}

func (m *Model) renderTeamPanelLines(limit, width int) []string {
	if limit <= 0 || !m.hasTeamMembers() {
		return nil
	}
	members := activeTeamMembers(m.runtime.Session.Team.Members())
	running, idle := 0, 0
	for _, mi := range members {
		switch mi.State.String() {
		case "running":
			running++
		case "idle":
			idle++
		}
	}
	lines := []string{m.workflowProgressLine(fmt.Sprintf("Team %d active · %d ready", running, idle), width)}
	if limit == 1 {
		return lines
	}
	itemBudget := limit - 1
	maxItems := min(itemBudget, len(members))
	for i := 0; i < maxItems; i++ {
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
		label := mi.Name + " (" + mi.State.String() + ")"
		lines = append(lines, m.workflowBullet(icon, label, width, false))
	}
	for len(lines) < limit {
		lines = append(lines, "")
	}
	return lines[:limit]
}

func (m *Model) renderPlanPanelLines(limit, width int) []string {
	if limit <= 0 || !m.planState.HasTasks() {
		return nil
	}
	completed, total := m.planState.Progress()
	_, inProgress, blocked, pending := m.planState.Counts()
	focus := m.planState.Focus()
	header := fmt.Sprintf("Plan %d/%d complete", completed, total)
	switch {
	case focus == nil:
	case focus.Status == "blocked":
		header = fmt.Sprintf("Plan %s blocked", styles.BlockedIcon)
	case focus.Status == "in_progress":
		header = fmt.Sprintf("Plan %s active", styles.InProgressIcon)
	case completed == total && total > 0:
		header = fmt.Sprintf("Plan %s complete", styles.CheckIcon)
	default:
		header = fmt.Sprintf("Plan %s next up", styles.PendingIcon)
	}
	lines := []string{m.workflowProgressLine(header, width)}
	if limit == 1 {
		return lines
	}
	if focus != nil {
		lines = append(lines, m.renderPlanSpotlight(focus, width))
	}
	if len(lines) < limit {
		if next := m.planState.Next(); next != nil && (focus == nil || next.ID != focus.ID) {
			lines = append(lines, m.workflowBullet(m.sty.Panel.IconPending.Render(styles.PendingIcon), "Next: "+next.Description, width, false))
		}
	}
	if len(lines) < limit {
		counts := fmt.Sprintf("Done %d/%d · Active %d · Blocked %d · Pending %d", completed, total, inProgress, blocked, pending)
		lines = append(lines, m.workflowProgressLine(counts, width))
	}
	for i := range m.planState.Tasks {
		if len(lines) >= limit {
			break
		}
		task := m.planState.Tasks[i]
		if focus != nil && task.ID == focus.ID {
			continue
		}
		if next := m.planState.Next(); next != nil && task.ID == next.ID {
			continue
		}
		icon := m.sty.Panel.IconPending.Render(styles.HollowIcon)
		done := false
		switch task.Status {
		case "completed":
			icon = m.sty.Panel.IconCompleted.Render(styles.CheckIcon)
			done = true
		case "in_progress":
			icon = m.sty.Panel.IconInProgress.Render(styles.InProgressIcon)
		case "blocked":
			icon = m.sty.Panel.IconBlocked.Render(styles.BlockedIcon)
		}
		lines = append(lines, m.workflowBullet(icon, task.Description, width, done))
	}
	if len(m.planState.Tasks) > 0 && len(lines) < limit {
		shown := len(lines) - 1
		if shown < len(m.planState.Tasks) {
			remaining := len(m.planState.Tasks) - shown
			if remaining > 0 {
				lines = append(lines, m.sty.Muted.Render(ansi.Truncate(fmt.Sprintf("… +%d plan tasks", remaining), max(1, width), "…")))
			}
		}
	}
	for len(lines) < limit {
		lines = append(lines, "")
	}
	return lines[:limit]
}

func (m *Model) renderInvariantPanelLines(limit, width int) []string {
	if limit <= 0 || !m.invariantState.HasItems() {
		return nil
	}
	hardTotal, hardPass, hardFail, hardUnresolved, _, _, _ := m.invariantState.Counts()
	focus := m.invariantState.Focus()
	header := fmt.Sprintf("Inv %d✓ %d✗ %d?", hardPass, hardFail, hardUnresolved)
	switch {
	case focus != nil && focus.Status == "fail":
		header = fmt.Sprintf("Inv %s failing", styles.BlockedIcon)
	case focus != nil && focus.Status == "in_progress":
		header = fmt.Sprintf("Inv %s checking", styles.InProgressIcon)
	case focus != nil && focus.Status == "unknown":
		header = "Inv unresolved"
	case hardTotal > 0 && hardUnresolved == 0 && hardFail == 0:
		header = fmt.Sprintf("Inv %s satisfied", styles.CheckIcon)
	}
	lines := []string{m.workflowProgressLine(header, width)}
	if limit == 1 {
		return lines
	}
	if focus != nil {
		prefix := "Open: "
		icon := m.sty.Panel.IconPending.Render(styles.HollowIcon)
		switch focus.Status {
		case "pass":
			prefix = "Pass: "
			icon = m.sty.Panel.IconCompleted.Render(styles.CheckIcon)
		case "fail":
			prefix = "Failing: "
			icon = m.sty.Panel.IconBlocked.Render(styles.BlockedIcon)
		case "in_progress":
			prefix = "Checking: "
			icon = m.sty.Panel.IconInProgress.Render(styles.InProgressIcon)
		}
		lines = append(lines, m.workflowBullet(icon, prefix+focus.Description, width, focus.Status == "pass"))
	}
	if len(lines) < limit {
		if next := m.invariantState.Next(); next != nil && (focus == nil || next.ID != focus.ID) {
			lines = append(lines, m.workflowBullet(m.sty.Panel.IconPending.Render(styles.PendingIcon), "Next: "+next.Description, width, false))
		}
	}
	if len(lines) < limit {
		lines = append(lines, m.workflowProgressLine(fmt.Sprintf("Hard %d/%d pass · %d fail", hardPass, max(1, hardTotal), hardFail), width))
	}
	for i := range m.invariantState.Items {
		if len(lines) >= limit {
			break
		}
		item := m.invariantState.Items[i]
		if focus != nil && item.ID == focus.ID {
			continue
		}
		if next := m.invariantState.Next(); next != nil && item.ID == next.ID {
			continue
		}
		icon := m.sty.Panel.IconPending.Render(styles.HollowIcon)
		done := false
		switch item.Status {
		case "pass":
			icon = m.sty.Panel.IconCompleted.Render(styles.CheckIcon)
			done = true
		case "fail":
			icon = m.sty.Panel.IconBlocked.Render(styles.BlockedIcon)
		case "in_progress":
			icon = m.sty.Panel.IconInProgress.Render(styles.InProgressIcon)
		}
		kind := "H"
		if strings.TrimSpace(item.Kind) != "" {
			kind = strings.ToUpper(strings.TrimSpace(item.Kind))[:1]
		}
		label := strings.TrimSpace(item.ID + " " + kind + " " + item.Description)
		lines = append(lines, m.workflowBullet(icon, label, width, done))
	}
	for len(lines) < limit {
		lines = append(lines, "")
	}
	return lines[:limit]
}

func (m *Model) renderVerificationPanelLines(limit, width int) []string {
	if limit <= 0 || !m.verificationState.HasEntries() {
		return nil
	}
	_, pass, fail, stale, inProgress := m.verificationState.Counts()
	focus := m.verificationState.Focus()
	header := fmt.Sprintf("Verify %d✓ %d✗ %d◐ %d*", pass, fail, inProgress, stale)
	switch {
	case focus != nil && focus.Status == "fail":
		header = fmt.Sprintf("Verify %s failed", styles.BlockedIcon)
	case focus != nil && focus.Status == "in_progress":
		header = fmt.Sprintf("Verify %s running", styles.InProgressIcon)
	case focus != nil && focus.Freshness == "stale":
		header = "Verify stale"
	case fail == 0 && stale == 0 && inProgress == 0 && pass > 0:
		header = fmt.Sprintf("Verify %s clean", styles.CheckIcon)
	}
	lines := []string{m.workflowProgressLine(header, width)}
	if limit == 1 {
		return lines
	}
	if focus != nil {
		prefix := "Latest: "
		icon := m.sty.Panel.IconPending.Render(styles.HollowIcon)
		if focus.Status == "fail" {
			prefix = "Failing: "
			icon = m.sty.Panel.IconBlocked.Render(styles.BlockedIcon)
		} else if focus.Status == "in_progress" {
			prefix = "Running: "
			icon = m.sty.Panel.IconInProgress.Render(styles.InProgressIcon)
		} else if focus.Freshness == "stale" {
			prefix = "Re-run: "
			icon = m.sty.Panel.IconPending.Render(styles.PendingIcon)
		} else if focus.Status == "pass" {
			prefix = "Passed: "
			icon = m.sty.Panel.IconCompleted.Render(styles.CheckIcon)
		}
		lines = append(lines, m.workflowBullet(icon, prefix+focus.Command, width, focus.Status == "pass" && focus.Freshness != "stale"))
	}
	if len(lines) < limit {
		if next := m.verificationState.Next(); next != nil && (focus == nil || next.ID != focus.ID) {
			lines = append(lines, m.workflowBullet(m.sty.Panel.IconPending.Render(styles.PendingIcon), "Next: "+next.Command, width, false))
		}
	}
	if len(lines) < limit {
		counts := fmt.Sprintf("Pass %d · Fail %d · Running %d · Stale %d", pass, fail, inProgress, stale)
		lines = append(lines, m.workflowProgressLine(counts, width))
	}
	for i := range m.verificationState.Entries {
		if len(lines) >= limit {
			break
		}
		entry := m.verificationState.Entries[i]
		if focus != nil && entry.ID == focus.ID {
			continue
		}
		if next := m.verificationState.Next(); next != nil && entry.ID == next.ID {
			continue
		}
		icon := m.sty.Panel.IconPending.Render(styles.HollowIcon)
		done := false
		switch entry.Status {
		case "pass":
			icon = m.sty.Panel.IconCompleted.Render(styles.CheckIcon)
			done = entry.Freshness != "stale"
		case "fail":
			icon = m.sty.Panel.IconBlocked.Render(styles.BlockedIcon)
		case "in_progress":
			icon = m.sty.Panel.IconInProgress.Render(styles.InProgressIcon)
		}
		label := entry.Command
		if entry.Freshness == "stale" {
			label += " *"
		}
		lines = append(lines, m.workflowBullet(icon, label, width, done))
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
	header := "Spec — " + m.specState.Headline()
	lines := []string{m.workflowProgressLine(header, width)}
	if limit == 1 {
		return lines
	}
	lines = append(lines, m.workflowBullet(m.sty.Panel.IconPending.Render(styles.PendingIcon), m.specState.NextAction(), width, false))
	if len(lines) >= limit {
		return lines[:limit]
	}
	progress := m.specState.GateSummary()
	if completed, total := m.specState.Progress(); total > 0 {
		progress += fmt.Sprintf(" · Tasks %d/%d", completed, total)
	}
	lines = append(lines, m.workflowProgressLine(progress, width))
	if len(lines) >= limit {
		return lines[:limit]
	}
	if fileLine := strings.TrimSpace(m.specState.FileLabel()); fileLine != "" {
		lines = append(lines, m.workflowBullet(m.sty.Panel.IconPending.Render(styles.HollowIcon), fileLine, width, false))
	}
	visible := m.specState.VisibleGates()
	focus := m.specState.FocusGateName()
	for i := range visible {
		if len(lines) >= limit {
			break
		}
		g := visible[i]
		icon := m.sty.Panel.IconPending.Render(styles.HollowIcon)
		done := false
		label := g.Name
		if g.Status == "passed" {
			icon = m.sty.Panel.IconCompleted.Render(styles.CheckIcon)
			done = true
		} else if strings.EqualFold(focus, g.Name) {
			label = "Gate: " + label
			icon = m.sty.Panel.IconPending.Render(styles.PendingIcon)
		}
		lines = append(lines, m.workflowBullet(icon, label, width, done))
	}
	for len(lines) < limit {
		lines = append(lines, "")
	}
	return lines[:limit]
}

func (m *Model) renderPlanSpotlight(task *plan.Task, width int) string {
	icon := m.sty.Panel.IconPending.Render(styles.PendingIcon)
	prefix := "Next: "
	done := false
	switch task.Status {
	case "completed":
		icon = m.sty.Panel.IconCompleted.Render(styles.CheckIcon)
		prefix = "Done: "
		done = true
	case "in_progress":
		icon = m.sty.Panel.IconInProgress.Render(styles.InProgressIcon)
		prefix = "In progress: "
	case "blocked":
		icon = m.sty.Panel.IconBlocked.Render(styles.BlockedIcon)
		prefix = "Blocked: "
	}
	label := prefix + task.Description
	if task.Notes != "" && task.Status == "blocked" {
		label += " — " + task.Notes
	}
	return m.workflowBullet(icon, label, width, done)
}

func (m *Model) workflowProgressLine(text string, width int) string {
	return m.sty.Panel.Progress.Render(ansi.Truncate(strings.TrimSpace(text), max(1, width), "…"))
}

func (m *Model) workflowBullet(icon, label string, width int, done bool) string {
	label = ansi.Truncate(strings.TrimSpace(label), max(1, width-4), "…")
	if done {
		label = m.sty.Panel.TaskDone.Render(label)
	} else {
		label = m.sty.Panel.TaskText.Render(label)
	}
	return fmt.Sprintf(" %s %s", icon, label)
}
