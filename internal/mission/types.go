// Package mission provides goal planning and validation for multi-step agent
// workflows, with persistent storage of mission state.
package mission

import (
	"encoding/json"
	"time"
)

// MissionStatus represents the lifecycle phase of a mission.
type MissionStatus string

const (
	MissionDraft            MissionStatus = "draft"
	MissionPlanning         MissionStatus = "planning"
	MissionAwaitingApproval MissionStatus = "awaiting_approval"
	MissionRunning          MissionStatus = "running"
	MissionBlocked          MissionStatus = "blocked"
	MissionPaused           MissionStatus = "paused"
	MissionCompleting       MissionStatus = "completing"
	MissionCompleted        MissionStatus = "completed"
	MissionFailed           MissionStatus = "failed"
	MissionCancelled        MissionStatus = "cancelled"
)

// IsTerminal returns true if the mission is in a terminal state.
func (s MissionStatus) IsTerminal() bool {
	return s == MissionCompleted || s == MissionFailed || s == MissionCancelled
}

// TaskStatus represents the lifecycle of a task within a mission.
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
	TaskBlocked        TaskStatus = "blocked"
	TaskFailed         TaskStatus = "failed"
	TaskRejected       TaskStatus = "rejected"
)

// TaskKind categorizes the type of work a task represents.
type TaskKind string

const (
	TaskKindCode              TaskKind = "code"
	TaskKindTest              TaskKind = "test"
	TaskKindDocs              TaskKind = "docs"
	TaskKindInvestigation     TaskKind = "investigation"
	TaskKindIntegrationFixup  TaskKind = "integration_followup"
	TaskKindReviewFix         TaskKind = "review_fix"
)

// RunStatus represents the lifecycle of a run.
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

// RunMode categorizes the type of run.
type RunMode string

const (
	RunModePlanner     RunMode = "planner"
	RunModeWorker      RunMode = "worker"
	RunModeReview      RunMode = "review"
	RunModeIntegration RunMode = "integration"
)

// ApprovalStatus represents the state of an approval request.
type ApprovalStatus string

const (
	ApprovalPending    ApprovalStatus = "pending"
	ApprovalApproved   ApprovalStatus = "approved"
	ApprovalRejected   ApprovalStatus = "rejected"
	ApprovalSuperseded ApprovalStatus = "superseded"
	ApprovalExpired    ApprovalStatus = "expired"
)

