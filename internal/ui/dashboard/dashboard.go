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

const (
	refreshInterval        = 2 * time.Second
	compactDashboardWidth  = 88
	compactDashboardHeight = 22
)

// dashboardRenderMode tracks the operator-visible dashboard mode so hit-testing matches rendering.
type dashboardRenderMode int

const (
	dashboardRenderNone dashboardRenderMode = iota
	dashboardRenderNoMission
	dashboardRenderError
	dashboardRenderCompact
	dashboardRenderFull
)

type dashboardRect struct {
	x int
	y int
	w int
	h int
}

func (r dashboardRect) contains(x, y int) bool {
	return x >= r.x && y >= r.y && x < r.x+r.w && y < r.y+r.h
}

type dashboardLayout struct {
	mode     dashboardRenderMode
	tabs     dashboardRect
	body     dashboardRect
	tasks    dashboardRect
	workers  dashboardRect
	evidence dashboardRect
	events   dashboardRect
}

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
	artifacts  []*mission.Artifact

	// Focus and scrolling.
	focusPane       pane
	scrollPos       [paneCount]int
	terminalFocused bool
	lastErr         error
	quitting        bool
}

type dashboardSnapshot struct {
	missionID  string
	missionObj *mission.Mission
	summary    *mission.MissionSummary
	tasks      []*mission.Task
	deps       []mission.TaskDependency
	runs       []*mission.Run
	events     []*mission.Event
	approvals  []*mission.Approval
	artifacts  []*mission.Artifact
}

// New creates a dashboard model for the given mission ID.
// If missionID is empty, displays the most recent active mission.
func New(missionID string) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	return &Model{
		ctx:             ctx,
		cancel:          cancel,
		missionID:       missionID,
		sty:             styles.New(nil),
		terminalFocused: true,
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
	return tea.Batch(tea.RequestBackgroundColor, m.initStore(), tickCmd())
}

func (m *Model) setFocus(active bool) {
	m.terminalFocused = active
}

func (m *Model) setFocusPane(p pane) {
	m.focusPane = p
}

func (m *Model) scrollFocusedPane(delta int) {
	next := m.scrollPos[m.focusPane] + delta
	if next < 0 {
		next = 0
	}
	m.scrollPos[m.focusPane] = next
}

func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	mouse := msg.Mouse()
	if m.width <= 0 || m.height <= 0 {
		return nil
	}
	if wheel, ok := msg.(tea.MouseWheelMsg); ok {
		switch wheel.Mouse().Button {
		case tea.MouseWheelUp:
			m.scrollFocusedPane(-1)
		case tea.MouseWheelDown:
			m.scrollFocusedPane(1)
		}
		return nil
	}
	if _, ok := msg.(tea.MouseClickMsg); !ok {
		return nil
	}
	if p, ok := m.paneAt(mouse.X, mouse.Y); ok {
		m.setFocusPane(p)
	}
	return nil
}

func (m *Model) paneAt(x, y int) (pane, bool) {
	if x < 0 || y < 0 || m.width <= 0 || m.height <= 0 {
		return paneTasks, false
	}
	layout := m.currentLayout()
	if p, ok := m.paneFromTabs(x, y); ok {
		return p, true
	}
	switch layout.mode {
	case dashboardRenderFull:
		switch {
		case layout.tasks.contains(x, y):
			return paneTasks, true
		case layout.workers.contains(x, y):
			return paneWorkers, true
		case layout.evidence.contains(x, y):
			return paneEvidence, true
		case layout.events.contains(x, y):
			return paneEvents, true
		}
	case dashboardRenderCompact:
		if !layout.body.contains(x, y) {
			return paneTasks, false
		}
		return m.focusPane, true
	case dashboardRenderNoMission, dashboardRenderError, dashboardRenderNone:
		return paneTasks, false
	}
	return paneTasks, false
}

func (m *Model) currentLayout() dashboardLayout {
	if m.width <= 0 || m.height <= 0 {
		return dashboardLayout{mode: dashboardRenderNone}
	}
	if m.missionObj == nil {
		if m.lastErr != nil && !isNoMissionError(m.lastErr) {
			return dashboardLayout{mode: dashboardRenderError}
		}
		return dashboardLayout{mode: dashboardRenderNoMission}
	}
	if m.useCompactLayout() {
		return m.compactDashboardLayout()
	}
	return m.fullDashboardLayout()
}

func (m *Model) fullDashboardLayout() dashboardLayout {
	headerHeight := len(m.renderHeader())
	tabsY := headerHeight
	separatorY := tabsY + 1
	bodyY := separatorY + 1
	eventHeight := 2
	if m.height >= 18 {
		eventHeight = 3
	}
	bodyHeight := m.height - (headerHeight + 1 + 1) - 1 - eventHeight - 1
	if bodyHeight < 4 {
		bodyHeight = 4
	}
	eventsY := bodyY + bodyHeight + 1

	leftWidth := m.width * 3 / 5
	rightWidth := m.width - leftWidth - 1
	if rightWidth < 10 {
		rightWidth = 10
		leftWidth = m.width - rightWidth - 1
	}
	if leftWidth < 1 {
		leftWidth = 1
		rightWidth = max(1, m.width-leftWidth-1)
	}
	if rightWidth < 1 {
		rightWidth = 1
	}

	taskHeight := bodyHeight * 3 / 5
	if taskHeight < 1 {
		taskHeight = 1
	}
	evidenceHeight := bodyHeight - taskHeight
	if evidenceHeight < 1 {
		evidenceHeight = 1
	}

	return dashboardLayout{
		mode:     dashboardRenderFull,
		tabs:     dashboardRect{x: 0, y: tabsY, w: m.width, h: 1},
		body:     dashboardRect{x: 0, y: bodyY, w: m.width, h: bodyHeight},
		tasks:    dashboardRect{x: 0, y: bodyY, w: leftWidth, h: taskHeight},
		evidence: dashboardRect{x: 0, y: bodyY + taskHeight, w: leftWidth, h: evidenceHeight},
		workers:  dashboardRect{x: leftWidth + 1, y: bodyY, w: rightWidth, h: bodyHeight},
		events:   dashboardRect{x: 0, y: eventsY, w: m.width, h: eventHeight},
	}
}

