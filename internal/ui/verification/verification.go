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
		if normalizeFreshness(e.Freshness) == "stale" {
			stale++
		}
		switch normalizeStatus(e.Status) {
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

// Summary returns an attention-first summary for the workflow rail.
func (s *State) Summary() string {
	_, pass, fail, stale, inProgress := s.Counts()
	switch {
	case fail > 0:
		return fmt.Sprintf("%d fail · %d running · %d stale · %d pass", fail, inProgress, stale, pass)
	case inProgress > 0:
		return fmt.Sprintf("%d running · %d stale · %d pass", inProgress, stale, pass)
	case stale > 0:
		return fmt.Sprintf("%d stale · %d pass", stale, pass)
	case pass > 0:
		return fmt.Sprintf("%d pass", pass)
	default:
		return ""
	}
}

// Focus returns the most urgent verification entry.
func (s *State) Focus() *Entry {
	for i := range s.Entries {
		if normalizeStatus(s.Entries[i].Status) == "fail" {
			return &s.Entries[i]
		}
	}
	for i := range s.Entries {
		if normalizeStatus(s.Entries[i].Status) == "in_progress" {
			return &s.Entries[i]
		}
	}
	for i := range s.Entries {
		if normalizeFreshness(s.Entries[i].Freshness) == "stale" {
			return &s.Entries[i]
		}
	}
	if len(s.Entries) == 0 {
		return nil
	}
	return &s.Entries[0]
}

// Next returns the next command that should likely run after the focus entry.
func (s *State) Next() *Entry {
	for i := range s.Entries {
		status := normalizeStatus(s.Entries[i].Status)
		if status == "in_progress" {
			continue
		}
		if status == "fail" || normalizeFreshness(s.Entries[i].Freshness) == "stale" {
			return &s.Entries[i]
		}
	}
	return nil
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
		if normalizeFreshness(e.Freshness) == "stale" {
			hasStale = true
		}
		switch normalizeStatus(e.Status) {
		case "fail":
			hasFail = true
			allPass = false
		case "in_progress":
			hasInProgress = true
			allPass = false
		default:
			if normalizeStatus(e.Status) != "pass" {
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
