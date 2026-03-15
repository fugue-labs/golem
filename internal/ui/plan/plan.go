package plan

import (
	"fmt"
	"strings"

	"github.com/fugue-labs/gollem/ext/deep"
)

// Task mirrors gollem's deep.PlanTask for TUI display.
type Task struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Notes       string `json:"notes,omitempty"`
}

// State tracks the agent's current plan for TUI display.
type State struct {
	Tasks []Task
}

func FromDeepPlan(plan deep.Plan) State {
	return State{Tasks: fromDeepTasks(plan.Tasks)}
}

func fromDeepTasks(tasks []deep.PlanTask) []Task {
	if tasks == nil {
		return nil
	}
	converted := make([]Task, len(tasks))
	for i, task := range tasks {
		converted[i] = normalizeTask(Task{
			ID:          task.ID,
			Description: task.Description,
			Status:      task.Status,
			Notes:       task.Notes,
		})
	}
	return converted
}

func (s *State) HasTasks() bool { return len(s.Tasks) > 0 }

// Progress returns completed and total task counts.
func (s *State) Progress() (completed, total int) {
	total = len(s.Tasks)
	for _, t := range s.Tasks {
		if normalizeTaskStatus(t.Status) == "completed" {
			completed++
		}
	}
	return
}

// Counts returns task counts grouped by workflow priority.
func (s *State) Counts() (completed, inProgress, blocked, pending int) {
	for _, t := range s.Tasks {
		switch normalizeTaskStatus(t.Status) {
		case "completed":
			completed++
		case "in_progress":
			inProgress++
		case "blocked":
			blocked++
		default:
			pending++
		}
	}
	return
}

// Summary returns an active-work-first summary for the workflow rail.
func (s *State) Summary() string {
	completed, total := s.Progress()
	_, inProgress, blocked, pending := s.Counts()
	focus := s.Focus()
	switch {
	case focus != nil && normalizeTaskStatus(focus.Status) == "blocked":
		return fmt.Sprintf("%d blocked · %d active · %d ready", blocked, inProgress, pending)
	case focus != nil && normalizeTaskStatus(focus.Status) == "in_progress":
		return fmt.Sprintf("%d active · %d ready · %d/%d done", inProgress, pending, completed, total)
	case total > 0 && completed == total:
		return fmt.Sprintf("%d/%d done", completed, total)
	case total > 0:
		return fmt.Sprintf("%d ready · %d/%d done", pending, completed, total)
	default:
		return ""
	}
}

// Focus returns the highest-priority task to surface in the rail.
func (s *State) Focus() *Task {
	for i := range s.Tasks {
		if normalizeTaskStatus(s.Tasks[i].Status) == "blocked" {
			return &s.Tasks[i]
		}
	}
	for i := range s.Tasks {
		if normalizeTaskStatus(s.Tasks[i].Status) == "in_progress" {
			return &s.Tasks[i]
		}
	}
	for i := range s.Tasks {
		if normalizeTaskStatus(s.Tasks[i].Status) != "completed" {
			return &s.Tasks[i]
		}
	}
	if len(s.Tasks) == 0 {
		return nil
	}
	return &s.Tasks[0]
}

// Next returns the next actionable non-complete task after the focus item.
func (s *State) Next() *Task {
	for i := range s.Tasks {
		status := normalizeTaskStatus(s.Tasks[i].Status)
		if status == "blocked" || status == "in_progress" || status == "completed" {
			continue
		}
		return &s.Tasks[i]
	}
	return nil
}

func normalizeTask(task Task) Task {
	task.ID = strings.TrimSpace(task.ID)
	task.Description = strings.TrimSpace(task.Description)
	task.Status = normalizeTaskStatus(task.Status)
	if task.Status == "" {
		task.Status = "pending"
	}
	return task
}

func normalizeTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "":
		return ""
	case "pending", "todo", "to_do", "queued", "not_started":
		return "pending"
	case "in_progress", "in-progress", "in progress", "wip", "working", "active":
		return "in_progress"
	case "completed", "complete", "done", "finished":
		return "completed"
	case "blocked", "stuck":
		return "blocked"
	default:
		return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(status)), " ", "_")
	}
}
