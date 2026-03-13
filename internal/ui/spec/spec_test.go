package spec

import (
	"testing"
)

func TestNew(t *testing.T) {
	s := New("path/to/spec.md")
	if s.FilePath != "path/to/spec.md" {
		t.Errorf("expected file path 'path/to/spec.md', got %q", s.FilePath)
	}
	if s.Phase != PhaseDraft {
		t.Errorf("expected phase 'draft', got %q", s.Phase)
	}
	if len(s.Gates) != 3 {
		t.Fatalf("expected 3 gates, got %d", len(s.Gates))
	}
	for _, g := range s.Gates {
		if g.Status != "pending" {
			t.Errorf("expected gate %q status 'pending', got %q", g.Name, g.Status)
		}
	}
}

func TestIsActive(t *testing.T) {
	s := State{}
	if s.IsActive() {
		t.Error("empty state should not be active")
	}
	s = New("spec.md")
	if !s.IsActive() {
		t.Error("state with file path should be active")
	}
}

func TestAdvanceGate(t *testing.T) {
	s := New("spec.md")
	if !s.AdvanceGate("Spec Approval") {
		t.Error("should find and advance 'Spec Approval' gate")
	}
	if s.Gates[0].Status != "passed" {
		t.Errorf("expected gate status 'passed', got %q", s.Gates[0].Status)
	}
	// Case-insensitive match.
	if !s.AdvanceGate("task decomposition") {
		t.Error("should find gate case-insensitively")
	}
	// Non-existent gate.
	if s.AdvanceGate("nonexistent") {
		t.Error("should return false for non-existent gate")
	}
}

func TestGateSummary(t *testing.T) {
	s := New("spec.md")
	if got := s.GateSummary(); got != "0/3 gates" {
		t.Errorf("expected '0/3 gates', got %q", got)
	}
	s.AdvanceGate("Spec Approval")
	s.AdvanceGate("Task Decomposition")
	if got := s.GateSummary(); got != "2/3 gates" {
		t.Errorf("expected '2/3 gates', got %q", got)
	}
}

func TestSetTaskProgress(t *testing.T) {
	s := New("spec.md")
	s.SetTaskProgress(3, 5)
	completed, total := s.Progress()
	if completed != 3 || total != 5 {
		t.Errorf("expected progress 3/5, got %d/%d", completed, total)
	}
}

func TestPhaseLabel(t *testing.T) {
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
		s := State{Phase: tt.phase}
		if got := s.PhaseLabel(); got != tt.label {
			t.Errorf("phase %q: expected label %q, got %q", tt.phase, tt.label, got)
		}
	}
}

func TestNormalizePhase(t *testing.T) {
	tests := []struct {
		input    string
		expected Phase
	}{
		{"draft", PhaseDraft},
		{"APPROVED", PhaseApproved},
		{"  Decomposed  ", PhaseDecomposed},
		{"accepted", PhaseAccepted},
		{"implementing", PhaseImplementing},
		{"reviewing", PhaseReviewing},
		{"complete", PhaseComplete},
		{"completed", PhaseComplete},
		{"done", PhaseComplete},
		{"unknown", PhaseDraft},
		{"", PhaseDraft},
	}
	for _, tt := range tests {
		got := NormalizePhase(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizePhase(%q): expected %q, got %q", tt.input, tt.expected, got)
		}
	}
}
