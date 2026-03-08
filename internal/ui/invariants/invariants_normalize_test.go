package invariants

import "testing"

func TestHandleToolResultNormalizesItems(t *testing.T) {
	state := State{}
	state.HandleToolCall(`{"command":"extract"}`)
	state.HandleToolResult(`{"status":"ok","extracted":true,"items":[{"id":" I1 ","description":" Check build ","kind":"SOFT","status":"Completed","evidence":" go test ./... "},{"id":"I2","description":"Review failures","kind":"hard","status":"UNMET"}]}`)

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
