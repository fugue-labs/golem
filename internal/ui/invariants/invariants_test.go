package invariants

import "testing"

func TestHandleToolErrorRestoresExtractedState(t *testing.T) {
	state := State{}
	state.HandleToolCall(`{"command":"extract"}`)
	state.HandleToolError()

	if state.Extracted {
		t.Fatalf("expected Extracted=false after rollback")
	}
	if len(state.Items) != 0 {
		t.Fatalf("expected no items after rollback, got %d", len(state.Items))
	}
}

func TestHandleToolResultReplacesItemsAndMarksExtracted(t *testing.T) {
	state := State{}
	state.HandleToolCall(`{"command":"extract"}`)
	state.HandleToolResult(`{"status":"ok","extracted":true,"items":[{"id":"I1","description":"Commit the work.","kind":"hard","status":"pass","evidence":"git commit abc123"},{"id":"I2","description":"Push the work.","kind":"hard","status":"unknown"},{"id":"I3","description":"Delight users.","kind":"soft","status":"pass"}]}`)

	if !state.Extracted {
		t.Fatalf("expected Extracted=true after successful extract result")
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
}

func TestHandleToolCallAddNormalizesItems(t *testing.T) {
	state := State{}
	state.HandleToolCall(`{"command":"add","items":[{"id":"I7","description":"Use planning","kind":"soft","status":"passed"},{"id":"I8","description":"Verify build","kind":"hard","status":"in-progress"}]}`)
	state.HandleToolResult(`{"status":"ok"}`)

	if got := len(state.Items); got != 2 {
		t.Fatalf("expected 2 items, got %d", got)
	}
	if state.Items[0].Status != "pass" || state.Items[0].Kind != "soft" {
		t.Fatalf("unexpected first item: %+v", state.Items[0])
	}
	if state.Items[1].Status != "in_progress" || state.Items[1].Kind != "hard" {
		t.Fatalf("unexpected second item: %+v", state.Items[1])
	}
}
