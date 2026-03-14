package mission

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
)

func init() {
	// Silence the mysql driver's internal logger to prevent raw error text
	// (e.g., packet read timeouts) from writing to stderr and corrupting
	// the TUI. Query-level errors are still returned via error values.
	_ = mysql.SetLogger(log.New(io.Discard, "", 0))
}

const (
	// DefaultDoltHost is the default Dolt SQL server address.
	DefaultDoltHost = "127.0.0.1:3307"
	// DefaultDoltDB is the default database name for mission state.
	DefaultDoltDB = "golem_missions"
)

// DefaultDSN returns the default MySQL DSN for the mission Dolt store.
func DefaultDSN() string {
	return "root@tcp(" + DefaultDoltHost + ")/" + DefaultDoltDB + "?timeout=5s&readTimeout=10s&writeTimeout=10s"
}

// Environment variable names for DSN configuration.
const (
	EnvDoltDSN  = "GOLEM_DOLT_DSN"
	EnvDoltHost = "GOLEM_DOLT_HOST"
	EnvDoltDB   = "GOLEM_DOLT_DB"
)

// ResolveDSN returns the Dolt DSN, checking environment overrides first.
// Precedence: GOLEM_DOLT_DSN > GOLEM_DOLT_HOST/GOLEM_DOLT_DB > defaults.
func ResolveDSN() string {
	if dsn := strings.TrimSpace(os.Getenv(EnvDoltDSN)); dsn != "" {
		return dsn
	}
	host := strings.TrimSpace(os.Getenv(EnvDoltHost))
	if host == "" {
		host = DefaultDoltHost
	}
	db := strings.TrimSpace(os.Getenv(EnvDoltDB))
	if db == "" {
		db = DefaultDoltDB
	}
	return "root@tcp(" + host + ")/" + db + "?timeout=5s&readTimeout=10s&writeTimeout=10s"
}

// DoltStore implements Store backed by a Dolt database (MySQL-protocol).
type DoltStore struct {
	db *sql.DB
	mu sync.RWMutex
}

// OpenDoltStore connects to a Dolt SQL server and initializes the mission schema.
// The dsn should be a MySQL-format DSN, e.g. "root@tcp(127.0.0.1:3307)/golem_missions".
// The target database is created automatically if it does not exist.
// If the database was previously dropped (e.g. by gt dolt cleanup treating it
// as an orphan), it is recovered via DOLT_UNDROP to preserve mission history.
func OpenDoltStore(dsn string) (*DoltStore, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	dbName := cfg.DBName
	if dbName == "" {
		return nil, fmt.Errorf("DSN must include a database name")
	}

	// Connect without database to ensure it exists.
	cfg.DBName = ""
	rootDB, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open root connection: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	safeName := sanitizeDBName(dbName)

	// Try to recover the database if it was previously dropped.
	// The gt dolt cleanup command drops databases not referenced in rig
	// metadata.json, which includes golem_missions. DOLT_UNDROP restores
	// the database with all its data from .dolt_dropped_databases/.
	if !databaseExists(ctx, rootDB, safeName) {
		// Ignore errors: DOLT_UNDROP may not be available or the database
		// may not be in the dropped list. CREATE DATABASE below handles both.
		rootDB.ExecContext(ctx, "CALL DOLT_UNDROP(?)", safeName) //nolint:errcheck
	}

	if _, err := rootDB.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS `"+safeName+"`"); err != nil {
		rootDB.Close()
		return nil, fmt.Errorf("create database %s: %w", dbName, err)
	}
	rootDB.Close()

	// Connect to the target database.
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open dolt: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping dolt: %w", err)
	}

	s := &DoltStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// sanitizeDBName removes backticks from database names to prevent injection.
func sanitizeDBName(name string) string {
	return strings.ReplaceAll(name, "`", "")
}

// databaseExists checks whether a database with the given name exists on the server.
func databaseExists(ctx context.Context, db *sql.DB, name string) bool {
	var count int
	err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?", name,
	).Scan(&count)
	return err == nil && count > 0
}

func (s *DoltStore) migrate() error {
	// Execute each statement separately — MySQL doesn't support multi-statement
	// DDL in a single Exec by default.
	for _, stmt := range doltSchemaStatements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec schema statement: %w\nSQL: %s", err, stmt)
		}
	}
	return nil
}

var doltSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS missions (
		id              VARCHAR(255) PRIMARY KEY,
		title           TEXT NOT NULL,
		goal            TEXT NOT NULL,
		repo_root       VARCHAR(512) NOT NULL DEFAULT '',
		base_commit     VARCHAR(255) NOT NULL DEFAULT '',
		base_branch     VARCHAR(255) NOT NULL DEFAULT '',
		status          VARCHAR(64) NOT NULL DEFAULT 'draft',
		policy_json     TEXT NOT NULL DEFAULT '{}',
		budget_json     TEXT NOT NULL DEFAULT '{}',
		success_criteria_json TEXT NOT NULL DEFAULT '[]',
		integration_ref VARCHAR(255) NOT NULL DEFAULT '',
		created_at      TEXT NOT NULL,
		updated_at      TEXT NOT NULL,
		started_at      TEXT,
		ended_at        TEXT,
		last_replan_at  TEXT
	)`,

	`CREATE TABLE IF NOT EXISTS tasks (
		id                      VARCHAR(255) PRIMARY KEY,
		mission_id              VARCHAR(255) NOT NULL,
		title                   TEXT NOT NULL,
		kind                    VARCHAR(64) NOT NULL DEFAULT 'code',
		objective               TEXT NOT NULL,
		status                  VARCHAR(64) NOT NULL DEFAULT 'pending',
		priority                INT NOT NULL DEFAULT 0,
		scope_json              TEXT NOT NULL DEFAULT '{}',
		acceptance_criteria_json TEXT NOT NULL DEFAULT '[]',
		review_requirements_json TEXT NOT NULL DEFAULT '{}',
		estimated_effort        VARCHAR(64) NOT NULL DEFAULT '',
		risk_level              VARCHAR(32) NOT NULL DEFAULT 'low',
		attempt_count           INT NOT NULL DEFAULT 0,
		blocking_reason         TEXT NOT NULL,
		created_at              TEXT NOT NULL,
		updated_at              TEXT NOT NULL,
		INDEX idx_tasks_mission (mission_id),
		INDEX idx_tasks_status (mission_id, status)
	)`,

	`CREATE TABLE IF NOT EXISTS task_dependencies (
		task_id       VARCHAR(255) NOT NULL,
		depends_on_id VARCHAR(255) NOT NULL,
		PRIMARY KEY (task_id, depends_on_id)
	)`,

	`CREATE TABLE IF NOT EXISTS runs (
		id               VARCHAR(255) PRIMARY KEY,
		mission_id       VARCHAR(255) NOT NULL,
		task_id          VARCHAR(255) NOT NULL DEFAULT '',
		mode             VARCHAR(64) NOT NULL,
		status           VARCHAR(64) NOT NULL DEFAULT 'queued',
		lease_owner      VARCHAR(255) NOT NULL DEFAULT '',
		lease_expires_at TEXT,
		heartbeat_at     TEXT,
		worktree_path    VARCHAR(512) NOT NULL DEFAULT '',
		started_at       TEXT,
		ended_at         TEXT,
		summary          TEXT NOT NULL,
		error_text       TEXT NOT NULL,
		INDEX idx_runs_mission (mission_id),
		INDEX idx_runs_task (task_id)
	)`,

	`CREATE TABLE IF NOT EXISTS artifacts (
		id            VARCHAR(255) PRIMARY KEY,
		mission_id    VARCHAR(255) NOT NULL,
		task_id       VARCHAR(255) NOT NULL DEFAULT '',
		run_id        VARCHAR(255) NOT NULL DEFAULT '',
		type          VARCHAR(64) NOT NULL,
		relative_path TEXT NOT NULL,
		sha256        VARCHAR(64) NOT NULL DEFAULT '',
		created_at    TEXT NOT NULL,
		INDEX idx_artifacts_mission (mission_id)
	)`,

	`CREATE TABLE IF NOT EXISTS approvals (
		id            VARCHAR(255) PRIMARY KEY,
		mission_id    VARCHAR(255) NOT NULL,
		task_id       VARCHAR(255) NOT NULL DEFAULT '',
		run_id        VARCHAR(255) NOT NULL DEFAULT '',
		kind          VARCHAR(64) NOT NULL,
		status        VARCHAR(64) NOT NULL DEFAULT 'pending',
		request_json  TEXT NOT NULL DEFAULT '{}',
		response_json TEXT NOT NULL DEFAULT '{}',
		created_at    TEXT NOT NULL,
		resolved_at   TEXT,
		INDEX idx_approvals_mission (mission_id)
	)`,

	`CREATE TABLE IF NOT EXISTS events (
		id           BIGINT AUTO_INCREMENT PRIMARY KEY,
		mission_id   VARCHAR(255) NOT NULL,
		task_id      VARCHAR(255) NOT NULL DEFAULT '',
		run_id       VARCHAR(255) NOT NULL DEFAULT '',
		type         VARCHAR(128) NOT NULL,
		payload_json TEXT NOT NULL DEFAULT '{}',
		created_at   TEXT NOT NULL,
		INDEX idx_events_mission (mission_id)
	)`,
}

// Close closes the database connection.
func (s *DoltStore) Close() error {
	return s.db.Close()
}

// --- Mission operations ---

func (s *DoltStore) CreateMission(ctx context.Context, m *Mission) error {
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
		formatNullTime(m.StartedAt), formatNullTime(m.EndedAt), formatNullTime(m.LastReplanAt),
	)
	return err
}

func (s *DoltStore) GetMission(ctx context.Context, id string) (*Mission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRowContext(ctx,
		`SELECT id, title, goal, repo_root, base_commit, base_branch, status,
			policy_json, budget_json, success_criteria_json, integration_ref,
			created_at, updated_at, started_at, ended_at, last_replan_at
		FROM missions WHERE id = ?`, id)

	return scanMission(row)
}

func (s *DoltStore) UpdateMission(ctx context.Context, m *Mission) error {
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
		formatTime(m.UpdatedAt), formatNullTime(m.StartedAt), formatNullTime(m.EndedAt),
		formatNullTime(m.LastReplanAt),
		m.ID,
	)
	return err
}

func (s *DoltStore) ListMissions(ctx context.Context) ([]*Mission, error) {
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

func (s *DoltStore) CreateTask(ctx context.Context, t *Task) error {
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

func (s *DoltStore) GetTask(ctx context.Context, id string) (*Task, error) {
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

func (s *DoltStore) UpdateTask(ctx context.Context, t *Task) error {
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

func (s *DoltStore) ListTasks(ctx context.Context, missionID string) ([]*Task, error) {
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

func (s *DoltStore) AddDependency(ctx context.Context, dep TaskDependency) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`INSERT IGNORE INTO task_dependencies (task_id, depends_on_id) VALUES (?, ?)`,
		dep.TaskID, dep.DependsOnID)
	return err
}

func (s *DoltStore) ListDependencies(ctx context.Context, missionID string) ([]TaskDependency, error) {
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

func (s *DoltStore) CreateRun(ctx context.Context, r *Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO runs (id, mission_id, task_id, mode, status, lease_owner,
			lease_expires_at, heartbeat_at, worktree_path, started_at, ended_at,
			summary, error_text)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.MissionID, r.TaskID, string(r.Mode), string(r.Status), r.LeaseOwner,
		formatNullTime(r.LeaseExpires), formatNullTime(r.HeartbeatAt), r.WorktreePath,
		formatNullTime(r.StartedAt), formatNullTime(r.EndedAt), r.Summary, r.ErrorText,
	)
	return err
}

func (s *DoltStore) CreateRunExclusive(ctx context.Context, r *Run) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO runs (id, mission_id, task_id, mode, status, lease_owner,
			lease_expires_at, heartbeat_at, worktree_path, started_at, ended_at,
			summary, error_text)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		FROM dual
		WHERE NOT EXISTS (
			SELECT 1 FROM runs WHERE task_id = ? AND mode = ? AND status = ?
		)`,
		r.ID, r.MissionID, r.TaskID, string(r.Mode), string(r.Status), r.LeaseOwner,
		formatNullTime(r.LeaseExpires), formatNullTime(r.HeartbeatAt), r.WorktreePath,
		formatNullTime(r.StartedAt), formatNullTime(r.EndedAt), r.Summary, r.ErrorText,
		r.TaskID, string(r.Mode), string(RunRunning),
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *DoltStore) GetRun(ctx context.Context, id string) (*Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRowContext(ctx,
		`SELECT id, mission_id, task_id, mode, status, lease_owner,
			lease_expires_at, heartbeat_at, worktree_path, started_at, ended_at,
			summary, error_text
		FROM runs WHERE id = ?`, id)

	return scanRun(row)
}

