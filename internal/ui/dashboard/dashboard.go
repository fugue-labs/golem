package dashboard

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fugue-labs/golem/internal/mission"
	"github.com/fugue-labs/golem/internal/ui/styles"
)

const refreshInterval = 2 * time.Second

// pane identifies which pane has focus for scrolling.
type pane int

const (
	paneTasks pane = iota
	paneWorkers
	paneEvidence
	paneEvents
	paneCount
)

// Model is the Bubble Tea model for the mission dashboard.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc

	ctrl   *mission.Controller
	sty    *styles.Styles
	width  int
	height int

	// Active mission being displayed.
	missionID string

	// Cached state from last refresh.
	missionObj *mission.Mission
	summary    *mission.MissionSummary
	tasks      []*mission.Task
	deps       []mission.TaskDependency
	runs       []*mission.Run
	events     []*mission.Event
	approvals  []*mission.Approval

	// Focus and scrolling.
	focusPane  pane
	scrollPos  [paneCount]int
	lastErr    error
	quitting   bool
}

// New creates a dashboard model for the given mission ID.
// If missionID is empty, displays the most recent active mission.
func New(missionID string) *Model {
	sty := styles.New(nil)
	ctx, cancel := context.WithCancel(context.Background())
	return &Model{
		ctx:       ctx,
		cancel:    cancel,
		missionID: missionID,
		sty:       sty,
	}
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type refreshDoneMsg struct {
	err error
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.initStore(), tickCmd())
}

func (m *Model) initStore() tea.Cmd {
	return func() tea.Msg {
		if m.ctrl != nil {
			return m.doRefresh()
		}
		store, err := mission.OpenSQLiteStore(mission.ResolveSQLitePath())
		if err != nil {
			return refreshDoneMsg{err: fmt.Errorf("open mission store: %w", err)}
		}
		m.ctrl = mission.NewController(store)
		return m.doRefresh()
	}
}

func (m *Model) doRefresh() tea.Msg {
	if m.ctrl == nil {
		return refreshDoneMsg{err: fmt.Errorf("store not initialized")}
	}

	ctx := m.ctx

	// If no specific mission, find the most recent active one.
	if m.missionID == "" {
		missions, err := m.ctrl.Store().ListMissions(ctx)
		if err != nil {
			return refreshDoneMsg{err: err}
		}
		// Prefer running > paused > blocked > awaiting_approval > planning > draft.
		var best *mission.Mission
		for _, ms := range missions {
			if ms.Status.IsTerminal() {
				continue
			}
			if best == nil || missionPriority(ms.Status) > missionPriority(best.Status) {
				best = ms
			}
		}
		if best == nil && len(missions) > 0 {
			best = missions[0] // Fallback to most recent.
		}
		if best != nil {
			m.missionID = best.ID
		}
	}

	if m.missionID == "" {
		return refreshDoneMsg{err: fmt.Errorf("no missions found")}
	}

	ms, err := m.ctrl.GetMission(ctx, m.missionID)
	if err != nil {
		return refreshDoneMsg{err: err}
	}
	m.missionObj = ms

	summary, err := m.ctrl.GetMissionSummary(ctx, m.missionID)
	if err != nil {
		return refreshDoneMsg{err: err}
	}
	m.summary = summary

	tasks, err := m.ctrl.Store().ListTasks(ctx, m.missionID)
	if err != nil {
		return refreshDoneMsg{err: err}
	}
	m.tasks = tasks

	deps, err := m.ctrl.Store().ListDependencies(ctx, m.missionID)
	if err != nil {
		return refreshDoneMsg{err: err}
	}
	m.deps = deps

	runs, err := m.ctrl.Store().ListRuns(ctx, m.missionID)
	if err != nil {
		return refreshDoneMsg{err: err}
	}
	m.runs = runs

	events, err := m.ctrl.Store().ListEvents(ctx, m.missionID, 50)
	if err != nil {
		return refreshDoneMsg{err: err}
	}
	m.events = events

	approvals, err := m.ctrl.Store().ListApprovals(ctx, m.missionID)
	if err != nil {
		return refreshDoneMsg{err: err}
	}
	m.approvals = approvals

	return refreshDoneMsg{}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		return m, tea.Batch(
			func() tea.Msg { return m.doRefresh() },
			tickCmd(),
		)

	case refreshDoneMsg:
		m.lastErr = msg.err
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			m.cancel()
			if m.ctrl != nil {
				m.ctrl.Close()
			}
			return m, tea.Quit
		case "r":
			return m, func() tea.Msg { return m.doRefresh() }
		case "tab":
			m.focusPane = (m.focusPane + 1) % paneCount
			return m, nil
		case "shift+tab":
			m.focusPane = (m.focusPane - 1 + paneCount) % paneCount
			return m, nil
		case "j", "down":
			m.scrollPos[m.focusPane]++
			return m, nil
		case "k", "up":
			if m.scrollPos[m.focusPane] > 0 {
				m.scrollPos[m.focusPane]--
			}
			return m, nil
		case "1":
			m.focusPane = paneTasks
			return m, nil
		case "2":
			m.focusPane = paneWorkers
			return m, nil
		case "3":
			m.focusPane = paneEvidence
			return m, nil
		case "4":
			m.focusPane = paneEvents
			return m, nil
		}
	}

	return m, nil
}

