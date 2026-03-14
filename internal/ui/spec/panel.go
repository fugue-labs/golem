package spec

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Panel is a standalone bubbletea Model for the spec-driven development panel.
// It wraps State and provides Init/Update/View for independent rendering and testing.
type Panel struct {
	state  State
	width  int
	height int
}

// Messages for updating the Panel.

// LoadSpecMsg loads a spec file into the panel.
type LoadSpecMsg struct{ FilePath string }

// AdvanceGateMsg marks a gate as passed.
type AdvanceGateMsg struct{ Name string }

// SetPhaseMsg updates the workflow phase.
type SetPhaseMsg struct{ Phase Phase }

// SetProgressMsg updates task progress counts.
type SetProgressMsg struct{ Completed, Total int }

// NewPanel creates a new Panel with default dimensions.
func NewPanel() Panel {
	return Panel{width: 40, height: 20}
}

// State returns the current spec state.
func (p Panel) State() State { return p.state }

// Init implements tea.Model.
func (p Panel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (p Panel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
	case LoadSpecMsg:
		p.state = New(msg.FilePath)
	case AdvanceGateMsg:
		p.state.AdvanceGate(msg.Name)
	case SetPhaseMsg:
		p.state.SetPhase(msg.Phase)
	case SetProgressMsg:
		p.state.SetTaskProgress(msg.Completed, msg.Total)
	}
	return p, nil
}

// View implements tea.Model.
func (p Panel) View() tea.View {
	if !p.state.IsActive() {
		return tea.NewView("No spec loaded.")
	}

	var b strings.Builder
	header := "Spec"
	if summary := p.state.PanelSummary(max(12, p.width-8)); summary != "" {
		header += " · " + summary
	}
	b.WriteString(header)
	b.WriteByte('\n')
	b.WriteString(fmt.Sprintf(" File %s\n", p.state.FileLabel(max(12, p.width-6))))
	b.WriteString(" Gates\n")

	for _, g := range p.state.Gates {
		icon := "○"
		if g.Status == "passed" {
			icon = "✓"
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", icon, g.Name))
	}

	completed, total := p.state.Progress()
	if total > 0 {
		b.WriteString(fmt.Sprintf(" Tasks %d/%d complete\n", completed, total))
	}

	return tea.NewView(strings.TrimRight(b.String(), "\n"))
}