func (s *DoltStore) UpdateRun(ctx context.Context, r *Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`UPDATE runs SET task_id=?, mode=?, status=?, lease_owner=?,
			lease_expires_at=?, heartbeat_at=?, worktree_path=?,
			started_at=?, ended_at=?, summary=?, error_text=?
		WHERE id=?`,
		r.TaskID, string(r.Mode), string(r.Status), r.LeaseOwner,
		formatNullTime(r.LeaseExpires), formatNullTime(r.HeartbeatAt), r.WorktreePath,
		formatNullTime(r.StartedAt), formatNullTime(r.EndedAt), r.Summary, r.ErrorText,
		r.ID,
	)
	return err
}

func (s *DoltStore) ListRuns(ctx context.Context, missionID string) ([]*Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, mission_id, task_id, mode, status, lease_owner,
			lease_expires_at, heartbeat_at, worktree_path, started_at, ended_at,
			summary, error_text
		FROM runs WHERE mission_id = ? ORDER BY id ASC`, missionID)
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

func (s *DoltStore) AppendEvent(ctx context.Context, e *Event) error {
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

func (s *DoltStore) ListEvents(ctx context.Context, missionID string, limit int) ([]*Event, error) {
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

func (s *DoltStore) CreateArtifact(ctx context.Context, a *Artifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO artifacts (id, mission_id, task_id, run_id, type, relative_path, sha256, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.MissionID, a.TaskID, a.RunID, a.Type, a.RelativePath, a.SHA256, formatTime(a.CreatedAt),
	)
	return err
}

func (s *DoltStore) ListArtifacts(ctx context.Context, missionID string) ([]*Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, mission_id, task_id, run_id, type, relative_path, sha256, created_at
		FROM artifacts WHERE mission_id = ? ORDER BY id ASC`, missionID)
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

