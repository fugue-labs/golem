package mission

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

const defaultEventSchemaVersion = 1

// --- Scan helpers ---

type scanner interface {
	Scan(dest ...any) error
}

func scanMission(row scanner) (*Mission, error) {
	var m Mission
	var policyStr, budgetStr, criteriaStr, planStateStr, metadataStr string
	var createdStr, updatedStr string
	var startedStr, endedStr, replanStr sql.NullString

	err := row.Scan(&m.ID, &m.Title, &m.Goal, &m.RepoRoot, &m.BaseCommit, &m.BaseBranch,
		&m.Status, &policyStr, &budgetStr, &criteriaStr, &m.IntegrationRef,
		&planStateStr, &metadataStr,
		&createdStr, &updatedStr, &startedStr, &endedStr, &replanStr)
	if err != nil {
		return nil, err
	}

	m.Policy = cloneRawMessage(json.RawMessage(policyStr))
	unmarshalJSONInto(budgetStr, &m.Budget)
	unmarshalJSONInto(criteriaStr, &m.SuccessCriteria)
	m.PlanStateJSON = cloneRawMessage(json.RawMessage(planStateStr))
	m.MetadataJSON = cloneRawMessage(json.RawMessage(metadataStr))
	m.CreatedAt = parseTime(createdStr)
	m.UpdatedAt = parseTime(updatedStr)
	m.StartedAt = parseOptionalTime(startedStr)
	m.EndedAt = parseOptionalTime(endedStr)
	m.LastReplanAt = parseOptionalTime(replanStr)
	return &m, nil
}

func scanMissionRows(rows *sql.Rows) (*Mission, error) {
	return scanMission(rows)
}

func scanTask(row scanner) (*Task, error) {
	var t Task
	var scopeStr, criteriaStr, reviewStr, outcomeStr, metadataStr string
	var createdStr, updatedStr string

	err := row.Scan(&t.ID, &t.MissionID, &t.Title, &t.Kind, &t.Objective,
		&t.Status, &t.Priority, &scopeStr, &criteriaStr, &reviewStr,
		&t.EstimatedEffort, &t.RiskLevel, &t.AttemptCount, &t.BlockingReason,
		&outcomeStr, &metadataStr,
		&createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}

	unmarshalJSONInto(scopeStr, &t.Scope)
	unmarshalJSONInto(criteriaStr, &t.AcceptanceCriteria)
	t.ReviewRequirements = cloneRawMessage(json.RawMessage(reviewStr))
	unmarshalJSONInto(outcomeStr, &t.Outcome)
	t.MetadataJSON = cloneRawMessage(json.RawMessage(metadataStr))
	t.CreatedAt = parseTime(createdStr)
	t.UpdatedAt = parseTime(updatedStr)
	return &t, nil
}

func scanTaskRows(rows *sql.Rows) (*Task, error) {
	return scanTask(rows)
}

func scanRun(row scanner) (*Run, error) {
	var r Run
	var outcomeStr, leaseJSONStr, commandJSONStr, controlJSONStr, verificationJSONStr, metadataStr string
	var leaseStr, heartbeatStr, startedStr, endedStr sql.NullString

	err := row.Scan(&r.ID, &r.MissionID, &r.TaskID, &r.ParentRunID, &r.Mode, &r.Status, &r.LeaseOwner,
		&leaseStr, &heartbeatStr, &r.WorktreePath, &r.BaseRef, &startedStr, &endedStr,
		&r.Summary, &r.ErrorText, &outcomeStr, &leaseJSONStr, &commandJSONStr, &controlJSONStr,
		&verificationJSONStr, &metadataStr)
	if err != nil {
		return nil, err
	}

	r.LeaseExpires = parseOptionalTime(leaseStr)
	r.HeartbeatAt = parseOptionalTime(heartbeatStr)
	r.StartedAt = parseOptionalTime(startedStr)
	r.EndedAt = parseOptionalTime(endedStr)
	unmarshalJSONInto(outcomeStr, &r.Outcome)
	r.LeaseJSON = cloneRawMessage(json.RawMessage(leaseJSONStr))
	r.CommandJSON = cloneRawMessage(json.RawMessage(commandJSONStr))
	r.ControlJSON = cloneRawMessage(json.RawMessage(controlJSONStr))
	r.VerificationJSON = cloneRawMessage(json.RawMessage(verificationJSONStr))
	r.MetadataJSON = cloneRawMessage(json.RawMessage(metadataStr))
	return &r, nil
}

