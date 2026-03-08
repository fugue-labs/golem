package plan

import "testing"

func TestHandleToolCallNormalizesStatuses(t *testing.T) {
	state := State{}
	state.HandleToolCall(`{"command":"create","tasks":[{"id":" T1 ","description":" First step ","status":"done"},{"id":"T2","description":"Second step","status":"in progress"},{"id":"T3","description":"Third step","status":""}]}`)
	state.HandleToolResult(`{"status":"created"}`)

	if got := state.Tasks[0].Status; got != "completed" {
		t.Fatalf("first task status = %q", got)
	}
	if got := state.Tasks[0].ID; got != "T1" {
		t.Fatalf("first task id = %q", got)
	}
	if got := state.Tasks[0].Description; got != "First step" {
		t.Fatalf("first task description = %q", got)
	}
	if got := state.Tasks[1].Status; got != "in_progress" {
		t.Fatalf("second task status = %q", got)
	}
	if got := state.Tasks[2].Status; got != "pending" {
		t.Fatalf("third task status = %q", got)
	}
	if completed, total := state.Progress(); completed != 1 || total != 3 {
		t.Fatalf("progress = %d/%d, want 1/3", completed, total)
	}
}

func TestHandleToolResultGetNormalizesStatuses(t *testing.T) {
	state := State{}
	state.HandleToolCall(`{"command":"get"}`)
	state.HandleToolResult(`{"tasks":[{"id":"A","description":"Alpha","status":"Finished"},{"id":"B","description":"Beta","status":"ACTIVE"}]}`)

	if got := state.Tasks[0].Status; got != "completed" {
		t.Fatalf("first task status = %q", got)
	}
	if got := state.Tasks[1].Status; got != "in_progress" {
		t.Fatalf("second task status = %q", got)
	}
}
