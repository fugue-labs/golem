package mission

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store backed by a SQLite database.
type SQLiteStore struct {
	db *sql.DB
	mu sync.RWMutex
}

// OpenSQLiteStore opens or creates a mission database at the given path.
func OpenSQLiteStore(dbPath string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Set connection limits for single-writer mode.
	db.SetMaxOpenConns(1)

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *SQLiteStore) migrate() error {
	_, err := s.db.Exec(schema)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS schema_version (
	version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS missions (
	id              TEXT PRIMARY KEY,
	title           TEXT NOT NULL,
	goal            TEXT NOT NULL,
	repo_root       TEXT NOT NULL DEFAULT '',
	base_commit     TEXT NOT NULL DEFAULT '',
	base_branch     TEXT NOT NULL DEFAULT '',
	status          TEXT NOT NULL DEFAULT 'draft',
	policy_json     TEXT NOT NULL DEFAULT '{}',
	budget_json     TEXT NOT NULL DEFAULT '{}',
	success_criteria_json TEXT NOT NULL DEFAULT '[]',
	integration_ref TEXT NOT NULL DEFAULT '',
	created_at      TEXT NOT NULL,
	updated_at      TEXT NOT NULL,
	started_at      TEXT,
	ended_at        TEXT,
	last_replan_at  TEXT
);

CREATE TABLE IF NOT EXISTS tasks (
	id                    TEXT PRIMARY KEY,
	mission_id            TEXT NOT NULL REFERENCES missions(id),
	title                 TEXT NOT NULL,
	kind                  TEXT NOT NULL DEFAULT 'code',
	objective             TEXT NOT NULL DEFAULT '',
	status                TEXT NOT NULL DEFAULT 'pending',
	priority              INTEGER NOT NULL DEFAULT 0,
	scope_json            TEXT NOT NULL DEFAULT '{}',
	acceptance_criteria_json TEXT NOT NULL DEFAULT '[]',
	review_requirements_json TEXT NOT NULL DEFAULT '{}',
	estimated_effort      TEXT NOT NULL DEFAULT '',
	risk_level            TEXT NOT NULL DEFAULT 'low',
	attempt_count         INTEGER NOT NULL DEFAULT 0,
	blocking_reason       TEXT NOT NULL DEFAULT '',
	created_at            TEXT NOT NULL,
	updated_at            TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tasks_mission ON tasks(mission_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(mission_id, status);

CREATE TABLE IF NOT EXISTS task_dependencies (
	task_id       TEXT NOT NULL REFERENCES tasks(id),
	depends_on_id TEXT NOT NULL REFERENCES tasks(id),
	PRIMARY KEY (task_id, depends_on_id)
);

CREATE TABLE IF NOT EXISTS runs (
	id              TEXT PRIMARY KEY,
	mission_id      TEXT NOT NULL REFERENCES missions(id),
	task_id         TEXT NOT NULL DEFAULT '',
	mode            TEXT NOT NULL,
	status          TEXT NOT NULL DEFAULT 'queued',
	lease_owner     TEXT NOT NULL DEFAULT '',
	lease_expires_at TEXT,
	heartbeat_at    TEXT,
	worktree_path   TEXT NOT NULL DEFAULT '',
	started_at      TEXT,
	ended_at        TEXT,
	summary         TEXT NOT NULL DEFAULT '',
	error_text      TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_runs_mission ON runs(mission_id);
CREATE INDEX IF NOT EXISTS idx_runs_task ON runs(task_id);

CREATE TABLE IF NOT EXISTS artifacts (
	id            TEXT PRIMARY KEY,
	mission_id    TEXT NOT NULL REFERENCES missions(id),
	task_id       TEXT NOT NULL DEFAULT '',
	run_id        TEXT NOT NULL DEFAULT '',
	type          TEXT NOT NULL,
	relative_path TEXT NOT NULL,
	sha256        TEXT NOT NULL DEFAULT '',
	created_at    TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_artifacts_mission ON artifacts(mission_id);

CREATE TABLE IF NOT EXISTS approvals (
	id            TEXT PRIMARY KEY,
	mission_id    TEXT NOT NULL REFERENCES missions(id),
	task_id       TEXT NOT NULL DEFAULT '',
	run_id        TEXT NOT NULL DEFAULT '',
	kind          TEXT NOT NULL,
	status        TEXT NOT NULL DEFAULT 'pending',
	request_json  TEXT NOT NULL DEFAULT '{}',
	response_json TEXT NOT NULL DEFAULT '{}',
	created_at    TEXT NOT NULL,
	resolved_at   TEXT
);

CREATE INDEX IF NOT EXISTS idx_approvals_mission ON approvals(mission_id);

CREATE TABLE IF NOT EXISTS events (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	mission_id   TEXT NOT NULL REFERENCES missions(id),
	task_id      TEXT NOT NULL DEFAULT '',
	run_id       TEXT NOT NULL DEFAULT '',
	type         TEXT NOT NULL,
	payload_json TEXT NOT NULL DEFAULT '{}',
	created_at   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_mission ON events(mission_id);
`

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// --- Mission operations ---

func (s *SQLiteStore) CreateMission(ctx context.Context, m *Mission) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	policyJSON := marshalOrDefault(m.Policy, "{}")
	budgetJSON, _ := json.Marshal(m.Budget)
	criteriaJSON, _ := json.Marshal(m.SuccessCriteria)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO missions (id, title, goal, repo_root, base_commit, base_branch, status,
			policy_json, budget_json, success_criteria_json, integration_ref,
			created_at, updated_at, started_at, ended_at, last_replan_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.Title, m.Goal, m.RepoRoot, m.BaseCommit, m.BaseBranch, string(m.Status),
		string(policyJSON), string(budgetJSON), string(criteriaJSON), m.IntegrationRef,
		formatTime(m.CreatedAt), formatTime(m.UpdatedAt),
		formatTimePtr(m.StartedAt), formatTimePtr(m.EndedAt), formatTimePtr(m.LastReplanAt),
	)
	return err
}

func (s *SQLiteStore) GetMission(ctx context.Context, id string) (*Mission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRowContext(ctx,
		`SELECT id, title, goal, repo_root, base_commit, base_branch, status,
			policy_json, budget_json, success_criteria_json, integration_ref,
			created_at, updated_at, started_at, ended_at, last_replan_at
		FROM missions WHERE id = ?`, id)

	return scanMission(row)
}

func (s *SQLiteStore) UpdateMission(ctx context.Context, m *Mission) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	policyJSON := marshalOrDefault(m.Policy, "{}")
	budgetJSON, _ := json.Marshal(m.Budget)
	criteriaJSON, _ := json.Marshal(m.SuccessCriteria)

	_, err := s.db.ExecContext(ctx,
		`UPDATE missions SET title=?, goal=?, repo_root=?, base_commit=?, base_branch=?, status=?,
			policy_json=?, budget_json=?, success_criteria_json=?, integration_ref=?,
			updated_at=?, started_at=?, ended_at=?, last_replan_at=?
		WHERE id=?`,
		m.Title, m.Goal, m.RepoRoot, m.BaseCommit, m.BaseBranch, string(m.Status),
		string(policyJSON), string(budgetJSON), string(criteriaJSON), m.IntegrationRef,
		formatTime(m.UpdatedAt), formatTimePtr(m.StartedAt), formatTimePtr(m.EndedAt),
		formatTimePtr(m.LastReplanAt),
		m.ID,
	)
	return err
}

func (s *SQLiteStore) ListMissions(ctx context.Context) ([]*Mission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, goal, repo_root, base_commit, base_branch, status,
			policy_json, budget_json, success_criteria_json, integration_ref,
			created_at, updated_at, started_at, ended_at, last_replan_at
		FROM missions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var missions []*Mission
	for rows.Next() {
		m, err := scanMissionRows(rows)
		if err != nil {
			return nil, err
		}
		missions = append(missions, m)
	}
	return missions, rows.Err()
}

// --- Task operations ---

func (s *SQLiteStore) CreateTask(ctx context.Context, t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	scopeJSON, _ := json.Marshal(t.Scope)
	criteriaJSON, _ := json.Marshal(t.AcceptanceCriteria)
	reviewJSON := marshalOrDefault(t.ReviewRequirements, "{}")

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tasks (id, mission_id, title, kind, objective, status, priority,
			scope_json, acceptance_criteria_json, review_requirements_json,
			estimated_effort, risk_level, attempt_count, blocking_reason,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.MissionID, t.Title, string(t.Kind), t.Objective, string(t.Status), t.Priority,
		string(scopeJSON), string(criteriaJSON), string(reviewJSON),
		t.EstimatedEffort, string(t.RiskLevel), t.AttemptCount, t.BlockingReason,
		formatTime(t.CreatedAt), formatTime(t.UpdatedAt),
	)
	return err
}

func (s *SQLiteStore) GetTask(ctx context.Context, id string) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRowContext(ctx,
		`SELECT id, mission_id, title, kind, objective, status, priority,
			scope_json, acceptance_criteria_json, review_requirements_json,
			estimated_effort, risk_level, attempt_count, blocking_reason,
			created_at, updated_at
		FROM tasks WHERE id = ?`, id)

	return scanTask(row)
}

func (s *SQLiteStore) UpdateTask(ctx context.Context, t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	scopeJSON, _ := json.Marshal(t.Scope)
	criteriaJSON, _ := json.Marshal(t.AcceptanceCriteria)
	reviewJSON := marshalOrDefault(t.ReviewRequirements, "{}")

	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET title=?, kind=?, objective=?, status=?, priority=?,
			scope_json=?, acceptance_criteria_json=?, review_requirements_json=?,
			estimated_effort=?, risk_level=?, attempt_count=?, blocking_reason=?,
			updated_at=?
		WHERE id=?`,
		t.Title, string(t.Kind), t.Objective, string(t.Status), t.Priority,
		string(scopeJSON), string(criteriaJSON), string(reviewJSON),
		t.EstimatedEffort, string(t.RiskLevel), t.AttemptCount, t.BlockingReason,
		formatTime(t.UpdatedAt),
		t.ID,
	)
	return err
}

func (s *SQLiteStore) ListTasks(ctx context.Context, missionID string) ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, mission_id, title, kind, objective, status, priority,
			scope_json, acceptance_criteria_json, review_requirements_json,
			estimated_effort, risk_level, attempt_count, blocking_reason,
			created_at, updated_at
		FROM tasks WHERE mission_id = ? ORDER BY priority DESC, created_at ASC`, missionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		t, err := scanTaskRows(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// --- Dependency operations ---

func (s *SQLiteStore) AddDependency(ctx context.Context, dep TaskDependency) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO task_dependencies (task_id, depends_on_id) VALUES (?, ?)`,
		dep.TaskID, dep.DependsOnID)
	return err
}

func (s *SQLiteStore) ListDependencies(ctx context.Context, missionID string) ([]TaskDependency, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT d.task_id, d.depends_on_id
		FROM task_dependencies d
		JOIN tasks t ON d.task_id = t.id
		WHERE t.mission_id = ?`, missionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []TaskDependency
	for rows.Next() {
		var d TaskDependency
		if err := rows.Scan(&d.TaskID, &d.DependsOnID); err != nil {
			return nil, err
		}
		deps = append(deps, d)
	}
	return deps, rows.Err()
}

// --- Run operations ---

func (s *SQLiteStore) CreateRun(ctx context.Context, r *Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO runs (id, mission_id, task_id, mode, status, lease_owner,
			lease_expires_at, heartbeat_at, worktree_path, started_at, ended_at,
			summary, error_text)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.MissionID, r.TaskID, string(r.Mode), string(r.Status), r.LeaseOwner,
		formatTimePtr(r.LeaseExpires), formatTimePtr(r.HeartbeatAt), r.WorktreePath,
		formatTimePtr(r.StartedAt), formatTimePtr(r.EndedAt), r.Summary, r.ErrorText,
	)
	return err
}

func (s *SQLiteStore) GetRun(ctx context.Context, id string) (*Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRowContext(ctx,
		`SELECT id, mission_id, task_id, mode, status, lease_owner,
			lease_expires_at, heartbeat_at, worktree_path, started_at, ended_at,
			summary, error_text
		FROM runs WHERE id = ?`, id)

	return scanRun(row)
}

func (s *SQLiteStore) UpdateRun(ctx context.Context, r *Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`UPDATE runs SET task_id=?, mode=?, status=?, lease_owner=?,
			lease_expires_at=?, heartbeat_at=?, worktree_path=?,
			started_at=?, ended_at=?, summary=?, error_text=?
		WHERE id=?`,
		r.TaskID, string(r.Mode), string(r.Status), r.LeaseOwner,
		formatTimePtr(r.LeaseExpires), formatTimePtr(r.HeartbeatAt), r.WorktreePath,
		formatTimePtr(r.StartedAt), formatTimePtr(r.EndedAt), r.Summary, r.ErrorText,
		r.ID,
	)
	return err
}

func (s *SQLiteStore) ListRuns(ctx context.Context, missionID string) ([]*Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, mission_id, task_id, mode, status, lease_owner,
			lease_expires_at, heartbeat_at, worktree_path, started_at, ended_at,
			summary, error_text
		FROM runs WHERE mission_id = ? ORDER BY rowid ASC`, missionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*Run
	for rows.Next() {
		r, err := scanRunRows(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// --- Event operations ---

func (s *SQLiteStore) AppendEvent(ctx context.Context, e *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	payloadJSON := marshalOrDefault(e.PayloadJSON, "{}")
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO events (mission_id, task_id, run_id, type, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		e.MissionID, e.TaskID, e.RunID, e.Type, string(payloadJSON), formatTime(e.CreatedAt),
	)
	return err
}

func (s *SQLiteStore) ListEvents(ctx context.Context, missionID string, limit int) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	q := `SELECT id, mission_id, task_id, run_id, type, payload_json, created_at
		FROM events WHERE mission_id = ? ORDER BY id DESC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.QueryContext(ctx, q, missionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		var e Event
		var createdStr string
		var payloadStr string
		if err := rows.Scan(&e.ID, &e.MissionID, &e.TaskID, &e.RunID, &e.Type, &payloadStr, &createdStr); err != nil {
			return nil, err
		}
		e.PayloadJSON = json.RawMessage(payloadStr)
		e.CreatedAt = parseTime(createdStr)
		events = append(events, &e)
	}
	return events, rows.Err()
}

// --- Artifact operations ---

func (s *SQLiteStore) CreateArtifact(ctx context.Context, a *Artifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO artifacts (id, mission_id, task_id, run_id, type, relative_path, sha256, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.MissionID, a.TaskID, a.RunID, a.Type, a.RelativePath, a.SHA256, formatTime(a.CreatedAt),
	)
	return err
}

func (s *SQLiteStore) ListArtifacts(ctx context.Context, missionID string) ([]*Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, mission_id, task_id, run_id, type, relative_path, sha256, created_at
		FROM artifacts WHERE mission_id = ? ORDER BY rowid ASC`, missionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []*Artifact
	for rows.Next() {
		var a Artifact
		var createdStr string
		if err := rows.Scan(&a.ID, &a.MissionID, &a.TaskID, &a.RunID, &a.Type, &a.RelativePath, &a.SHA256, &createdStr); err != nil {
			return nil, err
		}
		a.CreatedAt = parseTime(createdStr)
		artifacts = append(artifacts, &a)
	}
	return artifacts, rows.Err()
}

// --- Approval operations ---

func (s *SQLiteStore) CreateApproval(ctx context.Context, a *Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	reqJSON := marshalOrDefault(a.RequestJSON, "{}")
	respJSON := marshalOrDefault(a.ResponseJSON, "{}")

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO approvals (id, mission_id, task_id, run_id, kind, status,
			request_json, response_json, created_at, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.MissionID, a.TaskID, a.RunID, a.Kind, string(a.Status),
		string(reqJSON), string(respJSON), formatTime(a.CreatedAt), formatTimePtr(a.ResolvedAt),
	)
	return err
}

func (s *SQLiteStore) GetApproval(ctx context.Context, id string) (*Approval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var a Approval
	var reqStr, respStr, createdStr string
	var resolvedStr sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, mission_id, task_id, run_id, kind, status, request_json, response_json, created_at, resolved_at
		FROM approvals WHERE id = ?`, id).
		Scan(&a.ID, &a.MissionID, &a.TaskID, &a.RunID, &a.Kind, &a.Status,
			&reqStr, &respStr, &createdStr, &resolvedStr)
	if err != nil {
		return nil, err
	}
	a.RequestJSON = json.RawMessage(reqStr)
	a.ResponseJSON = json.RawMessage(respStr)
	a.CreatedAt = parseTime(createdStr)
	if resolvedStr.Valid {
		t := parseTime(resolvedStr.String)
		a.ResolvedAt = &t
	}
	return &a, nil
}

func (s *SQLiteStore) UpdateApproval(ctx context.Context, a *Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	respJSON := marshalOrDefault(a.ResponseJSON, "{}")

	_, err := s.db.ExecContext(ctx,
		`UPDATE approvals SET status=?, response_json=?, resolved_at=? WHERE id=?`,
		string(a.Status), string(respJSON), formatTimePtr(a.ResolvedAt), a.ID)
	return err
}

func (s *SQLiteStore) ListApprovals(ctx context.Context, missionID string) ([]*Approval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, mission_id, task_id, run_id, kind, status, request_json, response_json, created_at, resolved_at
		FROM approvals WHERE mission_id = ? ORDER BY rowid ASC`, missionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var approvals []*Approval
	for rows.Next() {
		var a Approval
		var reqStr, respStr, createdStr string
		var resolvedStr sql.NullString
		if err := rows.Scan(&a.ID, &a.MissionID, &a.TaskID, &a.RunID, &a.Kind, &a.Status,
			&reqStr, &respStr, &createdStr, &resolvedStr); err != nil {
			return nil, err
		}
		a.RequestJSON = json.RawMessage(reqStr)
		a.ResponseJSON = json.RawMessage(respStr)
		a.CreatedAt = parseTime(createdStr)
		if resolvedStr.Valid {
			t := parseTime(resolvedStr.String)
			a.ResolvedAt = &t
		}
		approvals = append(approvals, &a)
	}
	return approvals, rows.Err()
}

// --- Aggregate queries ---

func (s *SQLiteStore) GetMissionSummary(ctx context.Context, missionID string) (*MissionSummary, error) {
	m, err := s.GetMission(ctx, missionID)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var counts TaskCounts
	rows, err := s.db.QueryContext(ctx,
		`SELECT status, COUNT(*) FROM tasks WHERE mission_id = ? GROUP BY status`, missionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts.Total += count
		switch TaskStatus(status) {
		case TaskPending:
			counts.Pending = count
		case TaskReady:
			counts.Ready = count
		case TaskRunning:
			counts.Running = count
		case TaskAwaitingReview:
			counts.AwaitingReview = count
		case TaskAccepted:
			counts.Accepted = count
		case TaskIntegrated:
			counts.Integrated = count
		case TaskDone:
			counts.Done = count
		case TaskBlocked:
			counts.Blocked = count
		case TaskFailed:
			counts.Failed = count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var activeRuns int
	s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM runs WHERE mission_id = ? AND status IN ('queued', 'running')`,
		missionID).Scan(&activeRuns)

	var pendingApprovals int
	s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM approvals WHERE mission_id = ? AND status = 'pending'`,
		missionID).Scan(&pendingApprovals)

	return &MissionSummary{
		Mission:          m,
		TaskCounts:       counts,
		ActiveRuns:       activeRuns,
		PendingApprovals: pendingApprovals,
	}, nil
}

func (s *SQLiteStore) GetReadyTasks(ctx context.Context, missionID string) ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// A task is "ready" if its status is "ready" — the controller is responsible
	// for transitioning tasks from pending to ready when deps are satisfied.
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, mission_id, title, kind, objective, status, priority,
			scope_json, acceptance_criteria_json, review_requirements_json,
			estimated_effort, risk_level, attempt_count, blocking_reason,
			created_at, updated_at
		FROM tasks WHERE mission_id = ? AND status = 'ready'
		ORDER BY priority DESC, created_at ASC`, missionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		t, err := scanTaskRows(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// --- Scan helpers ---

type scanner interface {
	Scan(dest ...any) error
}

func scanMission(row scanner) (*Mission, error) {
	var m Mission
	var policyStr, budgetStr, criteriaStr string
	var createdStr, updatedStr string
	var startedStr, endedStr, replanStr sql.NullString

	err := row.Scan(&m.ID, &m.Title, &m.Goal, &m.RepoRoot, &m.BaseCommit, &m.BaseBranch,
		&m.Status, &policyStr, &budgetStr, &criteriaStr, &m.IntegrationRef,
		&createdStr, &updatedStr, &startedStr, &endedStr, &replanStr)
	if err != nil {
		return nil, err
	}

	m.Policy = json.RawMessage(policyStr)
	json.Unmarshal([]byte(budgetStr), &m.Budget)
	json.Unmarshal([]byte(criteriaStr), &m.SuccessCriteria)
	m.CreatedAt = parseTime(createdStr)
	m.UpdatedAt = parseTime(updatedStr)
	if startedStr.Valid {
		t := parseTime(startedStr.String)
		m.StartedAt = &t
	}
	if endedStr.Valid {
		t := parseTime(endedStr.String)
		m.EndedAt = &t
	}
	if replanStr.Valid {
		t := parseTime(replanStr.String)
		m.LastReplanAt = &t
	}
	return &m, nil
}

func scanMissionRows(rows *sql.Rows) (*Mission, error) {
	return scanMission(rows)
}

func scanTask(row scanner) (*Task, error) {
	var t Task
	var scopeStr, criteriaStr, reviewStr string
	var createdStr, updatedStr string

	err := row.Scan(&t.ID, &t.MissionID, &t.Title, &t.Kind, &t.Objective,
		&t.Status, &t.Priority, &scopeStr, &criteriaStr, &reviewStr,
		&t.EstimatedEffort, &t.RiskLevel, &t.AttemptCount, &t.BlockingReason,
		&createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(scopeStr), &t.Scope)
	json.Unmarshal([]byte(criteriaStr), &t.AcceptanceCriteria)
	t.ReviewRequirements = json.RawMessage(reviewStr)
	t.CreatedAt = parseTime(createdStr)
	t.UpdatedAt = parseTime(updatedStr)
	return &t, nil
}

func scanTaskRows(rows *sql.Rows) (*Task, error) {
	return scanTask(rows)
}

func scanRun(row scanner) (*Run, error) {
	var r Run
	var leaseStr, heartbeatStr, startedStr, endedStr sql.NullString

	err := row.Scan(&r.ID, &r.MissionID, &r.TaskID, &r.Mode, &r.Status, &r.LeaseOwner,
		&leaseStr, &heartbeatStr, &r.WorktreePath, &startedStr, &endedStr,
		&r.Summary, &r.ErrorText)
	if err != nil {
		return nil, err
	}

	if leaseStr.Valid {
		t := parseTime(leaseStr.String)
		r.LeaseExpires = &t
	}
	if heartbeatStr.Valid {
		t := parseTime(heartbeatStr.String)
		r.HeartbeatAt = &t
	}
	if startedStr.Valid {
		t := parseTime(startedStr.String)
		r.StartedAt = &t
	}
	if endedStr.Valid {
		t := parseTime(endedStr.String)
		r.EndedAt = &t
	}
	return &r, nil
}

func scanRunRows(rows *sql.Rows) (*Run, error) {
	return scanRun(rows)
}

// --- Time helpers ---

const timeFmt = time.RFC3339Nano

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(timeFmt)
}

func formatTimePtr(t *time.Time) sql.NullString {
	if t == nil || t.IsZero() {
		return sql.NullString{}
	}
	return sql.NullString{String: t.Format(timeFmt), Valid: true}
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(timeFmt, s)
	return t
}

func marshalOrDefault(raw json.RawMessage, def string) string {
	if len(raw) == 0 {
		return def
	}
	return string(raw)
}
