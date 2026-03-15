package mission

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unsafe"
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
	c.store.AppendEvent(ctx, &Event{ //nolint:errcheck
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
func (c *Controller) ApplyPlan(ctx context.Context, missionID string, plan *PlanResult) (err error) {
	prepared := clonePlanResult(plan)
	NormalizePlanResult(prepared)

	m, err := c.store.GetMission(ctx, missionID)
	if err != nil {
		return fmt.Errorf("get mission: %w", err)
	}
	if m.Status != MissionDraft && m.Status != MissionPlanning {
		return fmt.Errorf("cannot apply plan to mission in %s state", m.Status)
	}

	existingTasks, err := c.store.ListTasks(ctx, missionID)
	if err != nil {
		return fmt.Errorf("list mission tasks: %w", err)
	}
	if len(existingTasks) > 0 {
		return fmt.Errorf("cannot apply plan to mission %s with existing tasks; use replan instead", missionID)
	}
	existingDeps, err := c.store.ListDependencies(ctx, missionID)
	if err != nil {
		return fmt.Errorf("list mission dependencies: %w", err)
	}
	if len(existingDeps) > 0 {
		return fmt.Errorf("cannot apply plan to mission %s with existing dependencies; use replan instead", missionID)
	}

	existingTaskIDs, err := c.collectExistingTaskIDs(ctx)
	if err != nil {
		return fmt.Errorf("collect existing task ids: %w", err)
	}
	remapCollidingShortTaskIDs(prepared, existingTaskIDs)

	if err := ValidatePlanResult(prepared); err != nil {
		return fmt.Errorf("validate plan: %w", err)
	}

	now := time.Now().UTC()
	approval, err := newMissionPlanApproval(missionID, prepared, now)
	if err != nil {
		return fmt.Errorf("build plan approval: %w", err)
	}
	rollback := applyPlanRollback{
		mission:            cloneMission(m),
		createdTaskIDs:     make([]string, 0, len(prepared.Tasks)),
		addedDependencies:  append([]TaskDependency(nil), prepared.Dependencies...),
		createdApprovalIDs: []string{approval.ID},
	}
	committed := false
	defer func() {
		if err == nil || committed {
			return
		}
		if rollbackErr := c.rollbackApplyPlan(ctx, rollback); rollbackErr != nil {
			err = fmt.Errorf("%w (rollback: %v)", err, rollbackErr)
		}
	}()

	for _, pt := range prepared.Tasks {
		t := &Task{
			ID:                 pt.ID,
			MissionID:          missionID,
			Title:              pt.Title,
			Kind:               pt.Kind,
			Objective:          pt.Objective,
			Status:             TaskPending,
			Priority:           pt.Priority,
			Scope:              cloneTaskScope(pt.Scope),
			AcceptanceCriteria: append([]string(nil), pt.AcceptanceCriteria...),
			EstimatedEffort:    pt.EstimatedEffort,
			RiskLevel:          pt.RiskLevel,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := c.store.CreateTask(ctx, t); err != nil {
			return fmt.Errorf("create task %s: %w", t.ID, err)
		}
		rollback.createdTaskIDs = append(rollback.createdTaskIDs, t.ID)
	}

	for _, dep := range prepared.Dependencies {
		if err := c.store.AddDependency(ctx, dep); err != nil {
			return fmt.Errorf("add dependency %s->%s: %w", dep.TaskID, dep.DependsOnID, err)
		}
	}

	if err := c.resolveReadyTasks(ctx, missionID); err != nil {
		return fmt.Errorf("resolve ready tasks: %w", err)
	}
	if err := c.store.CreateApproval(ctx, approval); err != nil {
		return fmt.Errorf("create plan approval: %w", err)
	}

	updatedMission := cloneMission(m)
	if len(prepared.SuccessCriteria) > 0 {
		updatedMission.SuccessCriteria = append([]string(nil), prepared.SuccessCriteria...)
	}
	updatedMission.Status = MissionAwaitingApproval
	updatedMission.UpdatedAt = now
	if err := c.store.UpdateMission(ctx, updatedMission); err != nil {
		return fmt.Errorf("update mission: %w", err)
	}

	c.store.AppendEvent(ctx, &Event{ //nolint:errcheck
		MissionID: missionID,
		Type:      "plan.applied",
		CreatedAt: now,
	})
	committed = true
	return nil
}

// ApproveMission resolves the durable mission-plan approval gate.
func (c *Controller) ApproveMission(ctx context.Context, missionID string) (err error) {
	m, err := c.store.GetMission(ctx, missionID)
	if err != nil {
		return err
	}
	if m.Status != MissionAwaitingApproval {
		return fmt.Errorf("cannot approve mission in %s state", m.Status)
	}

	approval, err := c.resolveMissionPlanApproval(ctx, missionID)
	if err != nil {
		return err
	}
	if approval == nil {
		return fmt.Errorf("mission %s is missing a durable plan approval gate", missionID)
	}
	if approval.Status == ApprovalApproved {
		return nil
	}
	if approval.Status != ApprovalPending {
		return fmt.Errorf("cannot approve mission plan in %s state", approval.Status)
	}

	now := time.Now().UTC()
	rollback := *approval
	approval.Status = ApprovalApproved
	approval.ResolvedAt = &now
	approval.ResponseJSON = marshalApprovalResponseJSON(map[string]any{
		"decision":    "approved",
		"approved_at": now.Format(time.RFC3339Nano),
	})
	defer func() {
		if err == nil {
			return
		}
		_ = c.store.UpdateApproval(ctx, &rollback)
	}()
	if err := c.store.UpdateApproval(ctx, approval); err != nil {
		return fmt.Errorf("approve mission plan: %w", err)
	}
	c.store.AppendEvent(ctx, &Event{ //nolint:errcheck
		MissionID: missionID,
		Type:      "mission.approved",
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
	if m.Status == MissionAwaitingApproval {
		approval, err := c.resolveMissionPlanApproval(ctx, missionID)
		if err != nil {
			return err
		}
		if approval == nil {
			return fmt.Errorf("cannot start mission without a durable mission-plan gate")
		}
		if approval.Status == ApprovalPending {
			return fmt.Errorf("cannot start mission while mission plan approval is pending")
		}
		if approval.Status != ApprovalApproved {
			return fmt.Errorf("cannot start mission with %s plan approval", approval.Status)
		}
		pendingApprovals, err := c.listPendingApprovals(ctx, missionID)
		if err != nil {
			return fmt.Errorf("list pending approvals: %w", err)
		}
		if pendingApprovals > 0 {
			blockingApproval, blockErr := c.firstBlockingPendingApproval(ctx, missionID)
			if blockErr != nil {
				return fmt.Errorf("list pending approvals: %w", blockErr)
			}
			if blockingApproval != nil {
				return fmt.Errorf("mission plan approval is approved, but cannot start mission until %s", describeApprovalBlocker(blockingApproval))
			}
			return fmt.Errorf("mission plan approval is approved, but cannot start mission with %d pending approval(s)", pendingApprovals)
		}
	}

	now := time.Now().UTC()
	m.Status = MissionRunning
	m.UpdatedAt = now
	if m.StartedAt == nil {
		m.StartedAt = &now
	}
	m.EndedAt = nil
	if err := c.store.UpdateMission(ctx, m); err != nil {
		return err
	}
	c.store.AppendEvent(ctx, &Event{ //nolint:errcheck
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
	c.store.AppendEvent(ctx, &Event{ //nolint:errcheck
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
	c.store.AppendEvent(ctx, &Event{ //nolint:errcheck
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

	doneSet := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		if t.Status == TaskDone || t.Status == TaskIntegrated {
			doneSet[t.ID] = true
		}
	}

	unsatisfied := make(map[string]int)
	for _, dep := range deps {
		if !doneSet[dep.DependsOnID] {
			unsatisfied[dep.TaskID]++
		}
	}

	now := time.Now().UTC()
	for _, t := range tasks {
		if t.Status != TaskPending {
			continue
		}
		if unsatisfied[t.ID] != 0 {
			continue
		}
		t.Status = TaskReady
		t.UpdatedAt = now
		if err := c.store.UpdateTask(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

// Close releases controller resources.
func (c *Controller) Close() error {
	return c.store.Close()
}

type applyPlanRollback struct {
	mission            *Mission
	createdTaskIDs     []string
	addedDependencies  []TaskDependency
	createdApprovalIDs []string
}

func (c *Controller) collectExistingTaskIDs(ctx context.Context) (map[string]bool, error) {
	missions, err := c.store.ListMissions(ctx)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]bool)
	for _, mission := range missions {
		tasks, err := c.store.ListTasks(ctx, mission.ID)
		if err != nil {
			return nil, err
		}
		for _, task := range tasks {
			ids[task.ID] = true
		}
	}
	return ids, nil
}

func remapCollidingShortTaskIDs(plan *PlanResult, existing map[string]bool) {
	if plan == nil || len(plan.Tasks) == 0 || len(existing) == 0 {
		return
	}
	reserved := make(map[string]bool, len(existing)+len(plan.Tasks))
	for id := range existing {
		reserved[id] = true
	}
	idMap := make(map[string]string)
	for _, task := range plan.Tasks {
		id := strings.TrimSpace(task.ID)
		if id == "" {
			continue
		}
		if !reserved[id] {
			reserved[id] = true
			continue
		}
		if !isShortPlannerID(id) {
			continue
		}
		if _, ok := idMap[id]; ok {
			continue
		}
		newID := generateUniqueTaskID(reserved)
		reserved[newID] = true
		idMap[id] = newID
	}
	for i := range plan.Tasks {
		if newID, ok := idMap[plan.Tasks[i].ID]; ok {
			plan.Tasks[i].ID = newID
		}
	}
	for i := range plan.Dependencies {
		if newID, ok := idMap[plan.Dependencies[i].TaskID]; ok {
			plan.Dependencies[i].TaskID = newID
		}
		if newID, ok := idMap[plan.Dependencies[i].DependsOnID]; ok {
			plan.Dependencies[i].DependsOnID = newID
		}
	}
}

func generateUniqueTaskID(reserved map[string]bool) string {
	for {
		candidate := generateID("t")
		if !reserved[candidate] {
			return candidate
		}
	}
}

func (c *Controller) listPendingApprovals(ctx context.Context, missionID string) (int, error) {
	approvals, err := c.store.ListApprovals(ctx, missionID)
	if err != nil {
		return 0, err
	}
	pending := 0
	for _, approval := range approvals {
		if approval.Status == ApprovalPending {
			pending++
		}
	}
	return pending, nil
}

func (c *Controller) firstBlockingPendingApproval(ctx context.Context, missionID string) (*Approval, error) {
	approvals, err := c.store.ListApprovals(ctx, missionID)
	if err != nil {
		return nil, err
	}
	var planApproval *Approval
	var other *Approval
	for _, approval := range approvals {
		if approval == nil || approval.Status != ApprovalPending {
			continue
		}
		cp := *approval
		if approval.Kind == missionPlanApprovalKind {
			if planApproval == nil || approval.CreatedAt.Before(planApproval.CreatedAt) || (approval.CreatedAt.Equal(planApproval.CreatedAt) && approval.ID < planApproval.ID) {
				planApproval = &cp
			}
			continue
		}
		if other == nil || approval.CreatedAt.Before(other.CreatedAt) || (approval.CreatedAt.Equal(other.CreatedAt) && approval.ID < other.ID) {
			other = &cp
		}
	}
	if other != nil {
		return other, nil
	}
	return planApproval, nil
}

func describeApprovalBlocker(approval *Approval) string {
	if approval == nil {
		return "approval resolves"
	}
	if approval.Kind == missionPlanApprovalKind {
		return "the mission plan approval is resolved"
	}
	if approval.TaskID != "" {
		if approval.Kind != "" {
			return fmt.Sprintf("the %s approval for task %s is resolved", approval.Kind, approval.TaskID)
		}
		return fmt.Sprintf("the approval for task %s is resolved", approval.TaskID)
	}
	if approval.Kind != "" {
		return fmt.Sprintf("the %s approval is resolved", approval.Kind)
	}
	return "the pending approval is resolved"
}

func (c *Controller) resolveMissionPlanApproval(ctx context.Context, missionID string) (*Approval, error) {
	approvals, err := c.store.ListApprovals(ctx, missionID)
	if err != nil {
		return nil, fmt.Errorf("list approvals: %w", err)
	}
	var latest *Approval
	for _, approval := range approvals {
		if approval == nil || approval.Kind != missionPlanApprovalKind {
			continue
		}
		if latest == nil || approval.CreatedAt.After(latest.CreatedAt) || (approval.CreatedAt.Equal(latest.CreatedAt) && approval.ID > latest.ID) {
			cp := *approval
			latest = &cp
		}
	}
	return latest, nil
}

func newMissionPlanApproval(missionID string, plan *PlanResult, now time.Time) (*Approval, error) {
	request, err := marshalMissionPlanApprovalRequest(plan)
	if err != nil {
		return nil, err
	}
	return &Approval{
		ID:          generateID("ap"),
		MissionID:   missionID,
		Kind:        missionPlanApprovalKind,
		Status:      ApprovalPending,
		RequestJSON: request,
		CreatedAt:   now,
	}, nil
}

func marshalMissionPlanApprovalRequest(plan *PlanResult) (json.RawMessage, error) {
	payload := map[string]any{
		"summary":           "",
		"success_criteria":  []string{},
		"tasks":             []PlanTask{},
		"dependencies":      []TaskDependency{},
		"requires_approval": true,
	}
	if plan != nil {
		payload["summary"] = plan.Summary
		payload["success_criteria"] = append([]string(nil), plan.SuccessCriteria...)
		payload["tasks"] = clonePlanTasks(plan.Tasks)
		payload["dependencies"] = append([]TaskDependency(nil), plan.Dependencies...)
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(buf), nil
}

func marshalApprovalResponseJSON(payload map[string]any) json.RawMessage {
	if len(payload) == 0 {
		return json.RawMessage("{}")
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage("{}")
	}
	return json.RawMessage(buf)
}

func clonePlanTasks(tasks []PlanTask) []PlanTask {
	if len(tasks) == 0 {
		return nil
	}
	out := make([]PlanTask, len(tasks))
	for i, task := range tasks {
		out[i] = PlanTask{
			ID:                 task.ID,
			Title:              task.Title,
			Kind:               task.Kind,
			Objective:          task.Objective,
			Priority:           task.Priority,
			Scope:              cloneTaskScope(task.Scope),
			AcceptanceCriteria: append([]string(nil), task.AcceptanceCriteria...),
			EstimatedEffort:    task.EstimatedEffort,
			RiskLevel:          task.RiskLevel,
		}
	}
	return out
}

const missionPlanApprovalKind = "plan"

func (c *Controller) rollbackApplyPlan(ctx context.Context, rollback applyPlanRollback) error {
	var errs []string
	if len(rollback.createdApprovalIDs) > 0 {
		if err := rollbackApprovals(c.store, rollback.createdApprovalIDs); err != nil {
			errs = append(errs, fmt.Sprintf("remove created approvals: %v", err))
		}
	}
	if len(rollback.createdTaskIDs) > 0 || len(rollback.addedDependencies) > 0 {
		if err := rollbackPlanArtifacts(c.store, rollback.createdTaskIDs, rollback.addedDependencies); err != nil {
			errs = append(errs, fmt.Sprintf("remove created plan artifacts: %v", err))
		}
	}
	if rollback.mission != nil {
		if err := c.restoreMissionForRollback(ctx, rollback.mission); err != nil {
			errs = append(errs, fmt.Sprintf("restore mission: %v", err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.New(strings.Join(errs, "; "))
}

func (c *Controller) restoreMissionForRollback(ctx context.Context, mission *Mission) error {
	if mission == nil {
		return nil
	}
	if err := c.store.UpdateMission(ctx, cloneMission(mission)); err == nil {
		return nil
	}
	return restoreMissionViaReflection(c.store, mission)
}

func rollbackPlanArtifacts(store Store, taskIDs []string, deps []TaskDependency) error {
	if len(taskIDs) == 0 && len(deps) == 0 {
		return nil
	}
	if err := rollbackPlanArtifactsInDB(store, taskIDs, deps); err == nil {
		return nil
	}
	return removePlanArtifactsViaReflection(store, taskIDs, deps)
}

func rollbackApprovals(store Store, approvalIDs []string) error {
	if len(approvalIDs) == 0 {
		return nil
	}
	if err := rollbackApprovalsInDB(store, approvalIDs); err == nil {
		return nil
	}
	return removeApprovalsViaReflection(store, approvalIDs)
}

func rollbackApprovalsInDB(store Store, approvalIDs []string) error {
	db, err := extractStoreDB(store)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for _, approvalID := range approvalIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM approvals WHERE id = ?`, approvalID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func rollbackPlanArtifactsInDB(store Store, taskIDs []string, deps []TaskDependency) error {
	db, err := extractStoreDB(store)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for _, dep := range deps {
		if _, err := tx.ExecContext(ctx, `DELETE FROM task_dependencies WHERE task_id = ? AND depends_on_id = ?`, dep.TaskID, dep.DependsOnID); err != nil {
			return err
		}
	}
	for _, taskID := range taskIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM task_dependencies WHERE task_id = ? OR depends_on_id = ?`, taskID, taskID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, taskID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func extractStoreDB(store Store) (*sql.DB, error) {
	field, err := settableStructField(store, "db")
	if err != nil {
		return nil, err
	}
	db, ok := field.Interface().(*sql.DB)
	if !ok || db == nil {
		return nil, fmt.Errorf("db field is not *sql.DB")
	}
	return db, nil
}

func removePlanArtifactsViaReflection(store Store, taskIDs []string, deps []TaskDependency) error {
	if err := removeTasksViaReflection(store, taskIDs); err != nil {
		return err
	}
	if err := removeDependenciesViaReflection(store, deps); err != nil {
		return err
	}
	return nil
}

func removeApprovalsViaReflection(store Store, approvalIDs []string) error {
	if len(approvalIDs) == 0 {
		return nil
	}
	field, err := settableStructField(store, "approvals")
	if err != nil {
		return err
	}
	if field.Kind() != reflect.Map {
		return fmt.Errorf("approvals field is %s, want map", field.Kind())
	}
	for _, approvalID := range approvalIDs {
		field.SetMapIndex(reflect.ValueOf(approvalID), reflect.Value{})
	}
	return nil
}

func removeTasksViaReflection(store Store, taskIDs []string) error {
	if len(taskIDs) == 0 {
		return nil
	}
	field, err := settableStructField(store, "tasks")
	if err != nil {
		return err
	}
	if field.Kind() != reflect.Map {
		return fmt.Errorf("tasks field is %s, want map", field.Kind())
	}
	for _, taskID := range taskIDs {
		field.SetMapIndex(reflect.ValueOf(taskID), reflect.Value{})
	}
	return nil
}

func removeDependenciesViaReflection(store Store, deps []TaskDependency) error {
	if len(deps) == 0 {
		return nil
	}
	field, err := settableStructField(store, "deps")
	if err != nil {
		return err
	}
	if field.Kind() != reflect.Slice {
		return fmt.Errorf("deps field is %s, want slice", field.Kind())
	}
	remove := make(map[string]int, len(deps))
	for _, dep := range deps {
		remove[dependencyKey(dep)]++
	}
	filtered := reflect.MakeSlice(field.Type(), 0, field.Len())
	for i := 0; i < field.Len(); i++ {
		dep, ok := field.Index(i).Interface().(TaskDependency)
		if !ok {
			filtered = reflect.Append(filtered, field.Index(i))
			continue
		}
		key := dependencyKey(dep)
		if remove[key] > 0 {
			remove[key]--
			continue
		}
		filtered = reflect.Append(filtered, field.Index(i))
	}
	field.Set(filtered)
	return nil
}

func restoreMissionViaReflection(store Store, mission *Mission) error {
	field, err := settableStructField(store, "missions")
	if err != nil {
		return err
	}
	if field.Kind() != reflect.Map {
		return fmt.Errorf("missions field is %s, want map", field.Kind())
	}
	field.SetMapIndex(reflect.ValueOf(mission.ID), reflect.ValueOf(cloneMission(mission)))
	return nil
}

func settableStructField(target any, name string) (reflect.Value, error) {
	rv := reflect.ValueOf(target)
	if !rv.IsValid() || rv.Kind() != reflect.Pointer || rv.IsNil() {
		return reflect.Value{}, fmt.Errorf("store is not an addressable pointer")
	}
	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("store is %s, want struct pointer", elem.Kind())
	}
	field := elem.FieldByName(name)
	if !field.IsValid() {
		return reflect.Value{}, fmt.Errorf("store does not expose %q field", name)
	}
	if !field.CanAddr() {
		return reflect.Value{}, fmt.Errorf("store field %q is not addressable", name)
	}
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem(), nil
}

func dependencyKey(dep TaskDependency) string {
	return dep.TaskID + "->" + dep.DependsOnID
}

func cloneTaskScope(scope TaskScope) TaskScope {
	return TaskScope{
		WritePaths: append([]string(nil), scope.WritePaths...),
		ReadPaths:  append([]string(nil), scope.ReadPaths...),
	}
}

func clonePlanResult(plan *PlanResult) *PlanResult {
	if plan == nil {
		return nil
	}
	cp := &PlanResult{
		Summary:         plan.Summary,
		SuccessCriteria: append([]string(nil), plan.SuccessCriteria...),
		Tasks:           make([]PlanTask, len(plan.Tasks)),
		Dependencies:    append([]TaskDependency(nil), plan.Dependencies...),
	}
	for i, task := range plan.Tasks {
		cp.Tasks[i] = PlanTask{
			ID:                 task.ID,
			Title:              task.Title,
			Kind:               task.Kind,
			Objective:          task.Objective,
			Priority:           task.Priority,
			Scope:              cloneTaskScope(task.Scope),
			AcceptanceCriteria: append([]string(nil), task.AcceptanceCriteria...),
			EstimatedEffort:    task.EstimatedEffort,
			RiskLevel:          task.RiskLevel,
		}
	}
	return cp
}

// generateID creates a short random ID with a prefix.
func generateID(prefix string) string {
	b := make([]byte, 8)
	rand.Read(b) //nolint:errcheck
	return prefix + "_" + hex.EncodeToString(b)
}