func (m *Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	if m.width == 0 || m.height == 0 {
		return tea.NewView("Loading...")
	}

	if m.lastErr != nil && m.missionObj == nil {
		return tea.NewView(fmt.Sprintf("Dashboard error: %v\n\nPress q to quit.", m.lastErr))
	}

	var lines []string

	// === Mission Header (3 lines) ===
	headerLines := m.renderHeader()
	lines = append(lines, headerLines...)
	lines = append(lines, m.renderSeparator())

	// === Main body: left (tasks + evidence) | right (workers) ===
	bodyHeight := m.height - len(lines) - 3 // 3 = separator + events area (2 lines min) + footer
	if bodyHeight < 4 {
		bodyHeight = 4
	}

	leftWidth := m.width * 3 / 5
	rightWidth := m.width - leftWidth - 1 // -1 for border
	if rightWidth < 10 {
		rightWidth = 10
		leftWidth = m.width - rightWidth - 1
	}

	// Left pane: tasks (top 60%) + evidence (bottom 40%)
	taskHeight := bodyHeight * 3 / 5
	evidenceHeight := bodyHeight - taskHeight

	taskLines := m.renderTaskPane(taskHeight, leftWidth)
	evidenceLines := m.renderEvidencePane(evidenceHeight, leftWidth)
	leftLines := append(taskLines, evidenceLines...)

	// Right pane: workers
	workerLines := m.renderWorkerPane(bodyHeight, rightWidth)

	// Combine left and right panes side by side.
	borderChar := m.sty.Muted.Render("│")
	for i := range bodyHeight {
		left := ""
		if i < len(leftLines) {
			left = leftLines[i]
		}
		right := ""
		if i < len(workerLines) {
			right = workerLines[i]
		}
		left = padRight(left, leftWidth)
		right = padRight(right, rightWidth)
		lines = append(lines, left+borderChar+right)
	}

	// === Events row ===
	lines = append(lines, m.renderSeparator())
	eventLines := m.renderEventPane(2, m.width)
	lines = append(lines, eventLines...)

	// === Footer ===
	lines = append(lines, m.renderFooter())

	// Truncate/pad to terminal height.
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}

	return tea.NewView(strings.Join(lines, "\n"))
}

// renderHeader renders the 2-3 line mission header.
func (m *Model) renderHeader() []string {
	if m.missionObj == nil {
		return []string{m.sty.Panel.Title.Render("Mission Control — No active mission")}
	}

	ms := m.missionObj
	tc := m.summary.TaskCounts

	// Line 1: Title and status.
	statusIcon := missionStatusIcon(ms.Status)
	title := m.sty.Panel.Title.Render("Mission Control")
	status := fmt.Sprintf(" %s %s", statusIcon, ms.Status)
	line1 := title + m.sty.Muted.Render(status)

	// Line 2: Metrics bar.
	done := tc.Done + tc.Integrated + tc.Accepted
	var parts []string
	parts = append(parts, fmt.Sprintf("Tasks %d/%d", done, tc.Total))
	if tc.Running > 0 {
		parts = append(parts, fmt.Sprintf("Running %d", tc.Running))
	}
	if tc.Ready > 0 {
		parts = append(parts, fmt.Sprintf("Ready %d", tc.Ready))
	}
	if tc.Blocked > 0 {
		parts = append(parts, fmt.Sprintf("Blocked %d", tc.Blocked))
	}
	activeRuns := 0
	for _, r := range m.runs {
		if r.Status == mission.RunRunning || r.Status == mission.RunQueued {
			activeRuns++
		}
	}
	parts = append(parts, fmt.Sprintf("Workers %d", activeRuns))

	if ms.StartedAt != nil {
		elapsed := time.Since(*ms.StartedAt).Truncate(time.Second)
		parts = append(parts, fmt.Sprintf("Elapsed %s", elapsed))
	}

	line2 := m.sty.Panel.Progress.Render(strings.Join(parts, " │ "))

	// Line 3: Goal (truncated).
	goal := ansi.Truncate(ms.Goal, max(1, m.width-2), "…")
	line3 := m.sty.HalfMuted.Render(goal)

	return []string{line1, line2, line3}
}

