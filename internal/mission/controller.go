package mission

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Controller manages mission lifecycle and orchestration.
type Controller struct {
	store Store
}

// NewController creates a controller backed by the given store.
func NewController(store Store) *Controller {
	return &Controller{store: store}
}

// Store returns the underlying store (for direct queries in the TUI layer).
func (c *Controller) Store() Store {
	return c.store
}

// CreateMission creates a new mission in draft status.
func (c *Controller) CreateMission(ctx context.Context, req CreateMissionRequest) (*Mission, error) {
	now := time.Now().UTC()
	m := &Mission{
		ID:         generateID("m"),
		Title:      req.Title,
		Goal:       req.Goal,
		RepoRoot:   req.RepoRoot,
		BaseCommit: req.BaseCommit,
		BaseBranch: req.BaseBranch,
		Status:     MissionDraft,
		Budget:     req.Budget,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := c.store.CreateMission(ctx, m); err != nil {
		return nil, fmt.Errorf("create mission: %w", err)
	}

	c.store.AppendEvent(ctx, &Event{
		MissionID: m.ID,
		Type:      "mission.created",
		CreatedAt: now,
	})

	return m, nil
}

// GetMission retrieves a mission by ID.
func (c *Controller) GetMission(ctx context.Context, id string) (*Mission, error) {
	return c.store.GetMission(ctx, id)
}

// GetMissionSummary returns an aggregate view of mission state.
func (c *Controller) GetMissionSummary(ctx context.Context, id string) (*MissionSummary, error) {
	summary, err := BuildMissionSummary(ctx, c.store, id)
	if err != nil {
		return nil, err
	}
	if summary != nil {
		summary.FillDisplayDefaults()
	}
	return summary, nil
}

// ApplyPlan applies a planning result to a draft/planning mission, creating
// tasks and dependencies in the store.
func (c *Controller) ApplyPlan(ctx context.Context, missionID string, plan *PlanResult) error {
	NormalizePlanResult(plan)
	if err := ValidatePlanResult(plan); err != nil {
		return fmt.Errorf("validate plan: %w", err)
	}

	m, err := c.store.GetMission(ctx, missionID)
	if err != nil {
		return fmt.Errorf("get mission: %w", err)
	}

	if m.Status != MissionDraft && m.Status != MissionPlanning {
		return fmt.Errorf("cannot apply plan to mission in %s state", m.Status)
	}

	now := time.Now().UTC()

	// Apply success criteria from plan.
	if len(plan.SuccessCriteria) > 0 {
		m.SuccessCriteria = plan.SuccessCriteria
	}

	// Create tasks.
	for _, pt := range plan.Tasks {
		t := &Task{
			ID:                 pt.ID,
			MissionID:          missionID,
			Title:              pt.Title,
			Kind:               pt.Kind,
			Objective:          pt.Objective,
			Status:             TaskPending,
			Priority:           pt.Priority,
			Scope:              pt.Scope,
			AcceptanceCriteria: pt.AcceptanceCriteria,
			EstimatedEffort:    pt.EstimatedEffort,
			RiskLevel:          pt.RiskLevel,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := c.store.CreateTask(ctx, t); err != nil {
			return fmt.Errorf("create task %s: %w", t.ID, err)
		}
	}

	// Create dependencies.
	for _, dep := range plan.Dependencies {
		if err := c.store.AddDependency(ctx, dep); err != nil {
			return fmt.Errorf("add dependency %s->%s: %w", dep.TaskID, dep.DependsOnID, err)
		}
	}

	// Transition tasks with no dependencies to ready.
	if err := c.resolveReadyTasks(ctx, missionID); err != nil {
		return fmt.Errorf("resolve ready tasks: %w", err)
	}

	// Move mission to awaiting_approval.
	m.Status = MissionAwaitingApproval
	m.UpdatedAt = now
	if err := c.store.UpdateMission(ctx, m); err != nil {
		return fmt.Errorf("update mission: %w", err)
	}

	c.store.AppendEvent(ctx, &Event{
		MissionID: missionID,
		Type:      "plan.applied",
		CreatedAt: now,
	})

	return nil
}

// StartMission transitions a mission from awaiting_approval to running.
func (c *Controller) StartMission(ctx context.Context, missionID string) error {
	m, err := c.store.GetMission(ctx, missionID)
	if err != nil {
		return err
	}

	if m.Status != MissionAwaitingApproval && m.Status != MissionPaused {
		return fmt.Errorf("cannot start mission in %s state (need awaiting_approval or paused)", m.Status)
	}

	now := time.Now().UTC()
	m.Status = MissionRunning
	m.UpdatedAt = now
	if m.StartedAt == nil {
		m.StartedAt = &now
	}

	if err := c.store.UpdateMission(ctx, m); err != nil {
		return err
	}

	c.store.AppendEvent(ctx, &Event{
		MissionID: missionID,
		Type:      "mission.started",
		CreatedAt: now,
	})

	return nil
}

// PauseMission transitions a running mission to paused.
func (c *Controller) PauseMission(ctx context.Context, missionID string) error {
	m, err := c.store.GetMission(ctx, missionID)
	if err != nil {
		return err
	}
	if m.Status != MissionRunning {
		return fmt.Errorf("cannot pause mission in %s state", m.Status)
	}

	now := time.Now().UTC()
	m.Status = MissionPaused
	m.UpdatedAt = now
	if err := c.store.UpdateMission(ctx, m); err != nil {
		return err
	}

	c.store.AppendEvent(ctx, &Event{
		MissionID: missionID,
		Type:      "mission.paused",
		CreatedAt: now,
	})

	return nil
}

// CancelMission terminates a mission.
func (c *Controller) CancelMission(ctx context.Context, missionID string) error {
	m, err := c.store.GetMission(ctx, missionID)
	if err != nil {
		return err
	}
	if m.Status.IsTerminal() {
		return fmt.Errorf("mission already in terminal state: %s", m.Status)
	}

	now := time.Now().UTC()
	m.Status = MissionCancelled
	m.UpdatedAt = now
	m.EndedAt = &now
	if err := c.store.UpdateMission(ctx, m); err != nil {
		return err
	}

	c.store.AppendEvent(ctx, &Event{
		MissionID: missionID,
		Type:      "mission.cancelled",
		CreatedAt: now,
	})

	return nil
}

// resolveReadyTasks transitions pending tasks whose dependencies are all done to ready.
func (c *Controller) resolveReadyTasks(ctx context.Context, missionID string) error {
	tasks, err := c.store.ListTasks(ctx, missionID)
	if err != nil {
		return err
	}

	deps, err := c.store.ListDependencies(ctx, missionID)
	if err != nil {
		return err
	}

	// Build set of task IDs that are done/integrated.
	// TaskAccepted is NOT terminal — integration can still fail.
	doneSet := make(map[string]bool)
	for _, t := range tasks {
		if t.Status == TaskDone || t.Status == TaskIntegrated {
			doneSet[t.ID] = true
		}
	}

	// Build dependency map: taskID -> set of deps that are NOT done.
	unsatisfied := make(map[string]int)
	for _, d := range deps {
		if !doneSet[d.DependsOnID] {
			unsatisfied[d.TaskID]++
		}
	}

	now := time.Now().UTC()
	for _, t := range tasks {
		if t.Status != TaskPending {
			continue
		}
		if unsatisfied[t.ID] == 0 {
			t.Status = TaskReady
			t.UpdatedAt = now
			if err := c.store.UpdateTask(ctx, t); err != nil {
				return err
			}
		}
	}

	return nil
}

// Close releases controller resources.
func (c *Controller) Close() error {
	return c.store.Close()
}

// generateID creates a short random ID with a prefix.
func generateID(prefix string) string {
	b := make([]byte, 8)
	rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}
