package invariants

import (
	"testing"

	"github.com/fugue-labs/gollem/ext/codetool"
)

func TestFromToolStateCopiesAndCountsItems(t *testing.T) {
	state := FromToolState(codetool.InvariantsState{
		Extracted: true,
		Items: []codetool.InvariantItem{
			{ID: "I1", Description: "Commit the work.", Kind: "hard", Status: "pass", Evidence: "git commit abc123"},
			{ID: "I2", Description: "Push the work.", Kind: "hard", Status: "unknown"},
			{ID: "I3", Description: "Delight users.", Kind: "soft", Status: "pass"},
		},
	})

	if !state.Extracted {
		t.Fatalf("expected Extracted=true")
	}
	if got := len(state.Items); got != 3 {
		t.Fatalf("expected 3 items, got %d", got)
	}
	if state.Items[0].Evidence != "git commit abc123" {
		t.Fatalf("unexpected evidence: %q", state.Items[0].Evidence)
	}

	hardTotal, hardPass, hardFail, hardUnresolved, softTotal, softPass, softFail := state.Counts()
	if hardTotal != 2 || hardPass != 1 || hardFail != 0 || hardUnresolved != 1 {
		t.Fatalf("unexpected hard counts: total=%d pass=%d fail=%d unresolved=%d", hardTotal, hardPass, hardFail, hardUnresolved)
	}
	if softTotal != 1 || softPass != 1 || softFail != 0 {
		t.Fatalf("unexpected soft counts: total=%d pass=%d fail=%d", softTotal, softPass, softFail)
	}
	if got := state.Summary(50); got != "hard 1✓ 0✗ 1? · soft 1✓ 0✗" {
		t.Fatalf("Summary(50)=%q", got)
	}
}

func TestFromToolStateHandlesEmptySnapshot(t *testing.T) {
	state := FromToolState(codetool.InvariantsState{})
	if state.HasItems() {
		t.Fatal("expected no items")
	}
}