func (m *Model) compactDashboardLayout() dashboardLayout {
	headerHeight := len(m.renderCompactHeader())
	tabsY := headerHeight
	separatorY := tabsY + 1
	bodyY := separatorY + 1
	paneHeight := max(1, m.height-(headerHeight+1+1)-2)
	return dashboardLayout{
		mode: dashboardRenderCompact,
		tabs: dashboardRect{x: 0, y: tabsY, w: m.width, h: 1},
		body: dashboardRect{x: 0, y: bodyY, w: m.width, h: paneHeight},
	}
}

func (m *Model) paneFromTabs(x, y int) (pane, bool) {
	layout := m.currentLayout()
	if !layout.tabs.contains(x, y) {
		return paneTasks, false
	}
	joinerWidth := lipgloss.Width(m.sty.Panel.Separator.Render(" "))
	cursor := 0
	tabs := []struct {
		title string
		pane  pane
	}{
		{title: "[1] Tasks", pane: paneTasks},
		{title: "[2] Workers", pane: paneWorkers},
		{title: "[3] Evidence", pane: paneEvidence},
		{title: "[4] Events", pane: paneEvents},
	}
	for _, tab := range tabs {
		style := m.sty.Panel.FocusTabInactive
		if m.focusPane == tab.pane {
			style = m.sty.Panel.FocusTabActive
		}
		tabWidth := lipgloss.Width(style.Render(tab.title))
		if x >= cursor && x < cursor+tabWidth {
			return tab.pane, true
		}
		cursor += tabWidth + joinerWidth
		if cursor >= m.width {
			break
		}
	}
	return paneTasks, false
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
		m.clearSnapshot()
		return refreshDoneMsg{err: fmt.Errorf("store not initialized")}
	}

	ctx := m.ctx
	nextMissionID := m.missionID

	// If no specific mission, find the most recent active one.
	if nextMissionID == "" {
		missions, err := m.ctrl.Store().ListMissions(ctx)
		if err != nil {
			m.clearSnapshot()
			return refreshDoneMsg{err: err}
		}
		// Prefer running > blocked > paused > awaiting_approval > planning > draft.
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
			nextMissionID = best.ID
		}
	}

	if nextMissionID == "" {
		m.clearSnapshot()
		return refreshDoneMsg{err: fmt.Errorf("no missions found")}
	}

	ms, err := m.ctrl.GetMission(ctx, nextMissionID)
	if err != nil {
		m.clearSnapshot()
		return refreshDoneMsg{err: err}
	}

	summary, err := m.ctrl.GetMissionSummary(ctx, nextMissionID)
	if err != nil {
		m.clearSnapshot()
		return refreshDoneMsg{err: err}
	}

	tasks, err := m.ctrl.Store().ListTasks(ctx, nextMissionID)
	if err != nil {
		m.clearSnapshot()
		return refreshDoneMsg{err: err}
	}

	deps, err := m.ctrl.Store().ListDependencies(ctx, nextMissionID)
	if err != nil {
		m.clearSnapshot()
		return refreshDoneMsg{err: err}
	}

	runs, err := m.ctrl.Store().ListRuns(ctx, nextMissionID)
	if err != nil {
		m.clearSnapshot()
		return refreshDoneMsg{err: err}
	}

	events, err := m.ctrl.Store().ListEvents(ctx, nextMissionID, 50)
	if err != nil {
		m.clearSnapshot()
		return refreshDoneMsg{err: err}
	}

	approvals, err := m.ctrl.Store().ListApprovals(ctx, nextMissionID)
	if err != nil {
		m.clearSnapshot()
		return refreshDoneMsg{err: err}
	}

	artifacts, err := m.ctrl.Store().ListArtifacts(ctx, nextMissionID)
	if err != nil {
		m.clearSnapshot()
		return refreshDoneMsg{err: err}
	}

	m.applySnapshot(dashboardSnapshot{
		missionID:  nextMissionID,
		missionObj: ms,
		summary:    summary,
		tasks:      tasks,
		deps:       deps,
		runs:       runs,
		events:     events,
		approvals:  approvals,
		artifacts:  artifacts,
	})

	return refreshDoneMsg{}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		isDark := msg.IsDark()
		m.sty = styles.NewMode(msg.Color, &isDark)
		return m, nil

	case tea.FocusMsg:
		m.setFocus(true)
		return m, nil

	case tea.BlurMsg:
		m.setFocus(false)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.sty == nil {
			m.sty = styles.New(nil)
		}
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
			m.setFocusPane((m.focusPane + 1) % paneCount)
			return m, nil
		case "shift+tab":
			m.setFocusPane((m.focusPane - 1 + paneCount) % paneCount)
			return m, nil
		case "j", "down":
			m.scrollFocusedPane(1)
			return m, nil
		case "k", "up":
			m.scrollFocusedPane(-1)
			return m, nil
		case "1":
			m.setFocusPane(paneTasks)
			return m, nil
		case "2":
			m.setFocusPane(paneWorkers)
			return m, nil
		case "3":
			m.setFocusPane(paneEvidence)
			return m, nil
		case "4":
			m.setFocusPane(paneEvents)
			return m, nil
		}

	case tea.MouseMsg:
		return m, m.handleMouse(msg)
	}

	return m, nil
}

