package mission

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store provides durable persistence for mission state.
type Store struct {
	db   *sql.DB
	path string
}

// OpenStore opens (or creates) the mission store at the given path.
func OpenStore(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s := &Store{db: db, path: dbPath}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(schema)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS missions (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  goal TEXT NOT NULL,
  repo_root TEXT NOT NULL,
  base_commit TEXT NOT NULL,
  base_branch TEXT NOT NULL,
  status TEXT NOT NULL,
  policy_json TEXT NOT NULL DEFAULT '{}',
  budget_json TEXT NOT NULL DEFAULT '{}',
  success_criteria_json TEXT NOT NULL DEFAULT '[]',
  integration_ref TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  started_at TEXT,
  ended_at TEXT,
  last_replan_at TEXT
);

CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  mission_id TEXT NOT NULL,
  title TEXT NOT NULL,
  kind TEXT NOT NULL,
  objective TEXT NOT NULL,
  status TEXT NOT NULL,
  priority INTEGER NOT NULL DEFAULT 0,
  scope_json TEXT NOT NULL DEFAULT '{}',
  acceptance_criteria_json TEXT NOT NULL DEFAULT '[]',
  review_requirements_json TEXT NOT NULL DEFAULT '[]',
  estimated_effort TEXT NOT NULL DEFAULT '',
  risk_level TEXT NOT NULL DEFAULT 'low',
  attempt_count INTEGER NOT NULL DEFAULT 0,
  blocking_reason TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (mission_id) REFERENCES missions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_tasks_mission_status ON tasks(mission_id, status);
CREATE INDEX IF NOT EXISTS idx_tasks_mission_priority ON tasks(mission_id, priority DESC, created_at ASC);

CREATE TABLE IF NOT EXISTS task_dependencies (
  task_id TEXT NOT NULL,
  depends_on_task_id TEXT NOT NULL,
  PRIMARY KEY (task_id, depends_on_task_id),
  FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
  FOREIGN KEY (depends_on_task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_task_deps_depends_on ON task_dependencies(depends_on_task_id);

CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  mission_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  mode TEXT NOT NULL,
  status TEXT NOT NULL,
  lease_owner TEXT,
  lease_expires_at TEXT,
  heartbeat_at TEXT,
  worktree_path TEXT NOT NULL DEFAULT '',
  started_at TEXT,
  ended_at TEXT,
  summary TEXT,
  error_text TEXT,
  FOREIGN KEY (mission_id) REFERENCES missions(id) ON DELETE CASCADE,
  FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_runs_mission_status ON runs(mission_id, status);
CREATE INDEX IF NOT EXISTS idx_runs_task_id ON runs(task_id);
CREATE INDEX IF NOT EXISTS idx_runs_lease_expires ON runs(lease_expires_at);

CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  mission_id TEXT NOT NULL,
  task_id TEXT,
  run_id TEXT,
  type TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  FOREIGN KEY (mission_id) REFERENCES missions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_events_mission ON events(mission_id, id);
CREATE INDEX IF NOT EXISTS idx_events_task ON events(task_id);
`

// --- Mission CRUD ---

// CreateMission persists a new mission.
func (s *Store) CreateMission(m *Mission) error {
	policyJSON, _ := json.Marshal(m.Policy)
	budgetJSON, _ := json.Marshal(m.Budget)
	criteriaJSON, _ := json.Marshal(m.SuccessCriteria)
	now := time.Now().UTC()
	m.CreatedAt = now
	m.UpdatedAt = now

	_, err := s.db.Exec(`INSERT INTO missions (id, title, goal, repo_root, base_commit, base_branch,
		status, policy_json, budget_json, success_criteria_json, integration_ref,
		created_at, updated_at, started_at, ended_at, last_replan_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.Title, m.Goal, m.RepoRoot, m.BaseCommit, m.BaseBranch,
		string(m.Status), string(policyJSON), string(budgetJSON), string(criteriaJSON),
		m.IntegrationRef, fmtTime(m.CreatedAt), fmtTime(m.UpdatedAt),
		fmtTimePtr(m.StartedAt), fmtTimePtr(m.EndedAt), fmtTimePtr(m.LastReplanAt))
	return err
}

// GetMission loads a mission by ID.
func (s *Store) GetMission(id string) (*Mission, error) {
	row := s.db.QueryRow(`SELECT id, title, goal, repo_root, base_commit, base_branch,
		status, policy_json, budget_json, success_criteria_json, integration_ref,
		created_at, updated_at, started_at, ended_at, last_replan_at
		FROM missions WHERE id = ?`, id)
	return scanMission(row)
}

// UpdateMissionStatus updates the status (and updated_at) of a mission.
func (s *Store) UpdateMissionStatus(id string, status MissionStatus) error {
	now := fmtTime(time.Now().UTC())
	_, err := s.db.Exec(`UPDATE missions SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), now, id)
	return err
}

// ListActiveMissions returns all non-terminal missions.
func (s *Store) ListActiveMissions() ([]*Mission, error) {
	rows, err := s.db.Query(`SELECT id, title, goal, repo_root, base_commit, base_branch,
		status, policy_json, budget_json, success_criteria_json, integration_ref,
		created_at, updated_at, started_at, ended_at, last_replan_at
		FROM missions WHERE status NOT IN ('completed','failed','cancelled')
		ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var missions []*Mission
	for rows.Next() {
		m, err := scanMissionRow(rows)
		if err != nil {
			return nil, err
		}
		missions = append(missions, m)
	}
	return missions, rows.Err()
}

// --- Task CRUD ---

// CreateTask persists a new task and its dependencies.
func (s *Store) CreateTask(t *Task) error {
	scopeJSON, _ := json.Marshal(t.Scope)
	acJSON, _ := json.Marshal(t.AcceptanceCriteria)
	rrJSON, _ := json.Marshal(t.ReviewRequirements)
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO tasks (id, mission_id, title, kind, objective, status,
		priority, scope_json, acceptance_criteria_json, review_requirements_json,
		estimated_effort, risk_level, attempt_count, blocking_reason,
		created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.MissionID, t.Title, string(t.Kind), t.Objective, string(t.Status),
		t.Priority, string(scopeJSON), string(acJSON), string(rrJSON),
		t.EstimatedEffort, string(t.RiskLevel), t.AttemptCount, nilStr(t.BlockingReason),
		fmtTime(t.CreatedAt), fmtTime(t.UpdatedAt))
	if err != nil {
		return err
	}

	for _, depID := range t.Dependencies {
		_, err = tx.Exec(`INSERT OR IGNORE INTO task_dependencies (task_id, depends_on_task_id)
			VALUES (?, ?)`, t.ID, depID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetTask loads a task by ID.
func (s *Store) GetTask(id string) (*Task, error) {
	row := s.db.QueryRow(`SELECT id, mission_id, title, kind, objective, status,
		priority, scope_json, acceptance_criteria_json, review_requirements_json,
		estimated_effort, risk_level, attempt_count, blocking_reason,
		created_at, updated_at
		FROM tasks WHERE id = ?`, id)
	t, err := scanTask(row)
	if err != nil {
		return nil, err
	}
	deps, err := s.getTaskDependencies(t.ID)
	if err != nil {
		return nil, err
	}
	t.Dependencies = deps
	return t, nil
}

// UpdateTaskStatus updates the status of a task.
func (s *Store) UpdateTaskStatus(id string, status TaskStatus, reason string) error {
	now := fmtTime(time.Now().UTC())
	_, err := s.db.Exec(`UPDATE tasks SET status = ?, blocking_reason = ?, updated_at = ? WHERE id = ?`,
		string(status), nilStr(reason), now, id)
	return err
}

// IncrementTaskAttempt increments the attempt counter for a task.
func (s *Store) IncrementTaskAttempt(id string) error {
	_, err := s.db.Exec(`UPDATE tasks SET attempt_count = attempt_count + 1, updated_at = ? WHERE id = ?`,
		fmtTime(time.Now().UTC()), id)
	return err
}

// ListMissionTasks returns all tasks for a mission.
func (s *Store) ListMissionTasks(missionID string) ([]*Task, error) {
	rows, err := s.db.Query(`SELECT id, mission_id, title, kind, objective, status,
		priority, scope_json, acceptance_criteria_json, review_requirements_json,
		estimated_effort, risk_level, attempt_count, blocking_reason,
		created_at, updated_at
		FROM tasks WHERE mission_id = ?
		ORDER BY priority DESC, created_at ASC`, missionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []*Task
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			return nil, err
		}
		deps, err := s.getTaskDependencies(t.ID)
		if err != nil {
			return nil, err
		}
		t.Dependencies = deps
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// ReadyTasks returns tasks that are in "ready" status for a mission.
func (s *Store) ReadyTasks(missionID string) ([]*Task, error) {
	rows, err := s.db.Query(`SELECT id, mission_id, title, kind, objective, status,
		priority, scope_json, acceptance_criteria_json, review_requirements_json,
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
		t, err := scanTaskRow(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (s *Store) getTaskDependencies(taskID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT depends_on_task_id FROM task_dependencies WHERE task_id = ?`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var deps []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		deps = append(deps, d)
	}
	return deps, rows.Err()
}

// --- Run CRUD ---

// CreateRun persists a new run.
func (s *Store) CreateRun(r *Run) error {
	_, err := s.db.Exec(`INSERT INTO runs (id, mission_id, task_id, mode, status,
		lease_owner, lease_expires_at, heartbeat_at, worktree_path,
		started_at, ended_at, summary, error_text)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.MissionID, r.TaskID, string(r.Mode), string(r.Status),
		nilStr(r.LeaseOwner), fmtTimePtr(r.LeaseExpiresAt), fmtTimePtr(r.HeartbeatAt),
		r.WorktreePath, fmtTimePtr(r.StartedAt), fmtTimePtr(r.EndedAt),
		nilStr(r.Summary), nilStr(r.ErrorText))
	return err
}

// UpdateRunStatus updates the status and optional terminal fields of a run.
func (s *Store) UpdateRunStatus(id string, status RunStatus, summary, errorText string) error {
	now := fmtTime(time.Now().UTC())
	var endedAt *string
	if status == RunSucceeded || status == RunFailed || status == RunTimedOut || status == RunCancelled || status == RunLeaseLost {
		endedAt = &now
	}
	_, err := s.db.Exec(`UPDATE runs SET status = ?, summary = ?, error_text = ?, ended_at = COALESCE(?, ended_at) WHERE id = ?`,
		string(status), nilStr(summary), nilStr(errorText), endedAt, id)
	return err
}

// HeartbeatRun extends the lease and updates the heartbeat timestamp.
func (s *Store) HeartbeatRun(id string, leaseExpiry time.Time) error {
	now := fmtTime(time.Now().UTC())
	_, err := s.db.Exec(`UPDATE runs SET heartbeat_at = ?, lease_expires_at = ? WHERE id = ?`,
		now, fmtTime(leaseExpiry), id)
	return err
}

// ActiveRuns returns all non-terminal runs for a mission.
func (s *Store) ActiveRuns(missionID string) ([]*Run, error) {
	rows, err := s.db.Query(`SELECT id, mission_id, task_id, mode, status,
		lease_owner, lease_expires_at, heartbeat_at, worktree_path,
		started_at, ended_at, summary, error_text
		FROM runs WHERE mission_id = ? AND status IN ('queued','running')
		ORDER BY started_at ASC`, missionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []*Run
	for rows.Next() {
		r, err := scanRunRow(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// ExpiredLeaseRuns returns runs with expired leases.
func (s *Store) ExpiredLeaseRuns(missionID string) ([]*Run, error) {
	now := fmtTime(time.Now().UTC())
	rows, err := s.db.Query(`SELECT id, mission_id, task_id, mode, status,
		lease_owner, lease_expires_at, heartbeat_at, worktree_path,
		started_at, ended_at, summary, error_text
		FROM runs WHERE mission_id = ? AND status = 'running'
		AND lease_expires_at IS NOT NULL AND lease_expires_at < ?`, missionID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []*Run
	for rows.Next() {
		r, err := scanRunRow(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// --- Event Logging ---

// LogEvent appends an event to the event log.
func (s *Store) LogEvent(e *Event) error {
	now := time.Now().UTC()
	e.CreatedAt = now
	res, err := s.db.Exec(`INSERT INTO events (mission_id, task_id, run_id, type, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		e.MissionID, nilStr(e.TaskID), nilStr(e.RunID), string(e.Type),
		string(e.Payload), fmtTime(now))
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	e.ID = id
	return nil
}

// RecentEvents returns the most recent N events for a mission.
func (s *Store) RecentEvents(missionID string, limit int) ([]*Event, error) {
	rows, err := s.db.Query(`SELECT id, mission_id, task_id, run_id, type, payload_json, created_at
		FROM events WHERE mission_id = ?
		ORDER BY id DESC LIMIT ?`, missionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []*Event
	for rows.Next() {
		var e Event
		var taskID, runID sql.NullString
		var createdAt string
		if err := rows.Scan(&e.ID, &e.MissionID, &taskID, &runID, &e.Type, &e.Payload, &createdAt); err != nil {
			return nil, err
		}
		e.TaskID = taskID.String
		e.RunID = runID.String
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		events = append(events, &e)
	}
	return events, rows.Err()
}

// --- Worker Cards (derived query) ---

// WorkerCards returns a snapshot of active worker runs for TUI display.
func (s *Store) WorkerCards(missionID string) ([]WorkerCard, error) {
	rows, err := s.db.Query(`SELECT r.id, r.task_id, t.title, r.status, r.worktree_path, r.started_at
		FROM runs r JOIN tasks t ON r.task_id = t.id
		WHERE r.mission_id = ? AND r.mode = 'worker' AND r.status IN ('queued','running')
		ORDER BY r.started_at ASC`, missionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cards []WorkerCard
	for rows.Next() {
		var c WorkerCard
		var startedAt sql.NullString
		if err := rows.Scan(&c.RunID, &c.TaskID, &c.TaskTitle, &c.Status, &c.WorktreePath, &startedAt); err != nil {
			return nil, err
		}
		if startedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, startedAt.String)
			c.StartedAt = &t
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// --- Helpers ---

func fmtTime(t time.Time) string {
	return t.Format(time.RFC3339Nano)
}

func fmtTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := fmtTime(*t)
	return &s
}

func nilStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func parseTimePtr(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	t, _ := time.Parse(time.RFC3339Nano, s.String)
	return &t
}

// scanMission scans a mission row from a single-row query.
func scanMission(row *sql.Row) (*Mission, error) {
	var m Mission
	var status, policyJSON, budgetJSON, criteriaJSON string
	var createdAt, updatedAt string
	var startedAt, endedAt, lastReplanAt sql.NullString
	err := row.Scan(&m.ID, &m.Title, &m.Goal, &m.RepoRoot, &m.BaseCommit, &m.BaseBranch,
		&status, &policyJSON, &budgetJSON, &criteriaJSON, &m.IntegrationRef,
		&createdAt, &updatedAt, &startedAt, &endedAt, &lastReplanAt)
	if err != nil {
		return nil, err
	}
	m.Status = MissionStatus(status)
	json.Unmarshal([]byte(policyJSON), &m.Policy)
	json.Unmarshal([]byte(budgetJSON), &m.Budget)
	json.Unmarshal([]byte(criteriaJSON), &m.SuccessCriteria)
	m.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	m.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	m.StartedAt = parseTimePtr(startedAt)
	m.EndedAt = parseTimePtr(endedAt)
	m.LastReplanAt = parseTimePtr(lastReplanAt)
	return &m, nil
}

// scannerRow is the interface common to *sql.Row and *sql.Rows.
type scannerRow interface {
	Scan(dest ...any) error
}

func scanMissionRow(row scannerRow) (*Mission, error) {
	var m Mission
	var status, policyJSON, budgetJSON, criteriaJSON string
	var createdAt, updatedAt string
	var startedAt, endedAt, lastReplanAt sql.NullString
	err := row.Scan(&m.ID, &m.Title, &m.Goal, &m.RepoRoot, &m.BaseCommit, &m.BaseBranch,
		&status, &policyJSON, &budgetJSON, &criteriaJSON, &m.IntegrationRef,
		&createdAt, &updatedAt, &startedAt, &endedAt, &lastReplanAt)
	if err != nil {
		return nil, err
	}
	m.Status = MissionStatus(status)
	json.Unmarshal([]byte(policyJSON), &m.Policy)
	json.Unmarshal([]byte(budgetJSON), &m.Budget)
	json.Unmarshal([]byte(criteriaJSON), &m.SuccessCriteria)
	m.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	m.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	m.StartedAt = parseTimePtr(startedAt)
	m.EndedAt = parseTimePtr(endedAt)
	m.LastReplanAt = parseTimePtr(lastReplanAt)
	return &m, nil
}

func scanTask(row *sql.Row) (*Task, error) {
	var t Task
	var kind, status, riskLevel string
	var scopeJSON, acJSON, rrJSON string
	var createdAt, updatedAt string
	var blockingReason sql.NullString
	err := row.Scan(&t.ID, &t.MissionID, &t.Title, &kind, &t.Objective, &status,
		&t.Priority, &scopeJSON, &acJSON, &rrJSON,
		&t.EstimatedEffort, &riskLevel, &t.AttemptCount, &blockingReason,
		&createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	t.Kind = TaskKind(kind)
	t.Status = TaskStatus(status)
	t.RiskLevel = RiskLevel(riskLevel)
	json.Unmarshal([]byte(scopeJSON), &t.Scope)
	json.Unmarshal([]byte(acJSON), &t.AcceptanceCriteria)
	json.Unmarshal([]byte(rrJSON), &t.ReviewRequirements)
	if blockingReason.Valid {
		t.BlockingReason = blockingReason.String
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &t, nil
}

func scanTaskRow(row scannerRow) (*Task, error) {
	var t Task
	var kind, status, riskLevel string
	var scopeJSON, acJSON, rrJSON string
	var createdAt, updatedAt string
	var blockingReason sql.NullString
	err := row.Scan(&t.ID, &t.MissionID, &t.Title, &kind, &t.Objective, &status,
		&t.Priority, &scopeJSON, &acJSON, &rrJSON,
		&t.EstimatedEffort, &riskLevel, &t.AttemptCount, &blockingReason,
		&createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	t.Kind = TaskKind(kind)
	t.Status = TaskStatus(status)
	t.RiskLevel = RiskLevel(riskLevel)
	json.Unmarshal([]byte(scopeJSON), &t.Scope)
	json.Unmarshal([]byte(acJSON), &t.AcceptanceCriteria)
	json.Unmarshal([]byte(rrJSON), &t.ReviewRequirements)
	if blockingReason.Valid {
		t.BlockingReason = blockingReason.String
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &t, nil
}

func scanRunRow(row scannerRow) (*Run, error) {
	var r Run
	var mode, status string
	var leaseOwner, leaseExpires, heartbeat, startedAt, endedAt, summary, errorText sql.NullString
	err := row.Scan(&r.ID, &r.MissionID, &r.TaskID, &mode, &status,
		&leaseOwner, &leaseExpires, &heartbeat, &r.WorktreePath,
		&startedAt, &endedAt, &summary, &errorText)
	if err != nil {
		return nil, err
	}
	r.Mode = RunMode(mode)
	r.Status = RunStatus(status)
	r.LeaseOwner = leaseOwner.String
	r.LeaseExpiresAt = parseTimePtr(leaseExpires)
	r.HeartbeatAt = parseTimePtr(heartbeat)
	r.StartedAt = parseTimePtr(startedAt)
	r.EndedAt = parseTimePtr(endedAt)
	r.Summary = summary.String
	r.ErrorText = errorText.String
	return &r, nil
}