// renderTaskPane renders the task DAG view.
func (m *Model) renderTaskPane(height, width int) []string {
	header := m.renderPaneHeader("Tasks", m.focusPane == paneTasks, width)
	lines := []string{header}
	budget := height - 1

	if len(m.tasks) == 0 {
		lines = append(lines, m.sty.Muted.Render(" No tasks yet"))
		for len(lines) < height {
			lines = append(lines, "")
		}
		return lines[:height]
	}

	// Build dependency map for display.
	depMap := make(map[string][]string) // taskID -> depends_on IDs
	for _, d := range m.deps {
		depMap[d.TaskID] = append(depMap[d.TaskID], d.DependsOnID)
	}

	// Group tasks by status for DAG-like view.
	groups := []struct {
		label  string
		status []mission.TaskStatus
	}{
		{"Running", []mission.TaskStatus{mission.TaskRunning, mission.TaskLeased}},
		{"Review", []mission.TaskStatus{mission.TaskAwaitingReview}},
		{"Ready", []mission.TaskStatus{mission.TaskReady}},
		{"Blocked", []mission.TaskStatus{mission.TaskBlocked, mission.TaskFailed, mission.TaskRejected}},
		{"Done", []mission.TaskStatus{mission.TaskDone, mission.TaskIntegrated, mission.TaskAccepted}},
		{"Pending", []mission.TaskStatus{mission.TaskPending}},
	}

	offset := m.scrollPos[paneTasks]
	var allItems []string
	for _, g := range groups {
		var groupTasks []*mission.Task
		for _, t := range m.tasks {
			for _, s := range g.status {
				if t.Status == s {
					groupTasks = append(groupTasks, t)
					break
				}
			}
		}
		if len(groupTasks) == 0 {
			continue
		}
		allItems = append(allItems, m.sty.Panel.Progress.Render(fmt.Sprintf(" ── %s (%d) ──", g.label, len(groupTasks))))
		for _, t := range groupTasks {
			icon := taskIcon(t.Status, m.sty)
			title := ansi.Truncate(t.Title, max(1, width-8), "…")
			if t.Status == mission.TaskDone || t.Status == mission.TaskIntegrated || t.Status == mission.TaskAccepted {
				title = m.sty.Panel.TaskDone.Render(title)
			} else {
				title = m.sty.Panel.TaskText.Render(title)
			}
			allItems = append(allItems, fmt.Sprintf(" %s %s", icon, title))
			if depsOn := depMap[t.ID]; len(depsOn) > 0 {
				depStr := ansi.Truncate("needs: "+strings.Join(depsOn, ", "), max(1, width-6), "…")
				allItems = append(allItems, m.sty.Muted.Render("   └─ "+depStr))
			}
		}
	}

	// Apply scroll offset.
	if offset > len(allItems) {
		offset = len(allItems)
		m.scrollPos[paneTasks] = offset
	}
	visible := allItems[offset:]
	if len(visible) > budget {
		visible = visible[:budget]
	}
	lines = append(lines, visible...)

	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines[:height]
}

