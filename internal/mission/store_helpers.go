package mission

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	sqlite3 "modernc.org/sqlite"
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

// --- Copy helpers ---

func cloneMission(m *Mission) *Mission {
	if m == nil {
		return nil
	}
	cp := *m
	cp.Policy = json.RawMessage(cloneBytes(m.Policy))
	cp.SuccessCriteria = cloneStrings(m.SuccessCriteria)
	cp.StartedAt = cloneTimePtr(m.StartedAt)
	cp.EndedAt = cloneTimePtr(m.EndedAt)
	cp.LastReplanAt = cloneTimePtr(m.LastReplanAt)
	return &cp
}

func cloneTask(t *Task) *Task {
	if t == nil {
		return nil
	}
	cp := *t
	cp.Scope = TaskScope{
		WritePaths: cloneStrings(t.Scope.WritePaths),
		ReadPaths:  cloneStrings(t.Scope.ReadPaths),
	}
	cp.AcceptanceCriteria = cloneStrings(t.AcceptanceCriteria)
	cp.ReviewRequirements = json.RawMessage(cloneBytes(t.ReviewRequirements))
	return &cp
}

func cloneRun(r *Run) *Run {
	if r == nil {
		return nil
	}
	cp := *r
	cp.LeaseExpires = cloneTimePtr(r.LeaseExpires)
	cp.HeartbeatAt = cloneTimePtr(r.HeartbeatAt)
	cp.StartedAt = cloneTimePtr(r.StartedAt)
	cp.EndedAt = cloneTimePtr(r.EndedAt)
	return &cp
}

func cloneEvent(e *Event) *Event {
	if e == nil {
		return nil
	}
	cp := *e
	cp.PayloadJSON = json.RawMessage(cloneBytes(e.PayloadJSON))
	return &cp
}

func cloneArtifact(a *Artifact) *Artifact {
	if a == nil {
		return nil
	}
	cp := *a
	return &cp
}

func cloneApproval(a *Approval) *Approval {
	if a == nil {
		return nil
	}
	cp := *a
	cp.RequestJSON = json.RawMessage(cloneBytes(a.RequestJSON))
	cp.ResponseJSON = json.RawMessage(cloneBytes(a.ResponseJSON))
	cp.ResolvedAt = cloneTimePtr(a.ResolvedAt)
	return &cp
}

func cloneBytes(src []byte) []byte {
	if src == nil {
		return nil
	}
	return append([]byte(nil), src...)
}

func cloneStrings(src []string) []string {
	if src == nil {
		return nil
	}
	return append([]string(nil), src...)
}

func cloneTimePtr(src *time.Time) *time.Time {
	if src == nil {
		return nil
	}
	cp := *src
	return &cp
}

// --- Ordering helpers ---

func sortMissions(missions []*Mission) {
	sort.Slice(missions, func(i, j int) bool {
		if !missions[i].CreatedAt.Equal(missions[j].CreatedAt) {
			return missions[i].CreatedAt.After(missions[j].CreatedAt)
		}
		return missions[i].ID < missions[j].ID
	})
}

func sortTasks(tasks []*Task) {
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Priority != tasks[j].Priority {
			return tasks[i].Priority > tasks[j].Priority
		}
		if !tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
		}
		return tasks[i].ID < tasks[j].ID
	})
}

func sortDependencies(deps []TaskDependency) {
	sort.Slice(deps, func(i, j int) bool {
		if deps[i].TaskID != deps[j].TaskID {
			return deps[i].TaskID < deps[j].TaskID
		}
		return deps[i].DependsOnID < deps[j].DependsOnID
	})
}

func sortRuns(runs []*Run) {
	sort.Slice(runs, func(i, j int) bool {
		left := runSortTime(runs[i])
		right := runSortTime(runs[j])
		if cmp := compareTimePtrs(left, right); cmp != 0 {
			return cmp < 0
		}
		if runs[i].ID != runs[j].ID {
			return runs[i].ID < runs[j].ID
		}
		if runs[i].TaskID != runs[j].TaskID {
			return runs[i].TaskID < runs[j].TaskID
		}
		return runs[i].Mode < runs[j].Mode
	})
}

