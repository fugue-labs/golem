package invariants

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Item mirrors gollem's invariants tool item for TUI display.
type Item struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Kind        string `json:"kind,omitempty"`
	Status      string `json:"status,omitempty"`
	Evidence    string `json:"evidence,omitempty"`
}

// State tracks the agent's current invariant checklist by parsing invariants tool messages.
type State struct {
	Items         []Item
	Extracted     bool
	lastCommand   string
	prevItems     []Item
	prevExtracted bool
	hadPrev       bool
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

func cloneItems(items []Item) []Item {
	if items == nil {
		return nil
	}
	out := make([]Item, len(items))
	copy(out, items)
	return out
}

func (s *State) snapshot() {
	s.prevItems = cloneItems(s.Items)
	s.prevExtracted = s.Extracted
	s.hadPrev = true
}

func (s *State) clearSnapshot() {
	s.prevItems = nil
	s.prevExtracted = false
	s.hadPrev = false
}

func (s *State) revertSnapshot() {
	if s.hadPrev {
		s.Items = cloneItems(s.prevItems)
		s.Extracted = s.prevExtracted
	}
	s.clearSnapshot()
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

func (s *State) HandleToolCall(rawArgs string) {
	var cmd invariantCommand
	if err := json.Unmarshal([]byte(rawArgs), &cmd); err != nil {
		return
	}

	s.snapshot()
	s.lastCommand = cmd.Command

	switch cmd.Command {
	case "extract":
		s.Extracted = true
	case "update":
		for i := range s.Items {
			if s.Items[i].ID == cmd.ID {
				if cmd.Status != "" {
					s.Items[i].Status = normalizeStatus(cmd.Status)
				}
				if cmd.Evidence != "" {
					s.Items[i].Evidence = cmd.Evidence
				}
				break
			}
		}
	case "add":
		if len(cmd.Items) > 0 {
			for _, item := range cmd.Items {
				s.Items = append(s.Items, normalizeItem(item))
			}
		} else if cmd.Description != "" {
			id := strings.TrimSpace(cmd.ID)
			if id == "" {
				id = nextID(s.Items)
			}
			item := Item{
				ID:          id,
				Description: strings.TrimSpace(cmd.Description),
				Kind:        normalizeKind(cmd.Kind),
				Status:      normalizeStatus(cmd.Status),
				Evidence:    strings.TrimSpace(cmd.Evidence),
			}
			if item.Status == "" {
				item.Status = "unknown"
			}
			s.Items = append(s.Items, item)
		}
	}
}

func (s *State) HandleToolResult(result string) {
	defer s.clearSnapshot()

	var res invariantResult
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		return
	}
	if res.Items != nil {
		s.Items = make([]Item, len(res.Items))
		for i, item := range res.Items {
			s.Items[i] = normalizeItem(item)
		}
	}
	s.Extracted = s.Extracted || res.Extracted
}

func (s *State) HandleToolError() {
	s.revertSnapshot()
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
