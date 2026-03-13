// Package mission implements the multi-agent mission orchestration subsystem.
// It provides task scheduling, worktree management, worker lifecycle, and
// durable event logging for concurrent bounded-task execution.
package mission

import (
	"encoding/json"
	"time"
)

// MissionStatus represents the lifecycle state of a mission.
type MissionStatus string

const (
	MissionDraft            MissionStatus = "draft"
	MissionPlanning         MissionStatus = "planning"
	MissionAwaitingApproval MissionStatus = "awaiting_approval"
	MissionRunning          MissionStatus = "running"
	MissionPaused           MissionStatus = "paused"
	MissionBlocked          MissionStatus = "blocked"
	MissionCompleting       MissionStatus = "completing"
	MissionCompleted        MissionStatus = "completed"
	MissionFailed           MissionStatus = "failed"
	MissionCancelled        MissionStatus = "cancelled"
)

// Mission is a top-level goal with policy, budget, and success criteria.
type Mission struct {
	ID              string            `json:"id"`
	Title           string            `json:"title"`
	Goal            string            `json:"goal"`
	RepoRoot        string            `json:"repo_root"`
	BaseCommit      string            `json:"base_commit"`
	BaseBranch      string            `json:"base_branch"`
	Status          MissionStatus     `json:"status"`
	Policy          MissionPolicy     `json:"policy"`
	Budget          MissionBudget     `json:"budget"`
	SuccessCriteria []string          `json:"success_criteria"`
	IntegrationRef  string            `json:"integration_ref"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	StartedAt       *time.Time        `json:"started_at,omitempty"`
	EndedAt         *time.Time        `json:"ended_at,omitempty"`
	LastReplanAt    *time.Time        `json:"last_replan_at,omitempty"`
}

// MissionPolicy configures mission-level behavior constraints.
type MissionPolicy struct {
	RequireApprovalBeforeStart bool `json:"require_approval_before_start"`
	RequireReviewBeforeIntegration bool `json:"require_review_before_integration"`
	AllowDirtyRepo             bool `json:"allow_dirty_repo"`
	AutoApplyLocalReplans      bool `json:"auto_apply_local_replans"`
}

// MissionBudget defines resource limits for a mission.
type MissionBudget struct {
	MaxConcurrentWorkers   int           `json:"max_concurrent_workers"`
	MaxTotalRuns           int           `json:"max_total_runs"`
	MaxReplans             int           `json:"max_replans"`
	MaxConsecutiveFailures int           `json:"max_consecutive_failures"`
	MaxWallClock           time.Duration `json:"max_wall_clock"`
}

// DefaultBudget returns sensible defaults for mission budgets.
func DefaultBudget() MissionBudget {
	return MissionBudget{
		MaxConcurrentWorkers:   3,
		MaxTotalRuns:           50,
		MaxReplans:             5,
		MaxConsecutiveFailures: 3,
		MaxWallClock:           2 * time.Hour,
	}
}

// DefaultPolicy returns the default mission policy.
func DefaultPolicy() MissionPolicy {
	return MissionPolicy{
		RequireApprovalBeforeStart:     false,
		RequireReviewBeforeIntegration: true,
		AllowDirtyRepo:                 false,
		AutoApplyLocalReplans:          true,
	}
}

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

const (
	TaskPending        TaskStatus = "pending"
	TaskReady          TaskStatus = "ready"
	TaskLeased         TaskStatus = "leased"
	TaskRunning        TaskStatus = "running"
	TaskAwaitingReview TaskStatus = "awaiting_review"
	TaskAccepted       TaskStatus = "accepted"
	TaskIntegrated     TaskStatus = "integrated"
	TaskDone           TaskStatus = "done"
	TaskRejected       TaskStatus = "rejected"
	TaskFailed         TaskStatus = "failed"
	TaskBlocked        TaskStatus = "blocked"
	TaskSuperseded     TaskStatus = "superseded"
)

// TaskKind categorizes what a task does.
type TaskKind string

const (
	TaskKindCode              TaskKind = "code"
	TaskKindTest              TaskKind = "test"
	TaskKindDocs              TaskKind = "docs"
	TaskKindInvestigation     TaskKind = "investigation"
	TaskKindIntegrationFollowup TaskKind = "integration_followup"
	TaskKindReviewFix         TaskKind = "review_fix"
)

// RiskLevel indicates the risk level of a task.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// TaskScope defines the writable file/component ownership boundary.
type TaskScope struct {
	Paths      []string `json:"paths,omitempty"`
	Components []string `json:"components,omitempty"`
	RepoWide   bool     `json:"repo_wide,omitempty"`
}

// Overlaps returns true if this scope overlaps with another scope.
func (s TaskScope) Overlaps(other TaskScope) bool {
	if s.RepoWide || other.RepoWide {
		return true
	}
	// Check path overlap using simple prefix/glob matching.
	for _, p1 := range s.Paths {
		for _, p2 := range other.Paths {
			if pathsOverlap(p1, p2) {
				return true
			}
		}
	}
	// Check component overlap.
	for _, c1 := range s.Components {
		for _, c2 := range other.Components {
			if c1 == c2 {
				return true
			}
		}
	}
	return false
}

// Task is one bounded unit of mission work.
type Task struct {
	ID                   string   `json:"id"`
	MissionID            string   `json:"mission_id"`
	Title                string   `json:"title"`
	Kind                 TaskKind `json:"kind"`
	Objective            string   `json:"objective"`
	Status               TaskStatus `json:"status"`
	Priority             int      `json:"priority"`
	Scope                TaskScope `json:"scope"`
	AcceptanceCriteria   []string `json:"acceptance_criteria"`
	ReviewRequirements   []string `json:"review_requirements"`
	EstimatedEffort      string   `json:"estimated_effort"`
	RiskLevel            RiskLevel `json:"risk_level"`
	AttemptCount         int      `json:"attempt_count"`
	BlockingReason       string   `json:"blocking_reason,omitempty"`
	Dependencies         []string `json:"dependencies,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// RunMode indicates what kind of execution a run performs.
type RunMode string

const (
	RunModePlanner     RunMode = "planner"
	RunModeWorker      RunMode = "worker"
	RunModeReview      RunMode = "review"
	RunModeIntegration RunMode = "integration"
)

// RunStatus represents the lifecycle state of a run.
type RunStatus string

const (
	RunQueued    RunStatus = "queued"
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
	RunTimedOut  RunStatus = "timed_out"
	RunCancelled RunStatus = "cancelled"
	RunLeaseLost RunStatus = "lease_lost"
)

// Run is one actual execution attempt against one task.
type Run struct {
	ID             string    `json:"id"`
	MissionID      string    `json:"mission_id"`
	TaskID         string    `json:"task_id"`
	Mode           RunMode   `json:"mode"`
	Status         RunStatus `json:"status"`
	LeaseOwner     string    `json:"lease_owner,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	HeartbeatAt    *time.Time `json:"heartbeat_at,omitempty"`
	WorktreePath   string    `json:"worktree_path"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	Summary        string    `json:"summary,omitempty"`
	ErrorText      string    `json:"error_text,omitempty"`
}

// EventType categorizes mission events.
type EventType string

const (
	// Mission lifecycle events.
	EventMissionCreated           EventType = "mission_created"
	EventMissionPlanningStarted   EventType = "mission_planning_started"
	EventMissionPlanningCompleted EventType = "mission_planning_completed"
	EventMissionStarted           EventType = "mission_started"
	EventMissionPaused            EventType = "mission_paused"
	EventMissionResumed           EventType = "mission_resumed"
	EventMissionBlocked           EventType = "mission_blocked"
	EventMissionCompleted         EventType = "mission_completed"
	EventMissionFailed            EventType = "mission_failed"
	EventMissionCancelled         EventType = "mission_cancelled"

	// Task lifecycle events.
	EventTaskCreated        EventType = "task_created"
	EventTaskReady          EventType = "task_ready"
	EventTaskBlocked        EventType = "task_blocked"
	EventTaskLeased         EventType = "task_leased"
	EventTaskRequeued       EventType = "task_requeued"
	EventTaskAwaitingReview EventType = "task_awaiting_review"
	EventTaskAccepted       EventType = "task_accepted"
	EventTaskRejected       EventType = "task_rejected"
	EventTaskDone           EventType = "task_done"
	EventTaskSuperseded     EventType = "task_superseded"

	// Run lifecycle events.
	EventRunCreated   EventType = "run_created"
	EventRunStarted   EventType = "run_started"
	EventRunHeartbeat EventType = "run_heartbeat"
	EventRunSucceeded EventType = "run_succeeded"
	EventRunFailed    EventType = "run_failed"
	EventRunCancelled EventType = "run_cancelled"
	EventRunTimedOut  EventType = "run_timed_out"
	EventRunLeaseLost EventType = "run_lease_lost"

	// Recovery events.
	EventRecoveryStarted       EventType = "recovery_started"
	EventRecoveryReconciledRun EventType = "recovery_reconciled_run"
	EventRecoveryCompleted     EventType = "recovery_completed"
)

// Event is a durable log entry for mission activity.
type Event struct {
	ID        int64           `json:"id"`
	MissionID string          `json:"mission_id"`
	TaskID    string          `json:"task_id,omitempty"`
	RunID     string          `json:"run_id,omitempty"`
	Type      EventType       `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// WorkerCard is a snapshot of worker state for TUI display.
type WorkerCard struct {
	RunID        string    `json:"run_id"`
	TaskID       string    `json:"task_id"`
	TaskTitle    string    `json:"task_title"`
	Status       RunStatus `json:"status"`
	WorktreePath string    `json:"worktree_path"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	LastEvent    string    `json:"last_event,omitempty"`
}