func scanRunRows(rows *sql.Rows) (*Run, error) {
	return scanRun(rows)
}

func scanArtifact(row scanner) (*Artifact, error) {
	var a Artifact
	var contentStr, metadataStr, createdStr string
	if err := row.Scan(&a.ID, &a.MissionID, &a.TaskID, &a.RunID, &a.Type, &a.Role,
		&a.RelativePath, &a.SHA256, &a.MediaType, &a.SizeBytes, &contentStr, &metadataStr, &createdStr); err != nil {
		return nil, err
	}
	a.ContentJSON = cloneRawMessage(json.RawMessage(contentStr))
	a.MetadataJSON = cloneRawMessage(json.RawMessage(metadataStr))
	a.CreatedAt = parseTime(createdStr)
	return &a, nil
}

func scanArtifactRows(rows *sql.Rows) (*Artifact, error) {
	return scanArtifact(rows)
}

func scanApproval(row scanner) (*Approval, error) {
	var a Approval
	var reqStr, respStr, metadataStr, createdStr string
	var resolvedStr sql.NullString
	if err := row.Scan(&a.ID, &a.MissionID, &a.TaskID, &a.RunID, &a.Kind, &a.Status,
		&a.Approver, &a.Reason, &reqStr, &respStr, &metadataStr, &createdStr, &resolvedStr); err != nil {
		return nil, err
	}
	a.RequestJSON = cloneRawMessage(json.RawMessage(reqStr))
	a.ResponseJSON = cloneRawMessage(json.RawMessage(respStr))
	a.MetadataJSON = cloneRawMessage(json.RawMessage(metadataStr))
	a.CreatedAt = parseTime(createdStr)
	a.ResolvedAt = parseOptionalTime(resolvedStr)
	return &a, nil
}

func scanApprovalRows(rows *sql.Rows) (*Approval, error) {
	return scanApproval(rows)
}

func scanEvent(row scanner) (*Event, error) {
	var e Event
	var payloadStr, metadataStr, createdStr string
	if err := row.Scan(&e.ID, &e.MissionID, &e.TaskID, &e.RunID, &e.Type, &e.SchemaVersion,
		&e.CorrelationID, &e.CausationID, &payloadStr, &metadataStr, &createdStr); err != nil {
		return nil, err
	}
	if e.SchemaVersion == 0 {
		e.SchemaVersion = defaultEventSchemaVersion
	}
	e.PayloadJSON = cloneRawMessage(json.RawMessage(payloadStr))
	e.MetadataJSON = cloneRawMessage(json.RawMessage(metadataStr))
	e.CreatedAt = parseTime(createdStr)
	return &e, nil
}

func scanEventRows(rows *sql.Rows) (*Event, error) {
	return scanEvent(rows)
}

// --- Clone helpers ---

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cp := make(json.RawMessage, len(raw))
	copy(cp, raw)
	return cp
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	cp := *t
	return &cp
}

func cloneOutcome(in Outcome) Outcome {
	return Outcome{
		Status:      in.Status,
		Summary:     in.Summary,
		ErrorText:   in.ErrorText,
		ArtifactIDs: cloneStrings(in.ArtifactIDs),
		ApprovalIDs: cloneStrings(in.ApprovalIDs),
		PayloadJSON: cloneRawMessage(in.PayloadJSON),
	}
}