// renderWorkerPane renders the worker lane cards.
func (m *Model) renderWorkerPane(height, width int) []string {
	header := m.renderPaneHeader("Workers", m.focusPane == paneWorkers, width)
	lines := []string{header}
	budget := height - 1

	// Collect active runs (running or queued).
	var activeRuns []*mission.Run
	for _, r := range m.runs {
		if r.Status == mission.RunRunning || r.Status == mission.RunQueued {
			activeRuns = append(activeRuns, r)
		}
	}

	if len(activeRuns) == 0 {
		lines = append(lines, m.sty.Muted.Render(" No active workers"))
		for len(lines) < height {
			lines = append(lines, "")
		}
		return lines[:height]
	}

	// Build task lookup.
	taskMap := make(map[string]*mission.Task)
	for _, t := range m.tasks {
		taskMap[t.ID] = t
	}

	offset := m.scrollPos[paneWorkers]
	var allItems []string

	for _, r := range activeRuns {
		// Worker card header.
		modeIcon := runModeIcon(r.Mode)
		statusColor := runStatusStyle(r.Status, m.sty)
		cardHeader := fmt.Sprintf(" %s %s [%s]", modeIcon, r.ID[:min(12, len(r.ID))], statusColor)
		allItems = append(allItems, m.sty.Panel.TaskText.Render(ansi.Truncate(cardHeader, max(1, width-2), "…")))

		// Task info.
		if t, ok := taskMap[r.TaskID]; ok {
			taskLine := fmt.Sprintf("   task: %s", ansi.Truncate(t.Title, max(1, width-10), "…"))
			allItems = append(allItems, m.sty.HalfMuted.Render(taskLine))
		}

		// Worktree.
		if r.WorktreePath != "" {
			wt := filepath.Base(r.WorktreePath)
			allItems = append(allItems, m.sty.Muted.Render(fmt.Sprintf("   wt: %s", wt)))
		}

		// Heartbeat.
		if r.HeartbeatAt != nil {
			ago := time.Since(*r.HeartbeatAt).Truncate(time.Second)
			allItems = append(allItems, m.sty.Muted.Render(fmt.Sprintf("   heartbeat: %s ago", ago)))
		}

		allItems = append(allItems, "") // Spacer between cards.
	}

	// Also show recently completed runs.
	var recentDone []*mission.Run
	for _, r := range m.runs {
		if r.Status == mission.RunSucceeded || r.Status == mission.RunFailed {
			if r.EndedAt != nil && time.Since(*r.EndedAt) < 5*time.Minute {
				recentDone = append(recentDone, r)
			}
		}
	}
	if len(recentDone) > 0 {
		allItems = append(allItems, m.sty.Panel.Progress.Render(" ── Recent ──"))
		for _, r := range recentDone {
			icon := "✓"
			if r.Status == mission.RunFailed {
				icon = "✗"
			}
			line := fmt.Sprintf(" %s %s [%s]", icon, r.ID[:min(12, len(r.ID))], r.Status)
			allItems = append(allItems, m.sty.Muted.Render(ansi.Truncate(line, max(1, width-2), "…")))
		}
	}

	if offset > len(allItems) {
		offset = len(allItems)
		m.scrollPos[paneWorkers] = offset
	}
	visible := allItems[offset:]
	if len(visible) > budget {
		visible = visible[:budget]
	}
	lines = append(lines, visible...)

	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines[:height]
}

// renderEvidencePane renders review decisions, verification, and approvals.
func (m *Model) renderEvidencePane(height, width int) []string {
	sep := m.sty.Panel.Separator.Render(strings.Repeat(styles.Separator, width))
	header := m.renderPaneHeader("Evidence", m.focusPane == paneEvidence, width)
	lines := []string{sep, header}
	budget := height - 2

	offset := m.scrollPos[paneEvidence]
	var allItems []string

	// Review results from completed review runs.
	for _, r := range m.runs {
		if r.Mode != mission.RunModeReview {
			continue
		}
		if r.Status == mission.RunSucceeded || r.Status == mission.RunFailed {
			icon := "✓"
			label := "pass"
			if r.Status == mission.RunFailed {
				icon = "✗"
				label = "fail"
			}
			summary := r.Summary
			if summary == "" {
				summary = r.TaskID
			}
			line := fmt.Sprintf(" %s Review %s: %s", icon, label, ansi.Truncate(summary, max(1, width-20), "…"))
			allItems = append(allItems, m.sty.Panel.TaskText.Render(line))
		}
	}

	// Pending approvals.
	for _, a := range m.approvals {
		if a.Status != mission.ApprovalPending {
			continue
		}
		line := fmt.Sprintf(" ⏳ Approval: %s [pending]", a.Kind)
		allItems = append(allItems, m.sty.Panel.TaskText.Render(ansi.Truncate(line, max(1, width-2), "…")))
	}

	// Failed runs as evidence.
	for _, r := range m.runs {
		if r.Status != mission.RunFailed || r.Mode == mission.RunModeReview {
			continue
		}
		errText := r.ErrorText
		if errText == "" {
			errText = "unknown error"
		}
		errText = ansi.Truncate(errText, max(1, width-20), "…")
		line := fmt.Sprintf(" ✗ %s %s: %s", r.Mode, r.TaskID, errText)
		allItems = append(allItems, m.sty.Panel.TaskText.Render(ansi.Truncate(line, max(1, width-2), "…")))
	}

	if len(allItems) == 0 {
		allItems = append(allItems, m.sty.Muted.Render(" No evidence yet"))
	}

	if offset > len(allItems) {
		offset = len(allItems)
		m.scrollPos[paneEvidence] = offset
	}
	visible := allItems[offset:]
	if len(visible) > budget {
		visible = visible[:budget]
	}
	lines = append(lines, visible...)

	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines[:height]
}