func (m *Model) View() tea.View {
	if m.quitting {
		v := tea.NewView("")
		m.configureView(&v)
		return v
	}
	if m.width == 0 || m.height == 0 {
		v := tea.NewView("Loading...")
		m.configureView(&v)
		return v
	}
	if m.missionObj == nil {
		if m.lastErr != nil && !isNoMissionError(m.lastErr) {
			return m.renderErrorView()
		}
		return m.renderNoMissionView()
	}
	if m.useCompactLayout() {
		return m.renderCompactDashboardView()
	}
	return m.renderFullDashboardView()
}

func (m *Model) renderFullDashboardView() tea.View {
	lines := append([]string{}, m.renderHeader()...)
	lines = append(lines, m.renderFocusTabs(max(1, m.width)))
	lines = append(lines, m.renderSeparator())

	eventHeight := 2
	if m.height >= 18 {
		eventHeight = 3
	}
	bodyHeight := m.height - len(lines) - 1 - eventHeight - 1 // separator + events + footer
	if bodyHeight < 4 {
		bodyHeight = 4
	}

	leftWidth := m.width * 3 / 5
	rightWidth := m.width - leftWidth - 1
	if rightWidth < 10 {
		rightWidth = 10
		leftWidth = m.width - rightWidth - 1
	}
	if leftWidth < 1 {
		leftWidth = 1
		rightWidth = max(1, m.width-leftWidth-1)
	}

	taskHeight := bodyHeight * 3 / 5
	evidenceHeight := bodyHeight - taskHeight

	leftLines := append(m.renderTaskPane(taskHeight, leftWidth), m.renderEvidencePane(evidenceHeight, leftWidth)...)
	rightLines := m.renderWorkerPane(bodyHeight, rightWidth)

	divider := m.sty.Muted.Render("│")
	for i := 0; i < bodyHeight; i++ {
		left := ""
		if i < len(leftLines) {
			left = leftLines[i]
		}
		right := ""
		if i < len(rightLines) {
			right = rightLines[i]
		}
		lines = append(lines, padRight(left, leftWidth)+divider+padRight(right, rightWidth))
	}

	lines = append(lines, m.renderSeparator())
	lines = append(lines, m.renderEventPane(eventHeight, m.width)...)
	lines = append(lines, m.renderFooter())
	return m.finalizeView(lines)
}

func (m *Model) renderCompactDashboardView() tea.View {
	lines := append([]string{}, m.renderCompactHeader()...)
	lines = append(lines, m.renderFocusTabs(max(1, m.width)))
	lines = append(lines, m.renderSeparator())
	lines = append(lines, m.renderCompactSupportLine(max(1, m.width)))

	paneHeight := max(1, m.height-len(lines)-1) // footer
	lines = append(lines, m.renderFocusedPaneLines(paneHeight, max(1, m.width), true)...)
	lines = append(lines, m.renderFooter())
	return m.finalizeView(lines)
}

func (m *Model) renderCompactHeader() []string {
	if m.missionObj == nil {
		return []string{m.sty.Panel.Title.Render("Mission Control") + "  " + m.renderStatusChip("idle", "No mission")}
	}

	ms := m.missionObj
	counts := m.missionTaskCounts()
	done := counts.Done + counts.Integrated + counts.Accepted
	titleLine := m.sty.Panel.Title.Render("Mission Control") + "  " + m.renderStatusChip(string(ms.Status), fmt.Sprintf("%s %s", missionStatusIcon(ms.Status), ms.Status))
	missionTitle := strings.TrimSpace(ms.Title)
	if missionTitle == "" {
		missionTitle = ms.ID
	}
	summaryLine := strings.Join([]string{
		fmt.Sprintf("Tasks %d/%d", done, counts.Total),
		fmt.Sprintf("Workers %d active", m.activeRunsCount()),
	}, "  •  ")
	if pending := m.pendingApprovals(); pending > 0 {
		summaryLine += fmt.Sprintf("  •  Approvals %d pending", pending)
	}

	if m.isUltraCompactLayout() {
		lines := []string{
			titleLine,
			m.sty.Bold.Render(ansi.Truncate(missionTitle, max(1, m.width), "…")),
		}
		if ms.Goal != "" {
			lines = append(lines, m.sty.HalfMuted.Render(ansi.Truncate(ms.Goal, max(1, m.width), "…")))
		}
		lines = append(lines, m.sty.HalfMuted.Render(ansi.Truncate(summaryLine, max(1, m.width), "…")))
		return lines
	}

	return []string{
		titleLine,
		m.sty.Bold.Render(ansi.Truncate(missionTitle, max(1, m.width), "…")),
		m.sty.HalfMuted.Render(ansi.Truncate(ms.Goal, max(1, m.width), "…")),
		m.sty.HalfMuted.Render(ansi.Truncate(summaryLine, max(1, m.width), "…")),
	}
}

func (m *Model) renderNoMissionView() tea.View {
	lines := []string{
		m.sty.Panel.Title.Render("Mission Control") + "  " + m.renderStatusChip("idle", "No mission"),
	}
	lines = append(lines, m.renderStateCard(
		max(1, m.width),
		"No active mission",
		"Create one with /mission new or run golem mission new. The dashboard will attach as soon as durable mission state exists.",
		[]string{
			"Open the main shell and create a mission.",
			"Leave Mission Control open and press r to refresh when work starts.",
		},
	)...)
	lines = append(lines, m.renderHeaderHelpHint(max(1, m.width)))
	lines = append(lines, m.renderFooter())
	return m.finalizeView(lines)
}

