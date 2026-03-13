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

// --- 1. Spec panel rendering ---

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

	if !strings.Contains(view, "Spec") {
		t.Error("expected 'Spec' header in view")
	}
	if !strings.Contains(view, "Reviewing spec") {
		t.Errorf("expected phase label 'Reviewing spec', got: %q", view)
	}
	if !strings.Contains(view, "0/3 gates") {
		t.Errorf("expected '0/3 gates' summary, got: %q", view)
	}
}

func TestPanelRenderGateIcons(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, LoadSpecMsg{FilePath: "spec.md"})

	view := panelView(p)
	// All gates pending → hollow icons.
	if strings.Count(view, "○") != 3 {
		t.Errorf("expected 3 pending gate icons, got view: %q", view)
	}

	// Advance one gate.
	p = updatePanel(p, AdvanceGateMsg{Name: "Spec Approval"})
	view = panelView(p)
	if !strings.Contains(view, "✓") {
		t.Error("expected check icon after advancing gate")
	}
	if strings.Count(view, "○") != 2 {
		t.Errorf("expected 2 remaining pending icons, got view: %q", view)
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

// --- 2. Spec loading and display ---

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

	gates := []string{"Spec Approval", "Task Decomposition", "Final Diff Review"}
	for _, name := range gates {
		if !strings.Contains(view, name) {
			t.Errorf("expected gate %q in view, got: %q", name, view)
		}
	}
}

func TestPanelReloadSpec(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, LoadSpecMsg{FilePath: "old.md"})
	p = updatePanel(p, AdvanceGateMsg{Name: "Spec Approval"})
	p = updatePanel(p, SetPhaseMsg{Phase: PhaseApproved})

	// Reload with a new spec — state should reset.
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

// --- 3. Spec-to-task conversion flow ---

func TestPanelWorkflowProgression(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, LoadSpecMsg{FilePath: "spec.md"})

	// Phase 1: Draft → Approved.
	p = updatePanel(p, AdvanceGateMsg{Name: "Spec Approval"})
	p = updatePanel(p, SetPhaseMsg{Phase: PhaseApproved})
	view := panelView(p)
	if !strings.Contains(view, "Spec approved") {
		t.Errorf("expected 'Spec approved' after approval, got: %q", view)
	}
	if !strings.Contains(view, "1/3 gates") {
		t.Errorf("expected '1/3 gates', got: %q", view)
	}

	// Phase 2: Approved → Decomposed (task extraction).
	p = updatePanel(p, AdvanceGateMsg{Name: "Task Decomposition"})
	p = updatePanel(p, SetPhaseMsg{Phase: PhaseDecomposed})
	p = updatePanel(p, SetProgressMsg{Completed: 0, Total: 5})
	view = panelView(p)
	if !strings.Contains(view, "Tasks extracted") {
		t.Errorf("expected 'Tasks extracted', got: %q", view)
	}
	if !strings.Contains(view, "Tasks: 0/5") {
		t.Errorf("expected 'Tasks: 0/5', got: %q", view)
	}

	// Phase 3: Implementing — partial progress.
	p = updatePanel(p, SetPhaseMsg{Phase: PhaseImplementing})
	p = updatePanel(p, SetProgressMsg{Completed: 3, Total: 5})
	view = panelView(p)
	if !strings.Contains(view, "Implementing") {
		t.Errorf("expected 'Implementing', got: %q", view)
	}
	if !strings.Contains(view, "Tasks: 3/5") {
		t.Errorf("expected 'Tasks: 3/5', got: %q", view)
	}

	// Phase 4: Reviewing.
	p = updatePanel(p, AdvanceGateMsg{Name: "Final Diff Review"})
	p = updatePanel(p, SetPhaseMsg{Phase: PhaseReviewing})
	p = updatePanel(p, SetProgressMsg{Completed: 5, Total: 5})
	view = panelView(p)
	if !strings.Contains(view, "Final review") {
		t.Errorf("expected 'Final review', got: %q", view)
	}
	if !strings.Contains(view, "3/3 gates") {
		t.Errorf("expected '3/3 gates', got: %q", view)
	}
	if !strings.Contains(view, "Tasks: 5/5") {
		t.Errorf("expected 'Tasks: 5/5', got: %q", view)
	}

	// Phase 5: Complete.
	p = updatePanel(p, SetPhaseMsg{Phase: PhaseComplete})
	view = panelView(p)
	if !strings.Contains(view, "Complete") {
		t.Errorf("expected 'Complete', got: %q", view)
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

// --- 4. Spec validation display ---

func TestPanelGateValidation(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, LoadSpecMsg{FilePath: "spec.md"})

	// Verify each gate can be individually advanced.
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
	}{
		{PhaseDraft, "Reviewing spec"},
		{PhaseApproved, "Spec approved"},
		{PhaseDecomposed, "Tasks extracted"},
		{PhaseAccepted, "Plan accepted"},
		{PhaseImplementing, "Implementing"},
		{PhaseReviewing, "Final review"},
		{PhaseComplete, "Complete"},
	}
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

// --- Window resize ---

func TestPanelWindowResize(t *testing.T) {
	p := NewPanel()
	p = updatePanel(p, tea.WindowSizeMsg{Width: 60, Height: 30})
	if p.width != 60 || p.height != 30 {
		t.Errorf("expected 60x30, got %dx%d", p.width, p.height)
	}
}
