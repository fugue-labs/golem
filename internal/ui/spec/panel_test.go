package spec

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func panelView(p Panel) string {
	return p.View().Content
}

func updatePanel(p Panel, msg tea.Msg) Panel {
	m, _ := p.Update(msg)
	return m.(Panel)
}

func gateNames(gates []Gate) []string {
	names := make([]string, len(gates))
	for i, g := range gates {
		names[i] = g.Name
	}
	return names
}

func TestPanelRenderEmpty(t *testing.T) {
	p := NewPanel()
	view := panelView(p)
	if !strings.Contains(view, "No spec loaded") {
		t.Errorf("empty panel should show 'No spec loaded', got: %q", view)
	}
}

func TestPanelRenderWithSpec(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, LoadSpecMsg{FilePath: "design.md"})
	view := panelView(p)

	for _, want := range []string{"Spec", "Reviewing spec", "0/3 gates", "Next: approve Spec Approval", "File: design.md"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in view, got: %q", want, view)
		}
	}
}

func TestPanelRenderGateIcons(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, LoadSpecMsg{FilePath: "spec.md"})

	view := panelView(p)
	if strings.Count(view, "○") != 1 {
		t.Errorf("expected 1 pending gate icon while waiting on first approval, got view: %q", view)
	}
	if !strings.Contains(view, "Gate: Spec Approval") {
		t.Fatalf("expected focused gate label, got: %q", view)
	}

	p = updatePanel(p, AdvanceGateMsg{Name: "Spec Approval"})
	p = updatePanel(p, SetPhaseMsg{Phase: PhaseApproved})
	view = panelView(p)
	if !strings.Contains(view, "✓") {
		t.Error("expected check icon after advancing gate")
	}
	if strings.Contains(view, "Gate: Task Decomposition") {
		t.Fatalf("task decomposition gate should stay secondary until the workflow is waiting on it, got: %q", view)
	}
}

func TestPanelRenderAllGatesPassed(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, LoadSpecMsg{FilePath: "spec.md"})
	p = updatePanel(p, AdvanceGateMsg{Name: "Spec Approval"})
	p = updatePanel(p, AdvanceGateMsg{Name: "Task Decomposition"})
	p = updatePanel(p, AdvanceGateMsg{Name: "Final Diff Review"})

	view := panelView(p)
	if !strings.Contains(view, "3/3 gates") {
		t.Errorf("expected '3/3 gates', got: %q", view)
	}
	if strings.Contains(view, "○") {
		t.Error("no pending icons should remain when all gates passed")
	}
}

func TestPanelLoadSpec(t *testing.T) {
	p := NewPanel()
	s := p.State()
	if s.IsActive() {
		t.Error("panel should not be active before loading")
	}

	p = updatePanel(p, LoadSpecMsg{FilePath: "requirements/auth.md"})
	state := p.State()
	if !state.IsActive() {
		t.Error("panel should be active after loading spec")
	}
	if state.FilePath != "requirements/auth.md" {
		t.Errorf("expected file path 'requirements/auth.md', got %q", state.FilePath)
	}
	if state.Phase != PhaseDraft {
		t.Errorf("expected initial phase 'draft', got %q", state.Phase)
	}
	if len(state.Gates) != 3 {
		t.Fatalf("expected 3 default gates, got %d", len(state.Gates))
	}
}

func TestPanelLoadSpecDisplaysGateNames(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, LoadSpecMsg{FilePath: "spec.md"})
	view := panelView(p)

	if !strings.Contains(view, "Spec Approval") {
		t.Fatalf("expected active gate in view, got: %q", view)
	}
	for _, name := range []string{"Task Decomposition", "Final Diff Review"} {
		if !strings.Contains(strings.Join(gateNames(p.State().Gates), "\n"), name) {
			t.Errorf("expected gate %q in state, got: %v", name, gateNames(p.State().Gates))
		}
	}
}

func TestPanelVisibleGatesArePhaseAware(t *testing.T) {
	s := New("spec.md")
	if got := len(s.VisibleGates()); got != 1 {
		t.Fatalf("draft visible gates=%d, want 1", got)
	}

	s.AdvanceGate("Spec Approval")
	s.SetPhase(PhaseImplementing)
	s.SetTaskProgress(2, 5)
	visible := s.VisibleGates()
	if len(visible) != 1 || visible[0].Name != "Spec Approval" {
		t.Fatalf("implementing visible gates=%v, want only passed approvals", visible)
	}

	s.SetPhase(PhaseReviewing)
	visible = s.VisibleGates()
	if len(visible) != 2 || visible[1].Name != "Final Diff Review" {
		t.Fatalf("reviewing visible gates=%v, want final diff review surfaced", visible)
	}
}

func TestPanelReloadSpec(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, LoadSpecMsg{FilePath: "old.md"})
	p = updatePanel(p, AdvanceGateMsg{Name: "Spec Approval"})
	p = updatePanel(p, SetPhaseMsg{Phase: PhaseApproved})

	p = updatePanel(p, LoadSpecMsg{FilePath: "new.md"})
	state := p.State()
	if state.FilePath != "new.md" {
		t.Errorf("expected file path 'new.md', got %q", state.FilePath)
	}
	if state.Phase != PhaseDraft {
		t.Errorf("expected phase to reset to 'draft', got %q", state.Phase)
	}
	for _, g := range state.Gates {
		if g.Status != "pending" {
			t.Errorf("expected gate %q to be pending after reload, got %q", g.Name, g.Status)
		}
	}
}

