package spec

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Phase represents the current phase of the spec-driven workflow.
type Phase string

const (
	PhaseDraft        Phase = "draft"        // Spec loaded, awaiting agent review
	PhaseApproved     Phase = "approved"     // User approved the spec analysis
	PhaseDecomposed   Phase = "decomposed"   // Tasks extracted, awaiting user review
	PhaseAccepted     Phase = "accepted"     // User accepted task decomposition
	PhaseImplementing Phase = "implementing" // Agent is implementing tasks
	PhaseReviewing    Phase = "reviewing"    // Final diff review in progress
	PhaseComplete     Phase = "complete"     // All tasks done, spec updated
)

// Gate represents a human approval checkpoint in the workflow.
type Gate struct {
	Name   string `json:"name"`
	Status string `json:"status"` // pending, passed, failed
}

// State tracks the spec-driven development workflow for TUI display.
type State struct {
	FilePath  string `json:"file_path"`
	Phase     Phase  `json:"phase"`
	Gates     []Gate `json:"gates"`
	TaskCount int    `json:"task_count"`
	Completed int    `json:"completed"`
}

// DefaultGates returns the three standard gates for spec-driven development.
func DefaultGates() []Gate {
	return []Gate{
		{Name: "Spec Approval", Status: "pending"},
		{Name: "Task Decomposition", Status: "pending"},
		{Name: "Final Diff Review", Status: "pending"},
	}
}

// New creates a new spec state from a file path.
func New(filePath string) State {
	return State{
		FilePath: filePath,
		Phase:    PhaseDraft,
		Gates:    DefaultGates(),
	}
}

// IsActive returns true if a spec workflow is in progress.
func (s *State) IsActive() bool {
	return s.FilePath != ""
}

// PhaseLabel returns a human-readable label for the current phase.
func (s *State) PhaseLabel() string {
	switch s.Phase {
	case PhaseDraft:
		return "Reviewing spec"
	case PhaseApproved:
		return "Spec approved"
	case PhaseDecomposed:
		return "Tasks extracted"
	case PhaseAccepted:
		return "Plan accepted"
	case PhaseImplementing:
		return "Implementing"
	case PhaseReviewing:
		return "Final review"
	case PhaseComplete:
		return "Complete"
	default:
		return string(s.Phase)
	}
}

// GateSummary returns a compact summary of gate statuses.
func (s *State) GateSummary() string {
	passed, total := 0, len(s.Gates)
	for _, g := range s.Gates {
		if g.Status == "passed" {
			passed++
		}
	}
	return fmt.Sprintf("%d/%d gates", passed, total)
}

func (s *State) pendingGate(name string) string {
	for _, g := range s.Gates {
		if strings.EqualFold(g.Name, name) && g.Status != "passed" {
			return g.Name
		}
	}
	return ""
}

// WaitingGateName returns the approval gate that is actively blocking the phase.
func (s *State) WaitingGateName() string {
	switch s.Phase {
	case PhaseDraft:
		return s.pendingGate("Spec Approval")
	case PhaseDecomposed:
		return s.pendingGate("Task Decomposition")
	case PhaseReviewing:
		return s.pendingGate("Final Diff Review")
	default:
		return ""
	}
}

// FocusGateName returns the gate currently worth highlighting in the rail.
func (s *State) FocusGateName() string {
	return s.WaitingGateName()
}

// VisibleGates returns the gates that should be shown for the current phase.
func (s *State) VisibleGates() []Gate {
	if len(s.Gates) == 0 {
		return nil
	}
	if s.Phase == PhaseComplete {
		visible := make([]Gate, len(s.Gates))
		copy(visible, s.Gates)
		return visible
	}
	waiting := s.WaitingGateName()
	visible := make([]Gate, 0, len(s.Gates))
	for _, g := range s.Gates {
		if g.Status == "passed" || (waiting != "" && strings.EqualFold(g.Name, waiting)) {
			visible = append(visible, g)
		}
	}
	return visible
}

// Headline returns a concise workflow summary emphasizing the current bottleneck.
func (s *State) Headline() string {
	phase := s.PhaseLabel()
	if gate := s.WaitingGateName(); gate != "" {
		return fmt.Sprintf("%s · awaiting %s", phase, gate)
	}
	completed, total := s.Progress()
	switch s.Phase {
	case PhaseApproved:
		return phase + " · decomposing tasks"
	case PhaseAccepted, PhaseImplementing, PhaseReviewing, PhaseComplete:
		if total > 0 {
			return fmt.Sprintf("%s · tasks %d/%d", phase, completed, total)
		}
	}
	if total > 0 {
		return fmt.Sprintf("%s · tasks %d/%d", phase, completed, total)
	}
	return fmt.Sprintf("%s · %s", phase, s.GateSummary())
}

// NextAction returns the most useful operator-facing next step.
func (s *State) NextAction() string {
	if gate := s.WaitingGateName(); gate != "" {
		return "Next: approve " + gate
	}
	completed, total := s.Progress()
	switch s.Phase {
	case PhaseApproved:
		return "Next: decompose approved spec into tasks"
	case PhaseAccepted:
		return "Next: start implementation"
	case PhaseImplementing:
		if total > 0 && completed < total {
			return fmt.Sprintf("Next: finish implementation (%d remaining)", total-completed)
		}
		if total > 0 {
			return "Next: start Final Diff Review"
		}
		return "Next: continue implementation"
	case PhaseReviewing, PhaseComplete:
		return "Next: keep spec and implementation aligned"
	default:
		if s.IsActive() {
			return "Next: keep spec and implementation aligned"
		}
		return ""
	}
}

// FileLabel returns a compact file label for the rail.
func (s *State) FileLabel() string {
	if strings.TrimSpace(s.FilePath) == "" {
		return ""
	}
	base := filepath.Base(s.FilePath)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return s.FilePath
	}
	return "File: " + base
}

// AdvanceGate marks the named gate as passed and advances the phase.
func (s *State) AdvanceGate(name string) bool {
	for i := range s.Gates {
		if strings.EqualFold(s.Gates[i].Name, name) {
			s.Gates[i].Status = "passed"
			return true
		}
	}
	return false
}

// SetPhase updates the workflow phase.
func (s *State) SetPhase(phase Phase) {
	s.Phase = phase
}

// SetTaskProgress updates the task count and completed count.
func (s *State) SetTaskProgress(completed, total int) {
	s.Completed = completed
	s.TaskCount = total
}

// Progress returns completed and total task counts.
func (s *State) Progress() (completed, total int) {
	return s.Completed, s.TaskCount
}

// NormalizePhase converts a string to a valid Phase, defaulting to draft.
func NormalizePhase(phase string) Phase {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "draft":
		return PhaseDraft
	case "approved":
		return PhaseApproved
	case "decomposed":
		return PhaseDecomposed
	case "accepted":
		return PhaseAccepted
	case "implementing":
		return PhaseImplementing
	case "reviewing":
		return PhaseReviewing
	case "complete", "completed", "done":
		return PhaseComplete
	default:
		return PhaseDraft
	}
}