func (s *DoltStore) CreateApproval(ctx context.Context, a *Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	reqJSON := marshalOrDefault(a.RequestJSON, "{}")
	respJSON := marshalOrDefault(a.ResponseJSON, "{}")

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO approvals (id, mission_id, task_id, run_id, kind, status,
			request_json, response_json, created_at, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.MissionID, a.TaskID, a.RunID, a.Kind, string(a.Status),
		string(reqJSON), string(respJSON), formatTime(a.CreatedAt), formatNullTime(a.ResolvedAt),
	)
	return err
}

func (s *DoltStore) GetApproval(ctx context.Context, id string) (*Approval, error) {
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

func (s *DoltStore) UpdateApproval(ctx context.Context, a *Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	respJSON := marshalOrDefault(a.ResponseJSON, "{}")

	_, err := s.db.ExecContext(ctx,
		`UPDATE approvals SET status=?, response_json=?, resolved_at=? WHERE id=?`,
		string(a.Status), string(respJSON), formatNullTime(a.ResolvedAt), a.ID)
	return err
}

func (s *DoltStore) ListApprovals(ctx context.Context, missionID string) ([]*Approval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, mission_id, task_id, run_id, kind, status, request_json, response_json, created_at, resolved_at
		FROM approvals WHERE mission_id = ? ORDER BY id ASC`, missionID)
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

func (s *DoltStore) GetMissionSummary(ctx context.Context, missionID string) (*MissionSummary, error) {
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
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM runs WHERE mission_id = ? AND status IN ('queued', 'running')`,
		missionID).Scan(&activeRuns); err != nil {
		return nil, fmt.Errorf("count active runs: %w", err)
	}

	var pendingApprovals int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM approvals WHERE mission_id = ? AND status = 'pending'`,
		missionID).Scan(&pendingApprovals); err != nil {
		return nil, fmt.Errorf("count pending approvals: %w", err)
	}

	return &MissionSummary{
		Mission:          m,
		TaskCounts:       counts,
		ActiveRuns:       activeRuns,
		PendingApprovals: pendingApprovals,
	}, nil
}

func (s *DoltStore) GetReadyTasks(ctx context.Context, missionID string) ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

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

func (s *DoltStore) GetTasksByStatus(ctx context.Context, missionID string, status TaskStatus) ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, mission_id, title, kind, objective, status, priority,
			scope_json, acceptance_criteria_json, review_requirements_json,
			estimated_effort, risk_level, attempt_count, blocking_reason,
			created_at, updated_at
		FROM tasks WHERE mission_id = ? AND status = ?
		ORDER BY priority DESC, created_at ASC`, missionID, string(status))
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

func (s *DoltStore) GetRunsForTask(ctx context.Context, taskID string) ([]*Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, mission_id, task_id, mode, status, lease_owner,
			lease_expires_at, heartbeat_at, worktree_path, started_at, ended_at,
			summary, error_text
		FROM runs WHERE task_id = ? ORDER BY id ASC`, taskID)
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

// --- Dolt-specific operations ---

// DoltCommit creates a Dolt commit on the current branch with the given message.
func (s *DoltStore) DoltCommit(ctx context.Context, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `CALL DOLT_COMMIT('-Am', ?)`, message)
	return err
}

// DoltBranch creates a new Dolt branch from the current HEAD.
func (s *DoltStore) DoltBranch(ctx context.Context, branchName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `CALL DOLT_BRANCH(?)`, branchName)
	return err
}

// DoltCheckout switches the session to the given branch.
func (s *DoltStore) DoltCheckout(ctx context.Context, branchName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `CALL DOLT_CHECKOUT(?)`, branchName)
	return err
}

// DoltMerge merges the given branch into the current branch.
func (s *DoltStore) DoltMerge(ctx context.Context, branchName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `CALL DOLT_MERGE(?)`, branchName)
	return err
}

// --- Null-safe time formatting for MySQL ---

func formatNullTime(t *time.Time) interface{} {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.Format(timeFmt)
}
