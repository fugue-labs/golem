package invariants

import (
	"testing"

	"github.com/fugue-labs/gollem/ext/codetool"
)

func TestFromToolStateNormalizesItems(t *testing.T) {
	state := FromToolState(codetool.InvariantsState{
	Extracted: true,
		Items: []codetool.InvariantItem{
			{ID: " I1 ", Description: " Check build ", Kind: "SOFT", Status: "Completed", Evidence: " go test ./... "},
			{ID: "I2", Description: "Review failures", Kind: "hard", Status: "UNMET"},
		},
	})

	if !state.Extracted {
		t.Fatal("expected Extracted=true after extract result")
	}
	if got := state.Items[0].ID; got != "I1" {
		t.Fatalf("first item id = %q", got)
	}
	if got := state.Items[0].Description; got != "Check build" {
		t.Fatalf("first item description = %q", got)
	}
	if got := state.Items[0].Kind; got != "soft" {
		t.Fatalf("first item kind = %q", got)
	}
	if got := state.Items[0].Status; got != "pass" {
		t.Fatalf("first item status = %q", got)
	}
	if got := state.Items[0].Evidence; got != "go test ./..." {
		t.Fatalf("first item evidence = %q", got)
	}
	if got := state.Items[1].Status; got != "fail" {
		t.Fatalf("second item status = %q", got)
	}
}
