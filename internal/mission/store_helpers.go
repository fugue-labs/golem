package mission

import (
	"database/sql"
	"encoding/json"
	"time"
)

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
