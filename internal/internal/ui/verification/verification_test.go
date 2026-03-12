package verification

import (
	"testing"

	"github.com/fugue-labs/gollem/ext/codetool"
)

func TestFromToolStateConvertsEntries(t *testing.T) {
	snapshot := codetool.VerificationState{
		Entries: []codetool.VerificationEntry{
			{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh", Summary: "ok"},
			{ID: "V2", Command: "go build ./...", Status: "fail", Freshness: "stale", StaleBy: "edit main.go"},
		},
	}
	state := FromToolState(snapshot)
	if len(state.Entries) != 2 {
		t.Fatalf("entries=%d, want 2", len(state.Entries))
	}
	if state.Entries[0].ID != "V1" || state.Entries[0].Status != "pass" || state.Entries[0].Freshness != "fresh" {
		t.Fatalf("V1: %+v", state.Entries[0])
	}
	if state.Entries[1].StaleBy != "edit main.go" || state.Entries[1].Freshness != "stale" {
		t.Fatalf("V2: %+v", state.Entries[1])
	}
}

func TestCountsReturnsCorrectTotals(t *testing.T) {
	state := State{Entries: []Entry{
		{ID: "V1", Status: "pass", Freshness: "fresh"},
		{ID: "V2", Status: "fail", Freshness: "stale"},
		{ID: "V3", Status: "in_progress", Freshness: "fresh"},
	}}
	total, pass, fail, stale, inProgress := state.Counts()
	if total != 3 {
		t.Fatalf("total=%d", total)
	}
	if pass != 1 {
		t.Fatalf("pass=%d", pass)
	}
	if fail != 1 {
		t.Fatalf("fail=%d", fail)
	}
	if stale != 1 {
		t.Fatalf("stale=%d", stale)
	}
	if inProgress != 1 {
		t.Fatalf("inProgress=%d", inProgress)
	}
}

func TestBadgeReflectsWorstStatus(t *testing.T) {
	tests := []struct {
		name    string
		entries []Entry
		want    string
	}{
		{"empty", nil, "?"},
		{"all pass fresh", []Entry{{Status: "pass", Freshness: "fresh"}}, "✓"},
		{"all pass stale", []Entry{{Status: "pass", Freshness: "stale"}}, "✓*"},
		{"any fail", []Entry{{Status: "pass", Freshness: "fresh"}, {Status: "fail", Freshness: "fresh"}}, "✗"},
		{"fail stale", []Entry{{Status: "fail", Freshness: "stale"}}, "✗*"},
		{"in progress", []Entry{{Status: "in_progress", Freshness: "fresh"}}, "…"},
		{"in progress stale", []Entry{{Status: "in_progress", Freshness: "stale"}}, "…*"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := State{Entries: tt.entries}
			if got := s.Badge(); got != tt.want {
				t.Fatalf("Badge()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasEntriesReturnsFalseWhenEmpty(t *testing.T) {
	s := State{}
	if s.HasEntries() {
		t.Fatal("expected false for empty state")
	}
	s.Entries = []Entry{{ID: "V1", Status: "pass"}}
	if !s.HasEntries() {
		t.Fatal("expected true for non-empty state")
	}
}

func TestMarkAllStaleMarksFreshEntriesOnly(t *testing.T) {
	s := State{Entries: []Entry{
		{ID: "V1", Status: "pass", Freshness: "fresh"},
		{ID: "V2", Status: "fail", Freshness: "stale", StaleBy: "original reason"},
		{ID: "V3", Status: "pass", Freshness: "fresh"},
	}}

	s.MarkAllStale("edit main.go")

	for _, e := range s.Entries {
		if e.Freshness != "stale" {
			t.Fatalf("entry %s freshness=%q, want stale", e.ID, e.Freshness)
		}
	}
	// V2 was already stale — its reason should be preserved.
	if s.Entries[1].StaleBy != "original reason" {
		t.Fatalf("V2 StaleBy=%q, want preserved original reason", s.Entries[1].StaleBy)
	}
	// V1 and V3 should have the new reason.
	if s.Entries[0].StaleBy != "edit main.go" {
		t.Fatalf("V1 StaleBy=%q", s.Entries[0].StaleBy)
	}
	if s.Entries[2].StaleBy != "edit main.go" {
		t.Fatalf("V3 StaleBy=%q", s.Entries[2].StaleBy)
	}
}

func TestMarkAllStaleNoopOnEmpty(t *testing.T) {
	s := State{}
	s.MarkAllStale("edit")
	if s.HasEntries() {
		t.Fatal("expected no entries after MarkAllStale on empty state")
	}
}
