package plan

import (
	"testing"

	"github.com/fugue-labs/gollem/ext/deep"
)

func TestFromDeepPlanNormalizesStatuses(t *testing.T) {
	state := FromDeepPlan(deep.Plan{Tasks: []deep.PlanTask{
		{ID: " T1 ", Description: " First step ", Status: "done"},
		{ID: "T2", Description: "Second step", Status: "in progress"},
		{ID: "T3", Description: "Third step", Status: ""},
	}})

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

func TestFromDeepPlanKeepsEmptyState(t *testing.T) {
	state := FromDeepPlan(deep.Plan{})
	if state.HasTasks() {
		t.Fatal("expected no tasks")
	}
}