// renderEventPane renders recent events in a compact row.
func (m *Model) renderEventPane(height, width int) []string {
	header := m.renderPaneHeader("Events", m.focusPane == paneEvents, width)
	lines := []string{header}
	budget := height - 1

	offset := m.scrollPos[paneEvents]
	var allItems []string

	// Show events newest first.
	for i := len(m.events) - 1; i >= 0; i-- {
		e := m.events[i]
		ts := e.CreatedAt.Format("15:04:05")
		detail := e.Type
		if e.TaskID != "" {
			detail += " " + e.TaskID
		}
		line := fmt.Sprintf(" %s %s", ts, detail)
		allItems = append(allItems, m.sty.Muted.Render(ansi.Truncate(line, max(1, width-2), "…")))
	}

	if len(allItems) == 0 {
		allItems = append(allItems, m.sty.Muted.Render(" No events"))
	}

	if offset > len(allItems) {
		offset = len(allItems)
		m.scrollPos[paneEvents] = offset
	}
	visible := allItems[offset:]
	if len(visible) > budget {
		visible = visible[:budget]
	}
	lines = append(lines, visible...)

	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines[:height]
}

// renderPaneHeader renders a pane title with focus indicator.
func (m *Model) renderPaneHeader(title string, focused bool, width int) string {
	style := m.sty.Panel.Progress
	if focused {
		style = m.sty.Panel.Title
	}
	indicator := " "
	if focused {
		indicator = "▸"
	}
	return style.Render(indicator + " " + title)
}

func (m *Model) renderSeparator() string {
	return m.sty.Panel.Separator.Render(strings.Repeat(styles.Separator, m.width))
}

func (m *Model) renderFooter() string {
	keys := "q:quit  r:refresh  tab:switch pane  j/k:scroll  1-4:jump to pane"
	return m.sty.Muted.Render(ansi.Truncate(keys, max(1, m.width), "…"))
}

// Helper functions.

func missionStatusIcon(s mission.MissionStatus) string {
	switch s {
	case mission.MissionCompleted:
		return styles.CheckIcon
	case mission.MissionRunning:
		return styles.InProgressIcon
	case mission.MissionPaused:
		return "⏸"
	case mission.MissionBlocked:
		return styles.BlockedIcon
	case mission.MissionFailed, mission.MissionCancelled:
		return styles.ErrorIcon
	case mission.MissionAwaitingApproval:
		return "⏳"
	case mission.MissionPlanning:
		return styles.SpinnerIcon
	default:
		return styles.HollowIcon
	}
}

func missionPriority(s mission.MissionStatus) int {
	switch s {
	case mission.MissionRunning:
		return 6
	case mission.MissionBlocked:
		return 5
	case mission.MissionPaused:
		return 4
	case mission.MissionAwaitingApproval:
		return 3
	case mission.MissionPlanning:
		return 2
	case mission.MissionDraft:
		return 1
	default:
		return 0
	}
}

func taskIcon(s mission.TaskStatus, sty *styles.Styles) string {
	switch s {
	case mission.TaskDone, mission.TaskIntegrated, mission.TaskAccepted:
		return sty.Panel.IconCompleted.Render(styles.CheckIcon)
	case mission.TaskRunning, mission.TaskLeased:
		return sty.Panel.IconInProgress.Render(styles.InProgressIcon)
	case mission.TaskBlocked, mission.TaskFailed, mission.TaskRejected:
		return sty.Panel.IconBlocked.Render(styles.BlockedIcon)
	case mission.TaskReady:
		return sty.Panel.IconInProgress.Render(styles.PendingIcon)
	case mission.TaskAwaitingReview:
		return sty.Panel.IconInProgress.Render("◎")
	default:
		return sty.Panel.IconPending.Render(styles.HollowIcon)
	}
}

func runModeIcon(mode mission.RunMode) string {
	switch mode {
	case mission.RunModePlanner:
		return "🗺"
	case mission.RunModeWorker:
		return "⚙"
	case mission.RunModeReview:
		return "◎"
	case mission.RunModeIntegration:
		return "⇄"
	default:
		return "?"
	}
}

func runStatusStyle(s mission.RunStatus, sty *styles.Styles) string {
	switch s {
	case mission.RunRunning:
		return lipgloss.NewStyle().Foreground(sty.Yellow).Render(string(s))
	case mission.RunSucceeded:
		return lipgloss.NewStyle().Foreground(sty.Green).Render(string(s))
	case mission.RunFailed:
		return lipgloss.NewStyle().Foreground(sty.Red).Render(string(s))
	default:
		return string(s)
	}
}

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return ansi.Truncate(s, width, "…")
	}
	return s + strings.Repeat(" ", width-w)
}