func (m *Model) renderErrorView() tea.View {
	body := "Mission Control could not load durable mission state."
	if m.lastErr != nil {
		body = body + " " + m.lastErr.Error()
	}
	lines := []string{
		m.sty.Panel.Title.Render("Mission Control") + "  " + m.renderStatusChip("error", "Dashboard error"),
	}
	lines = append(lines, m.renderStateCard(
		max(1, m.width),
		"Unable to load dashboard",
		body,
		[]string{
			"Press r to retry the refresh.",
			"Press q to quit if the mission store is unavailable.",
		},
	)...)
	lines = append(lines, m.renderDashboardHelpLine(max(1, m.width), "/mission status in shell"))
	lines = append(lines, m.renderFooter())
	return m.finalizeView(lines)
}

func (m *Model) finalizeView(lines []string) tea.View {
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	m.configureView(&v)
	return v
}

func (m *Model) renderFocusedPaneLines(height, width int, compact bool) []string {
	switch m.focusPane {
	case paneWorkers:
		return m.renderWorkerPane(height, width)
	case paneEvidence:
		lines := m.renderEvidencePane(height+1, width)
		if compact && len(lines) > 0 {
			lines = lines[1:]
		}
		for len(lines) < height {
			lines = append(lines, "")
		}
		return lines[:height]
	case paneEvents:
		return m.renderEventPane(height, width)
	default:
		return m.renderTaskPane(height, width)
	}
}

func (m *Model) applySnapshot(snapshot dashboardSnapshot) {
	if snapshot.missionID != "" {
		m.missionID = snapshot.missionID
	}
	m.missionObj = snapshot.missionObj
	m.summary = snapshot.summary
	m.tasks = snapshot.tasks
	m.deps = snapshot.deps
	m.runs = snapshot.runs
	m.events = snapshot.events
	m.approvals = snapshot.approvals
	m.artifacts = snapshot.artifacts
}

func (m *Model) clearSnapshot() {
	m.missionObj = nil
	m.summary = nil
	m.tasks = nil
	m.deps = nil
	m.runs = nil
	m.events = nil
	m.approvals = nil
	m.artifacts = nil
}

func (m *Model) missionSummary() mission.MissionSummary {
	if m.summary == nil {
		return mission.MissionSummary{}
	}
	return *m.summary
}

func (m *Model) missionTaskCounts() mission.TaskCounts {
	return m.missionSummary().TaskCounts
}

func (m *Model) pendingApprovals() int {
	summary := m.missionSummary()
	if summary.PendingApprovals > 0 {
		return summary.PendingApprovals
	}
	pending := 0
	for _, approval := range m.approvals {
		if approval.Status == mission.ApprovalPending {
			pending++
		}
	}
	return pending
}

func (m *Model) activeRunsCount() int {
	summary := m.missionSummary()
	if summary.ActiveRuns > 0 {
		return summary.ActiveRuns
	}
	activeRuns := 0
	for _, r := range m.runs {
		if r.Status == mission.RunRunning || r.Status == mission.RunQueued {
			activeRuns++
		}
	}
	return activeRuns
}

func (m *Model) isUltraCompactLayout() bool {
	return m.width < 56 || m.height < 14
}

func (m *Model) useCompactLayout() bool {
	return m.width < compactDashboardWidth || m.height < compactDashboardHeight
}

func (m *Model) configureView(v *tea.View) {
	if v == nil {
		return
	}
	v.AltScreen = true
	v.WindowTitle = m.windowTitle()
	v.ReportFocus = true
	v.MouseMode = tea.MouseModeCellMotion
}

func (m *Model) windowTitle() string {
	title := "GOLEM Dashboard"
	if m.missionObj != nil {
		missionTitle := strings.TrimSpace(m.missionObj.Title)
		if missionTitle == "" {
			missionTitle = m.missionObj.ID
		}
		if missionTitle != "" {
			title += " — " + missionTitle
		}
	} else if m.missionID != "" {
		title += " — " + m.missionID
	}
	if !m.terminalFocused {
		title += " — unfocused"
	}
	return title
}

func (m *Model) renderCompactSupportLine(width int) string {
	title := "Compact"
	segments := []string{
		"Focus " + m.currentPaneTitle(),
		"q quit",
		"j/k scroll",
	}
	ultraCompact := width < 54 || (m.height > 0 && m.height < 14)
	if !ultraCompact {
		title = "Compact layout"
		if width >= 64 {
			segments = append(segments, "Tab panes")
		}
		if width >= 104 {
			segments = append(segments, "resize wider for the full four-pane Mission Control view")
		}
	}
	return renderDashboardHelpSurfaceLine(width, title, segments, func(text string) string {
		return m.sty.Panel.EmptyBody.Render(text)
	})
}

func (m *Model) currentPaneTitle() string {
	switch m.focusPane {
	case paneWorkers:
		return "Workers"
	case paneEvidence:
		return "Evidence"
	case paneEvents:
		return "Events"
	default:
		return "Tasks"
	}
}

func (m *Model) renderHeaderHelpHint(width int) string {
	if m.missionObj == nil {
		return m.renderDashboardHelpLine(width, "/mission new in shell")
	}
	return m.renderDashboardHelpLine(width, "/mission status in shell")
}