func cloneMission(in *Mission) *Mission {
	if in == nil {
		return nil
	}
	cp := *in
	cp.Policy = cloneRawMessage(in.Policy)
	cp.SuccessCriteria = cloneStrings(in.SuccessCriteria)
	cp.PlanStateJSON = cloneRawMessage(in.PlanStateJSON)
	cp.MetadataJSON = cloneRawMessage(in.MetadataJSON)
	cp.StartedAt = cloneTimePtr(in.StartedAt)
	cp.EndedAt = cloneTimePtr(in.EndedAt)
	cp.LastReplanAt = cloneTimePtr(in.LastReplanAt)
	return &cp
}

func cloneTask(in *Task) *Task {
	if in == nil {
		return nil
	}
	cp := *in
	cp.Scope = TaskScope{
		WritePaths: cloneStrings(in.Scope.WritePaths),
		ReadPaths:  cloneStrings(in.Scope.ReadPaths),
	}
	cp.AcceptanceCriteria = cloneStrings(in.AcceptanceCriteria)
	cp.ReviewRequirements = cloneRawMessage(in.ReviewRequirements)
	cp.Outcome = cloneOutcome(in.Outcome)
	cp.MetadataJSON = cloneRawMessage(in.MetadataJSON)
	return &cp
}

func cloneRun(in *Run) *Run {
	if in == nil {
		return nil
	}
	cp := *in
	cp.LeaseExpires = cloneTimePtr(in.LeaseExpires)
	cp.HeartbeatAt = cloneTimePtr(in.HeartbeatAt)
	cp.StartedAt = cloneTimePtr(in.StartedAt)
	cp.EndedAt = cloneTimePtr(in.EndedAt)
	cp.Outcome = cloneOutcome(in.Outcome)
	cp.LeaseJSON = cloneRawMessage(in.LeaseJSON)
	cp.CommandJSON = cloneRawMessage(in.CommandJSON)
	cp.ControlJSON = cloneRawMessage(in.ControlJSON)
	cp.VerificationJSON = cloneRawMessage(in.VerificationJSON)
	cp.MetadataJSON = cloneRawMessage(in.MetadataJSON)
	return &cp
}

func cloneArtifact(in *Artifact) *Artifact {
	if in == nil {
		return nil
	}
	cp := *in
	cp.ContentJSON = cloneRawMessage(in.ContentJSON)
	cp.MetadataJSON = cloneRawMessage(in.MetadataJSON)
	return &cp
}

func cloneApproval(in *Approval) *Approval {
	if in == nil {
		return nil
	}
	cp := *in
	cp.RequestJSON = cloneRawMessage(in.RequestJSON)
	cp.ResponseJSON = cloneRawMessage(in.ResponseJSON)
	cp.MetadataJSON = cloneRawMessage(in.MetadataJSON)
	cp.ResolvedAt = cloneTimePtr(in.ResolvedAt)
	return &cp
}

func cloneEvent(in *Event) *Event {
	if in == nil {
		return nil
	}
	cp := *in
	if cp.SchemaVersion == 0 {
		cp.SchemaVersion = defaultEventSchemaVersion
	}
	cp.PayloadJSON = cloneRawMessage(in.PayloadJSON)
	cp.MetadataJSON = cloneRawMessage(in.MetadataJSON)
	return &cp
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

func parseOptionalTime(s sql.NullString) *time.Time {
	if !s.Valid || strings.TrimSpace(s.String) == "" {
		return nil
	}
	t := parseTime(s.String)
	return &t
}

func marshalOrDefault(raw json.RawMessage, def string) string {
	if len(raw) == 0 {
		return def
	}
	return string(raw)
}

func marshalJSONOrDefault(v any, def string) string {
	data, err := json.Marshal(v)
	if err != nil || len(data) == 0 || string(data) == "null" {
		return def
	}
	return string(data)
}

func unmarshalJSONInto(data string, dst any) {
	if strings.TrimSpace(data) == "" {
		return
	}
	_ = json.Unmarshal([]byte(data), dst)
}
