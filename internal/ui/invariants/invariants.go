package invariants

import (
	"fmt"
	"strings"

	"github.com/fugue-labs/gollem/ext/codetool"
)

// Item mirrors gollem's invariants tool item for TUI display.
type Item struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Kind        string `json:"kind,omitempty"`
	Status      string `json:"status,omitempty"`
	Evidence    string `json:"evidence,omitempty"`
}

// State tracks the agent's current invariant checklist for TUI display.
type State struct {
	Items     []Item
	Extracted bool
}

func FromToolState(snapshot codetool.InvariantsState) State {
	items := make([]Item, len(snapshot.Items))
	for i, item := range snapshot.Items {
		items[i] = normalizeItem(Item{
			ID:          item.ID,
			Description: item.Description,
			Kind:        item.Kind,
			Status:      item.Status,
			Evidence:    item.Evidence,
		})
	}
	return State{Items: items, Extracted: snapshot.Extracted}
}

func (s *State) HasItems() bool { return s.Extracted || len(s.Items) > 0 }

// Summary returns a width-aware hard/soft invariant summary for panel headers.
func (s *State) Summary(width int) string {
	hardPass, hardFail, hardUnresolved, softTotal, softPass, softFail := 0, 0, 0, 0, 0, 0
	_, hardPass, hardFail, hardUnresolved, softTotal, softPass, softFail = s.Counts()
	if len(s.Items) == 0 {
		if s.Extracted {
			if width < 18 {
				return "0✓ 0✗ 0?"
			}
			return "hard 0✓ 0✗ 0?"
		}
		return "pending"
	}
	if width < 18 {
		return fmt.Sprintf("%d✓ %d✗ %d?", hardPass, hardFail, hardUnresolved)
	}
	hardSummary := fmt.Sprintf("hard %d✓ %d✗ %d?", hardPass, hardFail, hardUnresolved)
	if width < 34 || softTotal == 0 {
		return hardSummary
	}
	if width < 48 {
		return hardSummary + fmt.Sprintf(" · soft %d✓/%d", softPass, softTotal)
	}
	return hardSummary + fmt.Sprintf(" · soft %d✓ %d✗", softPass, softFail)
}

// Counts returns hard/soft invariant summary counts.
func (s *State) Counts() (hardTotal, hardPass, hardFail, hardUnresolved, softTotal, softPass, softFail int) {
	for _, item := range s.Items {
		if item.Kind == "soft" {
			softTotal++
			switch item.Status {
			case "pass":
				softPass++
			case "fail":
				softFail++
			}
			continue
		}
		hardTotal++
		switch item.Status {
		case "pass":
			hardPass++
		case "fail":
			hardFail++
		}
	}
	hardUnresolved = hardTotal - hardPass - hardFail
	return
}

func normalizeKind(kind string) string {
	if strings.EqualFold(strings.TrimSpace(kind), "soft") {
		return "soft"
	}
	return "hard"
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "":
		return ""
	case "unknown", "unresolved", "todo", "not_started":
		return "unknown"
	case "in_progress", "in-progress", "in progress", "wip", "working", "active":
		return "in_progress"
	case "pass", "passed", "done", "complete", "completed", "satisfied", "ok":
		return "pass"
	case "fail", "failed", "broken", "unmet":
		return "fail"
	default:
		return "unknown"
	}
}

func normalizeItem(item Item) Item {
	item.ID = strings.TrimSpace(item.ID)
	item.Description = strings.TrimSpace(item.Description)
	item.Kind = normalizeKind(item.Kind)
	item.Status = normalizeStatus(item.Status)
	if item.Status == "" {
		item.Status = "unknown"
	}
	item.Evidence = strings.TrimSpace(item.Evidence)
	return item
}