func (m *Model) renderDashboardHelpLine(width int, shellHint string) string {
	segments := []string{shellHint, "Tab switch panes", "Shift+Tab reverse", "1-4 jump", "j/k scroll", "r refresh", "q quit"}
	return renderDashboardHelpSurfaceLine(width, dashboardHelpTitle(), segments, func(text string) string {
		return m.sty.Panel.EmptyBody.Render(text)
	})
}

func (m *Model) renderStateCard(width int, title, body string, actions []string) []string {
	lines := []string{m.sty.Panel.EmptyTitle.Render(ansi.Truncate(title, max(1, width), "…"))}
	for _, line := range wrapPlainText(body, max(1, width-2)) {
		lines = append(lines, m.sty.Panel.EmptyBody.Render(" "+line))
	}
	for _, action := range actions {
		lines = append(lines, m.sty.Panel.EmptyHint.Render(" → "+ansi.Truncate(action, max(1, width-4), "…")))
	}
	return lines
}

func (m *Model) renderFocusTabs(width int) string {
	if width <= 0 {
		width = 1
	}
	tabs := []struct {
		title string
		pane  pane
	}{
		{title: "[1] Tasks", pane: paneTasks},
		{title: "[2] Workers", pane: paneWorkers},
		{title: "[3] Evidence", pane: paneEvidence},
		{title: "[4] Events", pane: paneEvents},
	}
	joiner := m.sty.Panel.Separator.Render(" ")
	var rendered []string
	for _, tab := range tabs {
		style := m.sty.Panel.FocusTabInactive
		if m.focusPane == tab.pane {
			style = m.sty.Panel.FocusTabActive
		}
		rendered = append(rendered, style.Render(tab.title))
	}
	return ansi.Truncate(strings.Join(rendered, joiner), width, "…")
}

func dashboardHelpTitle() string {
	return "Help"
}

func renderDashboardHelpSurfaceLine(width int, title string, segments []string, style func(string) string) string {
	if len(segments) == 0 {
		return ""
	}
	if width <= 0 {
		width = 1
	}
	content := strings.TrimSpace(title)
	if content == "" {
		content = "Help"
	}
	content += " · " + strings.Join(segments, " · ")
	content = "  " + ansi.Truncate(content, max(1, width-2), "…")
	if style != nil {
		return style(content)
	}
	return content
}

// renderHeader renders the mission header.
func (m *Model) renderHeader() []string {
	if m.missionObj == nil {
		titleLine := m.sty.Panel.Title.Render("Mission Control") + "  " + m.renderStatusChip("idle", "No mission")
		return []string{
			titleLine,
			m.sty.Panel.EmptyTitle.Render("No active mission"),
			m.sty.Panel.EmptyBody.Render("Create one with /mission new or run golem mission new."),
			m.sty.Panel.EmptyBody.Render("The dashboard will attach as soon as durable mission state exists."),
			m.renderHeaderHelpHint(max(1, m.width)),
		}
	}

	ms := m.missionObj
	c := m.summary.TaskCounts
	activeRuns := m.summary.ActiveRuns
	if activeRuns == 0 {
		for _, r := range m.runs {
			if r.Status == mission.RunRunning || r.Status == mission.RunQueued {
				activeRuns++
			}
		}
	}

	titleLine := m.sty.Panel.Title.Render("Mission Control") + "  " + m.renderStatusChip(string(ms.Status), fmt.Sprintf("%s %s", missionStatusIcon(ms.Status), ms.Status))

	missionTitle := strings.TrimSpace(ms.Title)
	if missionTitle == "" {
		missionTitle = ms.ID
	}
	missionLine := m.sty.Bold.Render(ansi.Truncate(missionTitle, max(1, m.width), "…"))

	done := c.Done + c.Integrated + c.Accepted
	blocked := c.Blocked + c.Failed
	metricSegments := []string{
		m.renderMetric("Tasks", fmt.Sprintf("%d/%d complete", done, c.Total)),
		m.renderMetric("Workers", fmt.Sprintf("%d active", activeRuns)),
	}
	if c.Running > 0 {
		metricSegments = append(metricSegments, m.renderMetric("Running", fmt.Sprintf("%d now", c.Running)))
	}
	if c.Ready > 0 {
		metricSegments = append(metricSegments, m.renderMetric("Ready", fmt.Sprintf("%d queued", c.Ready)))
	}
	if c.AwaitingReview > 0 {
		metricSegments = append(metricSegments, m.renderMetric("Review", fmt.Sprintf("%d waiting", c.AwaitingReview)))
	}
	if blocked > 0 {
		metricSegments = append(metricSegments, m.renderMetric("Blocked", fmt.Sprintf("%d stalled", blocked)))
	}
	if m.summary.PendingApprovals > 0 {
		metricSegments = append(metricSegments, m.renderMetric("Approvals", fmt.Sprintf("%d pending", m.summary.PendingApprovals)))
	}
	if len(m.artifacts) > 0 {
		metricSegments = append(metricSegments, m.renderMetric("Evidence", fmt.Sprintf("%d items", len(m.artifacts))))
	}
	if ms.StartedAt != nil {
		metricSegments = append(metricSegments, m.renderMetric("Elapsed", time.Since(*ms.StartedAt).Truncate(time.Second).String()))
	}

	repoName := filepath.Base(strings.TrimSpace(ms.RepoRoot))
	if repoName == "." || repoName == string(filepath.Separator) {
		repoName = ""
	}
	metaSegments := []string{}
	if ms.ID != "" {
		metaSegments = append(metaSegments, m.renderMetric("Mission", shortenID(ms.ID, 12)))
	}
	if repoName != "" {
		metaSegments = append(metaSegments, m.renderMetric("Repo", repoName))
	}
	if ms.BaseBranch != "" {
		metaSegments = append(metaSegments, m.renderMetric("Branch", ms.BaseBranch))
	}
	if ms.Budget.MaxConcurrentWorkers > 0 {
		metaSegments = append(metaSegments, m.renderMetric("Budget", fmt.Sprintf("%d workers", ms.Budget.MaxConcurrentWorkers)))
	}

	lines := []string{titleLine, missionLine}
	lines = append(lines, wrapSegments(metricSegments, max(1, m.width), m.sty.Panel.Separator.Render(" • "))...)
	if len(metaSegments) > 0 {
		lines = append(lines, wrapSegments(metaSegments, max(1, m.width), m.sty.Panel.Separator.Render(" • "))...)
	}

	goalLabel := m.sty.Panel.MetricKey.Render("Goal")
	goalText := ansi.Truncate(ms.Goal, max(1, m.width-lipgloss.Width(goalLabel)-1), "…")
	lines = append(lines, goalLabel+" "+m.sty.HalfMuted.Render(goalText))
	lines = append(lines, m.renderHeaderHelpHint(max(1, m.width)))
	return lines
}