// RiskLevel categorizes the risk associated with a task.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// Mission represents a high-level objective being orchestrated.
type Mission struct {
	ID              string          `json:"id"`
	Title           string          `json:"title"`
	Goal            string          `json:"goal"`
	RepoRoot        string          `json:"repo_root"`
	BaseCommit      string          `json:"base_commit"`
	BaseBranch      string          `json:"base_branch"`
	Status          MissionStatus   `json:"status"`
	Policy          json.RawMessage `json:"policy,omitempty"`
	Budget          Budget          `json:"budget"`
	SuccessCriteria []string        `json:"success_criteria,omitempty"`
	IntegrationRef  string          `json:"integration_ref,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	EndedAt         *time.Time      `json:"ended_at,omitempty"`
	LastReplanAt    *time.Time      `json:"last_replan_at,omitempty"`
}

// Budget defines resource limits for a mission.
type Budget struct {
	MaxConcurrentWorkers  int           `json:"max_concurrent_workers,omitempty"`
	MaxTotalRuns          int           `json:"max_total_runs,omitempty"`
	MaxModelCalls         int           `json:"max_model_calls,omitempty"`
	MaxCostUSD            float64       `json:"max_cost_usd,omitempty"`
	MaxWallClockDuration  time.Duration `json:"max_wall_clock_duration,omitempty"`
	MaxReplans            int           `json:"max_replans,omitempty"`
	MaxConsecutiveFailures int          `json:"max_consecutive_failures,omitempty"`
}

// Task represents a single unit of work within a mission.
type Task struct {
	ID                 string          `json:"id"`
	MissionID          string          `json:"mission_id"`
	Title              string          `json:"title"`
	Kind               TaskKind        `json:"kind"`
	Objective          string          `json:"objective"`
	Status             TaskStatus      `json:"status"`
	Priority           int             `json:"priority"`
	Scope              TaskScope       `json:"scope"`
	AcceptanceCriteria []string        `json:"acceptance_criteria,omitempty"`
	ReviewRequirements json.RawMessage `json:"review_requirements,omitempty"`
	EstimatedEffort    string          `json:"estimated_effort,omitempty"`
	RiskLevel          RiskLevel       `json:"risk_level"`
	AttemptCount       int             `json:"attempt_count"`
	BlockingReason     string          `json:"blocking_reason,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// TaskScope defines the writable/readable file scope for a task.
type TaskScope struct {
	WritePaths []string `json:"write_paths,omitempty"`
	ReadPaths  []string `json:"read_paths,omitempty"`
}

// TaskDependency represents a dependency between two tasks.
type TaskDependency struct {
	TaskID      string `json:"task_id"`
	DependsOnID string `json:"depends_on_id"`
}

// Run represents a single execution attempt (planner, worker, review, or integration).
type Run struct {
	ID            string     `json:"id"`
	MissionID     string     `json:"mission_id"`
	TaskID        string     `json:"task_id,omitempty"`
	Mode          RunMode    `json:"mode"`
	Status        RunStatus  `json:"status"`
	LeaseOwner    string     `json:"lease_owner,omitempty"`
	LeaseExpires  *time.Time `json:"lease_expires_at,omitempty"`
	HeartbeatAt   *time.Time `json:"heartbeat_at,omitempty"`
	WorktreePath  string     `json:"worktree_path,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	EndedAt       *time.Time `json:"ended_at,omitempty"`
	Summary       string     `json:"summary,omitempty"`
	ErrorText     string     `json:"error_text,omitempty"`
}

// Artifact represents a durable output from a run.
type Artifact struct {
	ID           string    `json:"id"`
	MissionID    string    `json:"mission_id"`
	TaskID       string    `json:"task_id,omitempty"`
	RunID        string    `json:"run_id,omitempty"`
	Type         string    `json:"type"`
	RelativePath string    `json:"relative_path"`
	SHA256       string    `json:"sha256,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// Approval represents a pending or resolved approval gate.
type Approval struct {
	ID          string          `json:"id"`
	MissionID   string          `json:"mission_id"`
	TaskID      string          `json:"task_id,omitempty"`
	RunID       string          `json:"run_id,omitempty"`
	Kind        string          `json:"kind"`
	Status      ApprovalStatus  `json:"status"`
	RequestJSON json.RawMessage `json:"request_json,omitempty"`
	ResponseJSON json.RawMessage `json:"response_json,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	ResolvedAt  *time.Time      `json:"resolved_at,omitempty"`
}

// Event represents an append-only event in the mission log.
type Event struct {
	ID          int64           `json:"id"`
	MissionID   string          `json:"mission_id"`
	TaskID      string          `json:"task_id,omitempty"`
	RunID       string          `json:"run_id,omitempty"`
	Type        string          `json:"type"`
	PayloadJSON json.RawMessage `json:"payload_json,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// CreateMissionRequest contains the parameters for creating a new mission.
type CreateMissionRequest struct {
	Title      string   `json:"title"`
	Goal       string   `json:"goal"`
	RepoRoot   string   `json:"repo_root"`
	BaseCommit string   `json:"base_commit"`
	BaseBranch string   `json:"base_branch"`
	Budget     Budget   `json:"budget,omitempty"`
}

// PlanResult represents the output of a planning run.
type PlanResult struct {
	Summary         string           `json:"summary"`
	SuccessCriteria []string         `json:"success_criteria"`
	Tasks           []PlanTask       `json:"tasks"`
	Dependencies    []TaskDependency `json:"dependencies"`
}

// PlanTask represents a task as proposed by the planner.
type PlanTask struct {
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	Kind               TaskKind  `json:"kind"`
	Objective          string    `json:"objective"`
	Priority           int       `json:"priority"`
	Scope              TaskScope `json:"scope"`
	AcceptanceCriteria []string  `json:"acceptance_criteria,omitempty"`
	EstimatedEffort    string    `json:"estimated_effort,omitempty"`
	RiskLevel          RiskLevel `json:"risk_level"`
}

// MissionSummary provides a concise view of mission state for the TUI.
type MissionSummary struct {
	Mission      *Mission `json:"mission"`
	TaskCounts   TaskCounts `json:"task_counts"`
	ActiveRuns   int      `json:"active_runs"`
	PendingApprovals int  `json:"pending_approvals"`
}

// TaskCounts aggregates task states for display.
type TaskCounts struct {
	Total          int `json:"total"`
	Pending        int `json:"pending"`
	Ready          int `json:"ready"`
	Running        int `json:"running"`
	AwaitingReview int `json:"awaiting_review"`
	Accepted       int `json:"accepted"`
	Integrated     int `json:"integrated"`
	Done           int `json:"done"`
	Blocked        int `json:"blocked"`
	Failed         int `json:"failed"`
}
