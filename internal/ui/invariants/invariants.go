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

// Counts returns hard/soft invariant summary counts.
func (s *State) Counts() (hardTotal, hardPass, hardFail, hardUnresolved, softTotal, softPass, softFail int) {
	for _, item := range s.Items {
		if normalizeKind(item.Kind) == "soft" {
			softTotal++
			switch normalizeStatus(item.Status) {
			case "pass":
				softPass++
			case "fail":
				softFail++
			}
			continue
		}
		hardTotal++
		switch normalizeStatus(item.Status) {
		case "pass":
			hardPass++
		case "fail":
			hardFail++
		}
	}
	hardUnresolved = hardTotal - hardPass - hardFail
	return
}

// Summary returns a bottleneck-first summary for the workflow rail.
func (s *State) Summary() string {
	hardTotal, hardPass, hardFail, hardUnresolved, _, _, _ := s.Counts()
	if hardTotal == 0 {
		return "Hard 0/0 pass"
	}
	switch {
	case hardFail > 0:
		return fmt.Sprintf("Hard %d fail · %d open · %d/%d pass", hardFail, hardUnresolved, hardPass, hardTotal)
	case hardUnresolved > 0:
		return fmt.Sprintf("Hard %d open · %d/%d pass", hardUnresolved, hardPass, hardTotal)
	default:
		return fmt.Sprintf("Hard %d/%d pass", hardPass, hardTotal)
	}
}

// Focus returns the first blocking or in-progress invariant that needs attention.
func (s *State) Focus() *Item {
	for i := range s.Items {
		if normalizeStatus(s.Items[i].Status) == "fail" {
			return &s.Items[i]
		}
	}
	for i := range s.Items {
		if normalizeStatus(s.Items[i].Status) == "in_progress" {
			return &s.Items[i]
		}
	}
	for i := range s.Items {
		if normalizeStatus(s.Items[i].Status) != "pass" {
			return &s.Items[i]
		}
	}
	if len(s.Items) == 0 {
		return nil
	}
	return &s.Items[0]
}

// Next returns the next unresolved invariant after the focused one.
func (s *State) Next() *Item {
	for i := range s.Items {
		if normalizeStatus(s.Items[i].Status) == "unknown" {
			return &s.Items[i]
		}
	}
	for i := range s.Items {
		if normalizeStatus(s.Items[i].Status) == "in_progress" {
			return &s.Items[i]
		}
	}
	return nil
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
