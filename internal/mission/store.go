package mission

import "context"

// Store defines the persistence interface for mission state.
// All operations must be safe for concurrent use.
type Store interface {
	// Mission CRUD.
	CreateMission(ctx context.Context, m *Mission) error
	GetMission(ctx context.Context, id string) (*Mission, error)
	UpdateMission(ctx context.Context, m *Mission) error
	ListMissions(ctx context.Context) ([]*Mission, error)

	// Task CRUD.
	CreateTask(ctx context.Context, t *Task) error
	GetTask(ctx context.Context, id string) (*Task, error)
	UpdateTask(ctx context.Context, t *Task) error
	ListTasks(ctx context.Context, missionID string) ([]*Task, error)

	// Dependencies.
	AddDependency(ctx context.Context, dep TaskDependency) error
	ListDependencies(ctx context.Context, missionID string) ([]TaskDependency, error)

	// Runs.
	CreateRun(ctx context.Context, r *Run) error
	// CreateRunExclusive atomically creates a run only if no other run with the
	// same task_id and mode is currently in RunRunning status. Returns true if
	// the run was created, false if an active run already exists.
	CreateRunExclusive(ctx context.Context, r *Run) (bool, error)
	GetRun(ctx context.Context, id string) (*Run, error)
	UpdateRun(ctx context.Context, r *Run) error
	ListRuns(ctx context.Context, missionID string) ([]*Run, error)

	// Events (append-only log).
	AppendEvent(ctx context.Context, e *Event) error
	ListEvents(ctx context.Context, missionID string, limit int) ([]*Event, error)

	// Artifacts.
	CreateArtifact(ctx context.Context, a *Artifact) error
	ListArtifacts(ctx context.Context, missionID string) ([]*Artifact, error)

	// Approvals.
	CreateApproval(ctx context.Context, a *Approval) error
	GetApproval(ctx context.Context, id string) (*Approval, error)
	UpdateApproval(ctx context.Context, a *Approval) error
	ListApprovals(ctx context.Context, missionID string) ([]*Approval, error)

	// Aggregate queries.
	GetMissionSummary(ctx context.Context, missionID string) (*MissionSummary, error)
	GetReadyTasks(ctx context.Context, missionID string) ([]*Task, error)
	GetTasksByStatus(ctx context.Context, missionID string, status TaskStatus) ([]*Task, error)
	GetRunsForTask(ctx context.Context, taskID string) ([]*Run, error)

	// Lifecycle.
	Close() error
}
