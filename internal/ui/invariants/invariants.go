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

type invariantCommand struct {
	Command     string `json:"command"`
	ID          string `json:"id,omitempty"`
	Status      string `json:"status,omitempty"`
	Evidence    string `json:"evidence,omitempty"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Items       []Item `json:"items,omitempty"`
}

type invariantResult struct {
	Status     string `json:"status"`
	Extracted  bool   `json:"extracted"`
	Items      []Item `json:"items,omitempty"`
	HardTotal  int    `json:"hard_total"`
	HardPass   int    `json:"hard_pass"`
	HardFail   int    `json:"hard_fail"`
	HardRemain int    `json:"hard_unresolved"`
	SoftTotal  int    `json:"soft_total"`
	SoftPass   int    `json:"soft_pass"`
	SoftFail   int    `json:"soft_fail"`
}

func (s *State) HasItems() bool { return s.Extracted || len(s.Items) > 0 }

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

func nextID(items []Item) string {
	highestID := 0
	for _, item := range items {
		var n int
		if _, err := fmt.Sscanf(item.ID, "I%d", &n); err == nil && n > highestID {
			highestID = n
		}
	}
	return fmt.Sprintf("I%d", highestID+1)
}