func TestPanelWorkflowProgression(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, LoadSpecMsg{FilePath: "spec.md"})

	p = updatePanel(p, AdvanceGateMsg{Name: "Spec Approval"})
	p = updatePanel(p, SetPhaseMsg{Phase: PhaseApproved})
	view := panelView(p)
	for _, want := range []string{"Spec approved", "1/3 gates", "Next: decompose approved spec into tasks"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q after approval, got: %q", want, view)
		}
	}

	p = updatePanel(p, SetPhaseMsg{Phase: PhaseDecomposed})
	p = updatePanel(p, SetProgressMsg{Completed: 0, Total: 5})
	view = panelView(p)
	for _, want := range []string{"Tasks extracted", "Tasks: 0/5", "Next: approve Task Decomposition"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q after decomposition, got: %q", want, view)
		}
	}

	p = updatePanel(p, AdvanceGateMsg{Name: "Task Decomposition"})
	p = updatePanel(p, SetPhaseMsg{Phase: PhaseAccepted})
	view = panelView(p)
	for _, want := range []string{"Plan accepted", "2/3 gates", "Next: start implementation"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q after acceptance, got: %q", want, view)
		}
	}

	p = updatePanel(p, SetPhaseMsg{Phase: PhaseImplementing})
	p = updatePanel(p, SetProgressMsg{Completed: 3, Total: 5})
	view = panelView(p)
	for _, want := range []string{"Implementing", "Tasks: 3/5", "Next: finish implementation (2 remaining)"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q during implementation, got: %q", want, view)
		}
	}
	if strings.Contains(view, "Final Diff Review") {
		t.Fatalf("implementation phase should not foreground final review yet, got: %q", view)
	}

	p = updatePanel(p, SetProgressMsg{Completed: 5, Total: 5})
	view = panelView(p)
	if !strings.Contains(view, "Next: start Final Diff Review") {
		t.Fatalf("expected final review handoff after implementation, got: %q", view)
	}

	p = updatePanel(p, SetPhaseMsg{Phase: PhaseReviewing})
	view = panelView(p)
	for _, want := range []string{"Final review", "2/3 gates", "Tasks: 5/5", "Next: approve Final Diff Review"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in review phase, got: %q", want, view)
		}
	}

	p = updatePanel(p, AdvanceGateMsg{Name: "Final Diff Review"})
	p = updatePanel(p, SetPhaseMsg{Phase: PhaseComplete})
	view = panelView(p)
	for _, want := range []string{"Complete", "3/3 gates", "Next: keep spec and implementation aligned"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q after completion, got: %q", want, view)
		}
	}
}

func TestPanelTaskProgressNotShownWhenZero(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, LoadSpecMsg{FilePath: "spec.md"})
	view := panelView(p)
	if strings.Contains(view, "Tasks:") {
		t.Error("task progress should not appear when total is 0")
	}
}

func TestPanelTaskProgressUpdates(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, LoadSpecMsg{FilePath: "spec.md"})
	p = updatePanel(p, SetProgressMsg{Completed: 2, Total: 8})

	state := p.State()
	completed, total := state.Progress()
	if completed != 2 || total != 8 {
		t.Errorf("expected progress 2/8, got %d/%d", completed, total)
	}

	view := panelView(p)
	if !strings.Contains(view, "Tasks: 2/8") {
		t.Errorf("expected 'Tasks: 2/8' in view, got: %q", view)
	}
}

func TestPanelGateValidation(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, LoadSpecMsg{FilePath: "spec.md"})

	gates := []string{"Spec Approval", "Task Decomposition", "Final Diff Review"}
	for i, name := range gates {
		p = updatePanel(p, AdvanceGateMsg{Name: name})
		state := p.State()
		if state.Gates[i].Status != "passed" {
			t.Errorf("gate %q should be passed, got %q", name, state.Gates[i].Status)
		}
		summary := state.GateSummary()
		expected := fmt.Sprintf("%d/3 gates", i+1)
		if summary != expected {
			t.Errorf("after advancing %q: expected %q, got %q", name, expected, summary)
		}
	}
}

func TestPanelGateCaseInsensitive(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, LoadSpecMsg{FilePath: "spec.md"})
	p = updatePanel(p, AdvanceGateMsg{Name: "spec approval"})
	if p.State().Gates[0].Status != "passed" {
		t.Error("gate advance should be case-insensitive")
	}
}

func TestPanelNonexistentGate(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, LoadSpecMsg{FilePath: "spec.md"})
	p = updatePanel(p, AdvanceGateMsg{Name: "nonexistent"})

	for _, g := range p.State().Gates {
		if g.Status != "pending" {
			t.Errorf("no gate should be advanced for nonexistent name, but %q is %q", g.Name, g.Status)
		}
	}
}

func TestPanelPhaseLabelsInView(t *testing.T) {
	tests := []struct {
		phase Phase
		label string
	}{{PhaseDraft, "Reviewing spec"}, {PhaseApproved, "Spec approved"}, {PhaseDecomposed, "Tasks extracted"}, {PhaseAccepted, "Plan accepted"}, {PhaseImplementing, "Implementing"}, {PhaseReviewing, "Final review"}, {PhaseComplete, "Complete"}}
	for _, tt := range tests {
		p := NewPanel()
		p = updatePanel(p, LoadSpecMsg{FilePath: "spec.md"})
		p = updatePanel(p, SetPhaseMsg{Phase: tt.phase})
		view := panelView(p)
		if !strings.Contains(view, tt.label) {
			t.Errorf("phase %q: expected label %q in view, got: %q", tt.phase, tt.label, view)
		}
	}
}

func TestPanelWindowResize(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, tea.WindowSizeMsg{Width: 60, Height: 30})
	if p.width != 60 || p.height != 30 {
		t.Errorf("expected 60x30, got %dx%d", p.width, p.height)
	}
}