func sortEvents(events []*Event) {
	sort.Slice(events, func(i, j int) bool {
		if events[i].ID != events[j].ID {
			return events[i].ID < events[j].ID
		}
		if !events[i].CreatedAt.Equal(events[j].CreatedAt) {
			return events[i].CreatedAt.Before(events[j].CreatedAt)
		}
		return events[i].Type < events[j].Type
	})
}

func sortArtifacts(artifacts []*Artifact) {
	sort.Slice(artifacts, func(i, j int) bool {
		if !artifacts[i].CreatedAt.Equal(artifacts[j].CreatedAt) {
			return artifacts[i].CreatedAt.Before(artifacts[j].CreatedAt)
		}
		return artifacts[i].ID < artifacts[j].ID
	})
}

func sortApprovals(approvals []*Approval) {
	sort.Slice(approvals, func(i, j int) bool {
		if !approvals[i].CreatedAt.Equal(approvals[j].CreatedAt) {
			return approvals[i].CreatedAt.Before(approvals[j].CreatedAt)
		}
		return approvals[i].ID < approvals[j].ID
	})
}

func runSortTime(r *Run) *time.Time {
	if r == nil {
		return nil
	}
	for _, candidate := range []*time.Time{r.StartedAt, r.HeartbeatAt, r.LeaseExpires, r.EndedAt} {
		if candidate != nil && !candidate.IsZero() {
			return candidate
		}
	}
	return nil
}

func compareTimePtrs(left, right *time.Time) int {
	switch {
	case left == nil && right == nil:
		return 0
	case left == nil:
		return -1
	case right == nil:
		return 1
	case left.Before(*right):
		return -1
	case left.After(*right):
		return 1
	default:
		return 0
	}
}

// --- Error helpers ---

func alreadyExistsError(kind, id string) error {
	return fmt.Errorf("%s %s already exists", kind, id)
}

func notFoundError(kind, id string) error {
	return fmt.Errorf("%s %s not found", kind, id)
}

func normalizeCreateError(err error, kind, id string) error {
	if err == nil {
		return nil
	}
	if isSQLiteDuplicateError(err) {
		return alreadyExistsError(kind, id)
	}
	return err
}

func isSQLiteDuplicateError(err error) bool {
	var sqlErr *sqlite3.Error
	if errors.As(err, &sqlErr) {
		switch sqlErr.Code() {
		case 19, 1555, 2067:
			return true
		}
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") ||
		strings.Contains(message, "constraint failed") && strings.Contains(message, "(1555)") ||
		strings.Contains(message, "constraint failed") && strings.Contains(message, "(2067)")
}

func requireRowsAffected(result sql.Result, kind, id string) error {
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return notFoundError(kind, id)
	}
	return nil
}

func validateDependencyTargets(dep TaskDependency, missionForTask func(string) (string, error)) error {
	taskMissionID, err := missionForTask(dep.TaskID)
	if err != nil {
		return err
	}
	dependsOnMissionID, err := missionForTask(dep.DependsOnID)
	if err != nil {
		return err
	}
	if taskMissionID != dependsOnMissionID {
		return fmt.Errorf("dependency %s -> %s crosses mission boundaries", dep.TaskID, dep.DependsOnID)
	}
	return nil
}

func dependencyBelongsToMission(dep TaskDependency, missionID string, missionForTask func(string) (string, bool)) bool {
	taskMissionID, ok := missionForTask(dep.TaskID)
	if !ok || taskMissionID != missionID {
		return false
	}
	dependsOnMissionID, ok := missionForTask(dep.DependsOnID)
	return ok && dependsOnMissionID == missionID
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
