package plan

import (
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
