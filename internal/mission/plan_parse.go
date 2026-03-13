package mission

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParsePlanResult parses planner output into a validated PlanResult.
func ParsePlanResult(text string) (*PlanResult, error) {
	jsonText, err := extractJSONObject(text)
	if err != nil {
		return nil, err
	}

	type rawPlanTask struct {
		ID                 string    `json:"id"`
		Title              string    `json:"title"`
		Description        string    `json:"description"`
		Kind               TaskKind  `json:"kind"`
		Objective          string    `json:"objective"`
		Priority           int       `json:"priority"`
		Scope              TaskScope `json:"scope"`
		AcceptanceCriteria []string  `json:"acceptance_criteria"`
		EstimatedEffort    string    `json:"estimated_effort"`
		RiskLevel          RiskLevel `json:"risk_level"`
	}
	type rawPlan struct {
		Summary         string           `json:"summary"`
		SuccessCriteria []string         `json:"success_criteria"`
		Tasks           []rawPlanTask    `json:"tasks"`
		Dependencies    []TaskDependency `json:"dependencies"`
	}

	var raw rawPlan
	if err := json.Unmarshal([]byte(jsonText), &raw); err != nil {
		return nil, fmt.Errorf("decode planner json: %w", err)
	}

	plan := &PlanResult{
		Summary:         raw.Summary,
		SuccessCriteria: raw.SuccessCriteria,
		Dependencies:    raw.Dependencies,
		Tasks:           make([]PlanTask, 0, len(raw.Tasks)),
	}
	for _, task := range raw.Tasks {
		title := strings.TrimSpace(task.Title)
		if title == "" {
			title = strings.TrimSpace(task.Description)
		}
		objective := strings.TrimSpace(task.Objective)
		if objective == "" {
			objective = strings.TrimSpace(task.Description)
		}
		if objective == "" {
			objective = title
		}
		plan.Tasks = append(plan.Tasks, PlanTask{
			ID:                 task.ID,
			Title:              title,
			Kind:               task.Kind,
			Objective:          objective,
			Priority:           task.Priority,
			Scope:              task.Scope,
			AcceptanceCriteria: task.AcceptanceCriteria,
			EstimatedEffort:    task.EstimatedEffort,
			RiskLevel:          task.RiskLevel,
		})
	}

	NormalizePlanResult(plan)
	if err := ValidatePlanResult(plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func extractJSONObject(text string) (string, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", fmt.Errorf("planner output is empty")
	}

	if block, ok := extractFenceBlock(trimmed, "json"); ok {
		return block, nil
	}
	if block, ok := extractFenceBlock(trimmed, ""); ok {
		return block, nil
	}
	if json.Valid([]byte(trimmed)) {
		return trimmed, nil
	}

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		candidate := strings.TrimSpace(trimmed[start : end+1])
		if json.Valid([]byte(candidate)) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("planner output did not contain a valid JSON object")
}

func extractFenceBlock(text, lang string) (string, bool) {
	open := "```"
	if lang != "" {
		open += lang
	}
	start := strings.Index(text, open)
	if start < 0 {
		return "", false
	}
	body := text[start+len(open):]
	body = strings.TrimPrefix(body, "\n")
	end := strings.Index(body, "```")
	if end < 0 {
		return "", false
	}
	candidate := strings.TrimSpace(body[:end])
	if !json.Valid([]byte(candidate)) {
		return "", false
	}
	return candidate, true
}
