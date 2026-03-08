package plan

import "encoding/json"

// Task mirrors gollem's deep.PlanTask for TUI display.
type Task struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Notes       string `json:"notes,omitempty"`
}

// State tracks the agent's current plan by parsing planning tool messages.
type State struct {
	Tasks       []Task
	lastCommand string
	prevTasks   []Task
	hasPrev     bool
}

// planCommand mirrors the planning tool's input schema.
type planCommand struct {
	Command     string `json:"command"`
	Tasks       []Task `json:"tasks,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
	Status      string `json:"status,omitempty"`
	Notes       string `json:"notes,omitempty"`
	Description string `json:"description,omitempty"`
}

// planResult captures relevant fields from planning tool results.
type planResult struct {
	Status string `json:"status"`
	Tasks  []Task `json:"tasks,omitempty"`
	Task   *Task  `json:"task,omitempty"`
}

func (s *State) HasTasks() bool { return len(s.Tasks) > 0 }

func cloneTasks(tasks []Task) []Task {
	if tasks == nil {
		return nil
	}
	cloned := make([]Task, len(tasks))
	copy(cloned, tasks)
	return cloned
}

func (s *State) snapshot() {
	s.prevTasks = cloneTasks(s.Tasks)
	s.hasPrev = true
}

func (s *State) clearSnapshot() {
	s.prevTasks = nil
	s.hasPrev = false
}

func (s *State) revertSnapshot() {
	if s.hasPrev {
		s.Tasks = cloneTasks(s.prevTasks)
	}
	s.clearSnapshot()
}

// Progress returns completed and total task counts.
func (s *State) Progress() (completed, total int) {
	total = len(s.Tasks)
	for _, t := range s.Tasks {
		if t.Status == "completed" {
			completed++
		}
	}
	return
}

// HandleToolCall parses planning tool call args and updates state optimistically.
func (s *State) HandleToolCall(rawArgs string) {
	var cmd planCommand
	if err := json.Unmarshal([]byte(rawArgs), &cmd); err != nil {
		return
	}

	s.snapshot()
	s.lastCommand = cmd.Command

	switch cmd.Command {
	case "create":
		s.Tasks = make([]Task, len(cmd.Tasks))
		copy(s.Tasks, cmd.Tasks)
		for i := range s.Tasks {
			if s.Tasks[i].Status == "" {
				s.Tasks[i].Status = "pending"
			}
		}

	case "add":
		if len(cmd.Tasks) > 0 {
			for _, t := range cmd.Tasks {
				if t.Status == "" {
					t.Status = "pending"
				}
				s.Tasks = append(s.Tasks, t)
			}
		} else if cmd.TaskID != "" && cmd.Description != "" {
			s.Tasks = append(s.Tasks, Task{
				ID:          cmd.TaskID,
				Description: cmd.Description,
				Status:      "pending",
			})
		}

	case "update":
		for i := range s.Tasks {
			if s.Tasks[i].ID == cmd.TaskID {
				if cmd.Status != "" {
					s.Tasks[i].Status = cmd.Status
				}
				if cmd.Notes != "" {
					s.Tasks[i].Notes = cmd.Notes
				}
				break
			}
		}

	case "delete":
		for i := range s.Tasks {
			if s.Tasks[i].ID == cmd.TaskID {
				s.Tasks = append(s.Tasks[:i], s.Tasks[i+1:]...)
				break
			}
		}
	}
}

// HandleToolResult reconciles state from planning tool results.
// The "get" result contains the full task list, serving as a sync point.
func (s *State) HandleToolResult(result string) {
	defer s.clearSnapshot()

	var res planResult
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		return
	}

	if s.lastCommand == "get" && res.Tasks != nil {
		s.Tasks = res.Tasks
		return
	}

	if s.lastCommand == "update" && res.Task != nil {
		for i := range s.Tasks {
			if s.Tasks[i].ID == res.Task.ID {
				s.Tasks[i] = *res.Task
				break
			}
		}
	}
}

func (s *State) HandleToolError() {
	s.revertSnapshot()
}
