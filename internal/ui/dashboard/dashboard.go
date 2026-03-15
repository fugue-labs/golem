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
	artifacts  []*mission.Artifact

	// Focus and scrolling.
	focusPane pane
	scrollPos [paneCount]int
	lastErr   error
	quitting  bool
}

// New creates a dashboard model for the given mission ID.
// If missionID is empty, displays the most recent active mission.
func New(missionID string) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	return &Model{
		ctx:       ctx,
		cancel:    cancel,
		missionID: missionID,
		sty:       styles.New(nil),
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
			m.missionID = best.ID
		}
	}

	if m.missionID == "" {
		m.missionObj = nil
		m.summary = nil
		m.tasks = nil
		m.deps = nil
		m.runs = nil
		m.events = nil
		m.approvals = nil
		m.artifacts = nil
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

	artifacts, err := m.ctrl.Store().ListArtifacts(ctx, m.missionID)
	if err != nil {
		return refreshDoneMsg{err: err}
	}
	m.artifacts = artifacts

	return refreshDoneMsg{}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		isDark := msg.IsDark()
		m.sty = styles.NewMode(msg.Color, &isDark)
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

	if m.lastErr != nil && m.missionObj == nil && !isNoMissionError(m.lastErr) {
		return tea.NewView(fmt.Sprintf("Dashboard error: %v\n\nPress q to quit.", m.lastErr))
	}

	var lines []string
	lines = append(lines, m.renderHeader()...)
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
	for i := range bodyHeight {
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

	for len(lines) < m.height {
		lines = append(lines, "")
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}

	return tea.NewView(strings.Join(lines, "\n"))
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
		}
	}

	ms := m.missionObj
	tc := m.summary.TaskCounts
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

	done := tc.Done + tc.Integrated + tc.Accepted
	blocked := tc.Blocked + tc.Failed
	metricSegments := []string{
		m.renderMetric("Tasks", fmt.Sprintf("%d/%d complete", done, tc.Total)),
		m.renderMetric("Workers", fmt.Sprintf("%d active", activeRuns)),
	}
	if tc.Running > 0 {
		metricSegments = append(metricSegments, m.renderMetric("Running", fmt.Sprintf("%d now", tc.Running)))
	}
	if tc.Ready > 0 {
		metricSegments = append(metricSegments, m.renderMetric("Ready", fmt.Sprintf("%d queued", tc.Ready)))
	}
	if tc.AwaitingReview > 0 {
		metricSegments = append(metricSegments, m.renderMetric("Review", fmt.Sprintf("%d waiting", tc.AwaitingReview)))
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
		m.renderMetric("Command center", "operator view"),
		"q:quit",
		"r:refresh",
		"tab:switch pane",
		"shift+tab:back",
		"j/k:scroll",
		"1-4:jump to pane",
	}
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
	meta := "tab to focus"
	if focused {
		indicator = "▸"
		headStyle = m.sty.Panel.HeaderActive
		metaStyle = m.sty.Panel.HeaderMeta.Bold(true)
		meta = "ACTIVE • j/k scroll"
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
