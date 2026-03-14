package verification

import (
	"fmt"
	"strings"

	"github.com/fugue-labs/gollem/ext/codetool"
)

// Entry mirrors gollem's verification tool entry for TUI display.
type Entry struct {
	ID        string `json:"id"`
	Command   string `json:"command"`
	Status    string `json:"status"`
	Freshness string `json:"freshness"`
	Summary   string `json:"summary,omitempty"`
	StaleBy   string `json:"stale_by,omitempty"`
}

// State tracks the agent's current verification results for TUI display.
type State struct {
	Entries []Entry
}

// FromToolState converts the gollem verification state snapshot to TUI state.
func FromToolState(snapshot codetool.VerificationState) State {
	entries := make([]Entry, len(snapshot.Entries))
	for i, e := range snapshot.Entries {
		entries[i] = Entry{
			ID:        strings.TrimSpace(e.ID),
			Command:   strings.TrimSpace(e.Command),
			Status:    normalizeStatus(e.Status),
			Freshness: normalizeFreshness(e.Freshness),
			Summary:   strings.TrimSpace(e.Summary),
			StaleBy:   strings.TrimSpace(e.StaleBy),
		}
	}
	return State{Entries: entries}
}

// HasEntries returns true if any verification entries exist.
func (s *State) HasEntries() bool { return len(s.Entries) > 0 }

// MarkAllStale marks all fresh entries as stale with the given reason.
func (s *State) MarkAllStale(reason string) {
	for i := range s.Entries {
		if s.Entries[i].Freshness != "stale" {
			s.Entries[i].Freshness = "stale"
			s.Entries[i].StaleBy = reason
		}
	}
}

// Counts returns verification summary counts.
func (s *State) Counts() (total, pass, fail, stale, inProgress int) {
	total = len(s.Entries)
	for _, e := range s.Entries {
		if e.Freshness == "stale" {
			stale++
		}
		switch e.Status {
		case "pass":
			pass++
		case "fail":
			fail++
		case "in_progress":
			inProgress++
		}
	}
	return
}

// Summary returns a width-aware verification summary for panel headers.
func (s *State) Summary(width int) string {
	total, pass, fail, stale, inProgress := s.Counts()
	if total == 0 {
		return "0 checks"
	}
	if width < 16 {
		return fmt.Sprintf("%d✓ %d✗", pass, fail)
	}
	if width < 28 {
		return fmt.Sprintf("%d✓ %d✗ %d◐ %d*", pass, fail, inProgress, stale)
	}
	parts := []string{fmt.Sprintf("%d pass", pass), fmt.Sprintf("%d fail", fail)}
	if inProgress > 0 {
		parts = append(parts, fmt.Sprintf("%d running", inProgress))
	}
	if stale > 0 {
		parts = append(parts, fmt.Sprintf("%d stale", stale))
	}
	return strings.Join(parts, " · ")
}

// Badge returns a compact status indicator for the status bar.
func (s *State) Badge() string {
	if len(s.Entries) == 0 {
		return "?"
	}
	hasFail := false
	hasStale := false
	hasInProgress := false
	allPass := true
	for _, e := range s.Entries {
		if e.Freshness == "stale" {
			hasStale = true
		}
		switch e.Status {
		case "fail":
			hasFail = true
			allPass = false
		case "in_progress":
			hasInProgress = true
			allPass = false
		default:
			if e.Status != "pass" {
				allPass = false
			}
		}
	}
	switch {
	case hasFail:
		if hasStale {
			return "✗*"
		}
		return "✗"
	case hasInProgress:
		if hasStale {
			return "…*"
		}
		return "…"
	case allPass && !hasStale:
		return "✓"
	case allPass && hasStale:
		return "✓*"
	default:
		if hasStale {
			return "?*"
		}
		return "?"
	}
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pass", "passed":
		return "pass"
	case "fail", "failed":
		return "fail"
	case "in_progress", "in-progress", "in progress", "running":
		return "in_progress"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func normalizeFreshness(freshness string) string {
	switch strings.ToLower(strings.TrimSpace(freshness)) {
	case "stale":
		return "stale"
	default:
		return "fresh"
	}
}
