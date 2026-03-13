package mission

import (
	"fmt"
	"strings"
)

var allowedTaskKinds = map[TaskKind]bool{
	TaskKindCode:             true,
	TaskKindTest:             true,
	TaskKindDocs:             true,
	TaskKindInvestigation:    true,
	TaskKindIntegrationFixup: true,
	TaskKindReviewFix:        true,
}

var allowedRiskLevels = map[RiskLevel]bool{
	RiskLow:    true,
	RiskMedium: true,
	RiskHigh:   true,
}

var allowedEfforts = map[string]bool{
	"small":  true,
	"medium": true,
	"large":  true,
}

// NormalizePlanResult trims planner output and fills safe defaults used by the
// mission controller before validation/persistence.
func NormalizePlanResult(plan *PlanResult) {
	if plan == nil {
		return
	}

	plan.Summary = strings.TrimSpace(plan.Summary)
	for i := range plan.SuccessCriteria {
		plan.SuccessCriteria[i] = strings.TrimSpace(plan.SuccessCriteria[i])
	}

	for i := range plan.Tasks {
		t := &plan.Tasks[i]
		t.ID = strings.TrimSpace(t.ID)
		t.Title = strings.TrimSpace(t.Title)
		t.Objective = strings.TrimSpace(t.Objective)
		if t.Title == "" {
			t.Title = t.Objective
		}
		if t.Objective == "" {
			t.Objective = t.Title
		}
		t.Kind = TaskKind(strings.ToLower(strings.TrimSpace(string(t.Kind))))
		if t.Priority < 0 {
			t.Priority = 0
		}
		for j := range t.Scope.WritePaths {
			t.Scope.WritePaths[j] = strings.TrimSpace(t.Scope.WritePaths[j])
		}
		for j := range t.Scope.ReadPaths {
			t.Scope.ReadPaths[j] = strings.TrimSpace(t.Scope.ReadPaths[j])
		}
		for j := range t.AcceptanceCriteria {
			t.AcceptanceCriteria[j] = strings.TrimSpace(t.AcceptanceCriteria[j])
		}
		t.EstimatedEffort = strings.ToLower(strings.TrimSpace(t.EstimatedEffort))
		t.RiskLevel = RiskLevel(strings.ToLower(strings.TrimSpace(string(t.RiskLevel))))
		if t.RiskLevel == "" {
			t.RiskLevel = RiskLow
		}
	}

	for i := range plan.Dependencies {
		plan.Dependencies[i].TaskID = strings.TrimSpace(plan.Dependencies[i].TaskID)
		plan.Dependencies[i].DependsOnID = strings.TrimSpace(plan.Dependencies[i].DependsOnID)
	}
}

// ValidatePlanResult checks that a plan is internally consistent before it is
// written to durable mission state.
func ValidatePlanResult(plan *PlanResult) error {
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}
	if len(plan.Tasks) == 0 {
		return fmt.Errorf("plan must contain at least one task")
	}

	ids := make(map[string]bool, len(plan.Tasks))
	for _, t := range plan.Tasks {
		if t.ID == "" {
			return fmt.Errorf("plan task id cannot be empty")
		}
		if ids[t.ID] {
			return fmt.Errorf("duplicate task id %q", t.ID)
		}
		ids[t.ID] = true
		if strings.TrimSpace(t.Title) == "" {
			return fmt.Errorf("task %s title cannot be empty", t.ID)
		}
		if !allowedTaskKinds[t.Kind] {
			return fmt.Errorf("task %s has invalid kind %q", t.ID, t.Kind)
		}
		if !allowedRiskLevels[t.RiskLevel] {
			return fmt.Errorf("task %s has invalid risk level %q", t.ID, t.RiskLevel)
		}
		if t.EstimatedEffort != "" && !allowedEfforts[t.EstimatedEffort] {
			return fmt.Errorf("task %s has invalid estimated effort %q", t.ID, t.EstimatedEffort)
		}
	}

	seenDeps := make(map[string]bool, len(plan.Dependencies))
	graph := make(map[string][]string, len(plan.Tasks))
	for _, dep := range plan.Dependencies {
		if dep.TaskID == "" || dep.DependsOnID == "" {
			return fmt.Errorf("dependencies must include task_id and depends_on_id")
		}
		if !ids[dep.TaskID] {
			return fmt.Errorf("dependency references unknown task %q", dep.TaskID)
		}
		if !ids[dep.DependsOnID] {
			return fmt.Errorf("dependency references unknown task %q", dep.DependsOnID)
		}
		if dep.TaskID == dep.DependsOnID {
			return fmt.Errorf("task %s cannot depend on itself", dep.TaskID)
		}
		key := dep.TaskID + "->" + dep.DependsOnID
		if seenDeps[key] {
			return fmt.Errorf("duplicate dependency %s", key)
		}
		seenDeps[key] = true
		graph[dep.TaskID] = append(graph[dep.TaskID], dep.DependsOnID)
	}

	visiting := make(map[string]bool, len(plan.Tasks))
	visited := make(map[string]bool, len(plan.Tasks))
	var dfs func(string) error
	dfs = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("dependency cycle detected at task %s", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, depID := range graph[id] {
			if err := dfs(depID); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	for id := range ids {
		if err := dfs(id); err != nil {
			return err
		}
	}

	return nil
}