// renderTaskPane renders the task DAG view.
func (m *Model) renderTaskPane(height, width int) []string {
	header := m.renderPaneHeader("Tasks", m.focusPane == paneTasks, width)
	lines := []string{header}
	budget := height - 1

	if len(m.tasks) == 0 {
		lines = append(lines, m.renderEmptyState(width, "No tasks yet", "Plan the mission to populate the task graph and operator queue.")...)
		for len(lines) < height {
			lines = append(lines, "")
		}
		return lines[:height]
	}

	depMap := make(map[string][]string)
	for _, d := range m.deps {
		depMap[d.TaskID] = append(depMap[d.TaskID], d.DependsOnID)
	}

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

		allItems = append(allItems, m.renderSectionLabel(g.label, len(groupTasks), width))
		for _, t := range groupTasks {
			title := ansi.Truncate(t.Title, max(1, width-11), "…")
			if t.Status == mission.TaskDone || t.Status == mission.TaskIntegrated || t.Status == mission.TaskAccepted {
				title = m.sty.Panel.TaskDone.Render(title)
			} else {
				title = m.sty.Panel.TaskText.Render(title)
			}
			badge := lipgloss.NewStyle().Foreground(m.sty.FgMuted).Render(fmt.Sprintf("[%s]", taskStatusShort(t.Status)))
			line := fmt.Sprintf(" %s %s %s", taskIcon(t.Status, m.sty), badge, title)
			allItems = append(allItems, ansi.Truncate(line, max(1, width-1), "…"))
			if depsOn := depMap[t.ID]; len(depsOn) > 0 {
				depStr := ansi.Truncate("needs: "+strings.Join(depsOn, ", "), max(1, width-8), "…")
				allItems = append(allItems, m.sty.Muted.Render("   └─ "+depStr))
			}
		}
		allItems = append(allItems, "")
	}

	if len(allItems) > 0 && allItems[len(allItems)-1] == "" {
		allItems = allItems[:len(allItems)-1]
	}
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

	var activeRuns []*mission.Run
	for _, r := range m.runs {
		if r.Status == mission.RunRunning || r.Status == mission.RunQueued {
			activeRuns = append(activeRuns, r)
		}
	}

	if len(activeRuns) == 0 {
		lines = append(lines, m.renderEmptyState(width, "No active workers", "Mission Control is idle. Start the mission to see workers, reviews, and queued work.")...)
		for len(lines) < height {
			lines = append(lines, "")
		}
		return lines[:height]
	}

	taskMap := make(map[string]*mission.Task)
	for _, t := range m.tasks {
		taskMap[t.ID] = t
	}

	offset := m.scrollPos[paneWorkers]
	allItems := []string{m.renderSectionLabel("Active", len(activeRuns), width)}
	for _, r := range activeRuns {
		runID := r.ID[:min(12, len(r.ID))]
		headerLine := fmt.Sprintf(" %s %s [%s]", runModeIcon(r.Mode), runID, runStatusStyle(r.Status, m.sty))
		allItems = append(allItems, ansi.Truncate(m.sty.Bold.Render(headerLine), max(1, width-1), "…"))

		if t, ok := taskMap[r.TaskID]; ok {
			taskLine := fmt.Sprintf("   task: %s", ansi.Truncate(t.Title, max(1, width-10), "…"))
			allItems = append(allItems, m.sty.Panel.TaskText.Render(taskLine))
		}

		var metaParts []string
		if r.WorktreePath != "" {
			metaParts = append(metaParts, "wt: "+filepath.Base(r.WorktreePath))
		}
		if r.HeartbeatAt != nil {
			metaParts = append(metaParts, fmt.Sprintf("heartbeat: %s ago", time.Since(*r.HeartbeatAt).Truncate(time.Second)))
		}
		if len(metaParts) > 0 {
			allItems = append(allItems, m.sty.Muted.Render(ansi.Truncate("   "+strings.Join(metaParts, "  •  "), max(1, width-2), "…")))
		}
		if r.Summary != "" {
			allItems = append(allItems, m.sty.HalfMuted.Render("   summary: "+ansi.Truncate(r.Summary, max(1, width-13), "…")))
		}
		allItems = append(allItems, "")
	}

	var recentDone []*mission.Run
	for _, r := range m.runs {
		if (r.Status == mission.RunSucceeded || r.Status == mission.RunFailed) && r.EndedAt != nil && time.Since(*r.EndedAt) < 5*time.Minute {
			recentDone = append(recentDone, r)
		}
	}
	if len(recentDone) > 0 {
		allItems = append(allItems, m.renderSectionLabel("Recent", len(recentDone), width))
		for _, r := range recentDone {
			icon := "✓"
			if r.Status == mission.RunFailed {
				icon = "✗"
			}
			line := fmt.Sprintf(" %s %s [%s]", icon, r.ID[:min(12, len(r.ID))], r.Status)
			allItems = append(allItems, m.sty.Muted.Render(ansi.Truncate(line, max(1, width-2), "…")))
		}
	}

	if len(allItems) > 0 && allItems[len(allItems)-1] == "" {
		allItems = allItems[:len(allItems)-1]
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
	sep := m.sty.Panel.Separator.Render(strings.Repeat(styles.Separator, max(1, width)))
	header := m.renderPaneHeader("Evidence", m.focusPane == paneEvidence, width)
	lines := []string{sep, header}
	budget := height - 2

	offset := m.scrollPos[paneEvidence]
	var allItems []string

	var reviewLines []string
	for _, r := range m.runs {
		if r.Mode != mission.RunModeReview {
			continue
		}
		if r.Status != mission.RunSucceeded && r.Status != mission.RunFailed {
			continue
		}
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
		reviewLines = append(reviewLines, m.sty.Panel.TaskText.Render(line))
	}
	if len(reviewLines) > 0 {
		allItems = append(allItems, m.renderSectionLabel("Reviews", len(reviewLines), width))
		allItems = append(allItems, reviewLines...)
	}

	var approvalLines []string
	for _, a := range m.approvals {
		if a.Status != mission.ApprovalPending {
			continue
		}
		line := fmt.Sprintf(" ⏳ Approval: %s [pending]", a.Kind)
		approvalLines = append(approvalLines, m.sty.Panel.TaskText.Render(ansi.Truncate(line, max(1, width-2), "…")))
	}
	if len(approvalLines) > 0 {
		allItems = append(allItems, m.renderSectionLabel("Approvals", len(approvalLines), width))
		allItems = append(allItems, approvalLines...)
	}

	var failureLines []string
	for _, r := range m.runs {
		if r.Status != mission.RunFailed || r.Mode == mission.RunModeReview {
			continue
		}
		errText := r.ErrorText
		if errText == "" {
			errText = "unknown error"
		}
		line := fmt.Sprintf(" ✗ %s %s: %s", r.Mode, r.TaskID, ansi.Truncate(errText, max(1, width-20), "…"))
		failureLines = append(failureLines, m.sty.Panel.TaskText.Render(ansi.Truncate(line, max(1, width-2), "…")))
	}
	if len(failureLines) > 0 {
		allItems = append(allItems, m.renderSectionLabel("Failures", len(failureLines), width))
		allItems = append(allItems, failureLines...)
	}

	var artifactLines []string
	for _, a := range m.artifacts {
		label := a.Type
		if label == "" {
			label = "artifact"
		}
		target := a.RelativePath
		if target == "" {
			target = shortenID(a.ID, 12)
		}
		line := fmt.Sprintf(" %s %s: %s", styles.ResultPrefix, label, ansi.Truncate(target, max(1, width-16), "…"))
		artifactLines = append(artifactLines, m.sty.Panel.TaskText.Render(ansi.Truncate(line, max(1, width-2), "…")))
	}
	if len(artifactLines) > 0 {
		allItems = append(allItems, m.renderSectionLabel("Artifacts", len(artifactLines), width))
		allItems = append(allItems, artifactLines...)
	}

	if len(allItems) == 0 {
		allItems = append(allItems, m.renderEmptyState(width, "No evidence yet", "Reviews, approvals, failures, and artifacts will collect here as the mission runs.")...)
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
	for i := len(m.events) - 1; i >= 0; i-- {
		e := m.events[i]
		detail := e.Type
		if e.TaskID != "" {
			detail += " " + e.TaskID
		}
		line := fmt.Sprintf(" %s %s %s", eventIcon(e.Type), e.CreatedAt.Format("15:04:05"), detail)
		allItems = append(allItems, m.sty.Muted.Render(ansi.Truncate(line, max(1, width-2), "…")))
	}
	if len(allItems) == 0 {
		allItems = append(allItems, m.renderEmptyState(width, "No events yet", "Mission lifecycle, scheduling, and approval events will stream here.")...)
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

func (m *Model) renderSeparator() string {
	return m.sty.Panel.Separator.Render(strings.Repeat(styles.Separator, max(1, m.width)))
}

func (m *Model) renderFooter() string {
	keys := []string{
		"q:quit",
		"r:refresh",
		"tab/shift+tab:panes",
		"j/k:scroll",
		"1-4:jump",
	}
	if m.missionObj != nil {
		keys = append(keys, "/mission status:shell")
	} else {
		keys = append(keys, "/mission new:shell")
	}
	keys = append(keys, "operator view")
	return m.sty.Muted.Render(ansi.Truncate(strings.Join(keys, "  •  "), max(1, m.width), "…"))
}

func (m *Model) renderSectionLabel(title string, count, width int) string {
	label := fmt.Sprintf(" %s (%d) ", title, count)
	if lipgloss.Width(label) < width {
		label += strings.Repeat(styles.Separator, width-lipgloss.Width(label))
	}
	return m.sty.Panel.Progress.Render(ansi.Truncate(label, max(1, width), "…"))
}

// renderPaneHeader renders a pane title with focus indicator.
func (m *Model) renderPaneHeader(title string, focused bool, width int) string {
	indicator := "○"
	headStyle := m.sty.Panel.HeaderInactive
	metaStyle := m.sty.Panel.HeaderMeta
	meta := "Tab/Shift+Tab focus • 1-4 jump"
	if focused {
		indicator = "▸"
		headStyle = m.sty.Panel.HeaderActive
		metaStyle = m.sty.Panel.HeaderMeta.Bold(true)
		meta = "ACTIVE • j/k scroll • Tab/Shift+Tab panes"
	}
	label := fmt.Sprintf("%s %s %s", indicator, paneShortcut(title), title)
	line := headStyle.Render(label) + " " + metaStyle.Render(meta)
	return ansi.Truncate(line, max(1, width), "…")
}

func (m *Model) renderMetric(key, value string) string {
	return lipgloss.JoinHorizontal(lipgloss.Left,
		m.sty.Panel.MetricKey.Render(key),
		" ",
		m.sty.Panel.MetricValue.Render(value),
	)
}

func (m *Model) renderStatusChip(kind, text string) string {
	style := lipgloss.NewStyle().
		Foreground(m.sty.FgBase).
		Background(m.sty.BgSubtle).
		Bold(true).
		Padding(0, 1)
	lower := strings.ToLower(kind)
	switch {
	case strings.Contains(lower, "run") || strings.Contains(lower, "active"):
		style = style.Background(m.sty.Blue)
	case strings.Contains(lower, "await") || strings.Contains(lower, "review"):
		style = style.Background(m.sty.Yellow)
	case strings.Contains(lower, "block") || strings.Contains(lower, "fail") || strings.Contains(lower, "error"):
		style = style.Background(m.sty.Red)
	case strings.Contains(lower, "done") || strings.Contains(lower, "complete") || strings.Contains(lower, "success"):
		style = style.Background(m.sty.Green)
	case strings.Contains(lower, "idle") || strings.Contains(lower, "draft") || strings.Contains(lower, "pause"):
		style = style.Background(m.sty.BgSubtle)
	default:
		style = style.Background(m.sty.Primary)
	}
	return style.Render(text)
}

func (m *Model) renderEmptyState(width int, title, body string) []string {
	lines := []string{m.sty.Panel.EmptyTitle.Render(" " + title)}
	wrapped := wrapPlainText(body, max(1, width-2))
	for _, line := range wrapped {
		lines = append(lines, m.sty.Panel.EmptyBody.Render(" "+line))
	}
	var hint string
	switch title {
	case "No tasks yet":
		hint = "→ Use /mission plan in the shell to generate the task DAG."
	case "No active workers":
		hint = "→ Use /mission start in the shell when approvals are resolved."
	case "No evidence yet":
		hint = "→ Review results and artifacts will appear automatically."
	case "No events yet":
		hint = "→ Press r to refresh if another process is writing mission state."
	}
	if hint != "" {
		lines = append(lines, m.sty.Panel.EmptyHint.Render(ansi.Truncate(" "+hint, max(1, width), "…")))
	}
	return lines
}

func wrapSegments(segments []string, width int, joiner string) []string {
	if len(segments) == 0 {
		return nil
	}
	if width <= 0 {
		width = 1
	}
	var lines []string
	current := segments[0]
	for _, segment := range segments[1:] {
		candidate := current + joiner + segment
		if lipgloss.Width(candidate) <= width {
			current = candidate
			continue
		}
		lines = append(lines, ansi.Truncate(current, width, "…"))
		current = segment
	}
	lines = append(lines, ansi.Truncate(current, width, "…"))
	return lines
}

func wrapPlainText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		candidate := current + " " + word
		if lipgloss.Width(candidate) <= width {
			current = candidate
			continue
		}
		lines = append(lines, current)
		current = word
	}
	lines = append(lines, current)
	return lines
}

func shortenID(id string, width int) string {
	if width <= 0 {
		return id
	}
	return ansi.Truncate(id, width, "…")
}

func isNoMissionError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "no missions") || strings.Contains(lower, "no active mission")
}

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
	case mission.RunQueued:
		return lipgloss.NewStyle().Foreground(sty.Blue).Render(string(s))
	case mission.RunSucceeded:
		return lipgloss.NewStyle().Foreground(sty.Green).Render(string(s))
	case mission.RunFailed:
		return lipgloss.NewStyle().Foreground(sty.Red).Render(string(s))
	default:
		return string(s)
	}
}

func paneShortcut(title string) string {
	switch title {
	case "Tasks":
		return "[1]"
	case "Workers":
		return "[2]"
	case "Evidence":
		return "[3]"
	case "Events":
		return "[4]"
	default:
		return "[•]"
	}
}

func taskStatusShort(s mission.TaskStatus) string {
	switch s {
	case mission.TaskDone, mission.TaskIntegrated, mission.TaskAccepted:
		return "DONE"
	case mission.TaskRunning, mission.TaskLeased:
		return "RUN"
	case mission.TaskBlocked, mission.TaskFailed, mission.TaskRejected:
		return "BLOCK"
	case mission.TaskReady:
		return "READY"
	case mission.TaskAwaitingReview:
		return "REVIEW"
	default:
		return "PEND"
	}
}

func eventIcon(eventType string) string {
	lower := strings.ToLower(eventType)
	switch {
	case strings.Contains(lower, "fail"):
		return "✗"
	case strings.Contains(lower, "complete"), strings.Contains(lower, "applied"):
		return "✓"
	case strings.Contains(lower, "start"), strings.Contains(lower, "run"):
		return "◐"
	default:
		return "•"
	}
}

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return ansi.Truncate(s, width, "…")
	}
	return s + strings.Repeat(" ", width-w)
}
