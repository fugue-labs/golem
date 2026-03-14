package mission

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
	var policyStr, budgetStr, criteriaStr, planStateStr, metadataStr sql.NullString
	var createdStr, updatedStr sql.NullString
	var startedStr, endedStr, replanStr sql.NullString

	err := row.Scan(&m.ID, &m.Title, &m.Goal, &m.RepoRoot, &m.BaseCommit, &m.BaseBranch,
		&m.Status, &policyStr, &budgetStr, &criteriaStr, &m.IntegrationRef,
		&planStateStr, &metadataStr,
		&createdStr, &updatedStr, &startedStr, &endedStr, &replanStr)
	if err != nil {
		return nil, err
	}

	m.Policy, err = parseRawJSON("missions.policy_json", policyStr)
	if err != nil {
		return nil, err
	}
	if err := unmarshalJSONInto("missions.budget_json", nullStringValue(budgetStr), &m.Budget); err != nil {
		return nil, err
	}
	if err := unmarshalJSONInto("missions.success_criteria_json", nullStringValue(criteriaStr), &m.SuccessCriteria); err != nil {
		return nil, err
	}
	m.PlanStateJSON, err = parseRawJSON("missions.plan_state_json", planStateStr)
	if err != nil {
		return nil, err
	}
	m.MetadataJSON, err = parseRawJSON("missions.metadata_json", metadataStr)
	if err != nil {
		return nil, err
	}
	m.CreatedAt, err = parseTime("missions.created_at", createdStr)
	if err != nil {
		return nil, err
	}
	m.UpdatedAt, err = parseTime("missions.updated_at", updatedStr)
	if err != nil {
		return nil, err
	}
	m.StartedAt, err = parseOptionalTime("missions.started_at", startedStr)
	if err != nil {
		return nil, err
	}
	m.EndedAt, err = parseOptionalTime("missions.ended_at", endedStr)
	if err != nil {
		return nil, err
	}
	m.LastReplanAt, err = parseOptionalTime("missions.last_replan_at", replanStr)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func scanMissionRows(rows *sql.Rows) (*Mission, error) {
	return scanMission(rows)
}

func scanTask(row scanner) (*Task, error) {
	var t Task
	var scopeStr, criteriaStr, reviewStr, outcomeStr, metadataStr sql.NullString
	var createdStr, updatedStr sql.NullString

	err := row.Scan(&t.ID, &t.MissionID, &t.Title, &t.Kind, &t.Objective,
		&t.Status, &t.Priority, &scopeStr, &criteriaStr, &reviewStr,
		&t.EstimatedEffort, &t.RiskLevel, &t.AttemptCount, &t.BlockingReason,
		&outcomeStr, &metadataStr,
		&createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}

	if err := unmarshalJSONInto("tasks.scope_json", nullStringValue(scopeStr), &t.Scope); err != nil {
		return nil, err
	}
	if err := unmarshalJSONInto("tasks.acceptance_criteria_json", nullStringValue(criteriaStr), &t.AcceptanceCriteria); err != nil {
		return nil, err
	}
	t.ReviewRequirements, err = parseRawJSON("tasks.review_requirements_json", reviewStr)
	if err != nil {
		return nil, err
	}
	if err := unmarshalJSONInto("tasks.outcome_json", nullStringValue(outcomeStr), &t.Outcome); err != nil {
		return nil, err
	}
	t.MetadataJSON, err = parseRawJSON("tasks.metadata_json", metadataStr)
	if err != nil {
		return nil, err
	}
	t.CreatedAt, err = parseTime("tasks.created_at", createdStr)
	if err != nil {
		return nil, err
	}
	t.UpdatedAt, err = parseTime("tasks.updated_at", updatedStr)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func scanTaskRows(rows *sql.Rows) (*Task, error) {
	return scanTask(rows)
}

func scanRun(row scanner) (*Run, error) {
	var r Run
	var outcomeStr, leaseJSONStr, commandJSONStr, controlJSONStr, verificationJSONStr, metadataStr sql.NullString
	var leaseStr, heartbeatStr, startedStr, endedStr sql.NullString

	err := row.Scan(&r.ID, &r.MissionID, &r.TaskID, &r.ParentRunID, &r.Mode, &r.Status, &r.LeaseOwner,
		&leaseStr, &heartbeatStr, &r.WorktreePath, &r.BaseRef, &startedStr, &endedStr,
		&r.Summary, &r.ErrorText, &outcomeStr, &leaseJSONStr, &commandJSONStr, &controlJSONStr,
		&verificationJSONStr, &metadataStr)
	if err != nil {
		return nil, err
	}

	r.LeaseExpires, err = parseOptionalTime("runs.lease_expires_at", leaseStr)
	if err != nil {
		return nil, err
	}
	r.HeartbeatAt, err = parseOptionalTime("runs.heartbeat_at", heartbeatStr)
	if err != nil {
		return nil, err
	}
	r.StartedAt, err = parseOptionalTime("runs.started_at", startedStr)
	if err != nil {
		return nil, err
	}
	r.EndedAt, err = parseOptionalTime("runs.ended_at", endedStr)
	if err != nil {
		return nil, err
	}
	if err := unmarshalJSONInto("runs.outcome_json", nullStringValue(outcomeStr), &r.Outcome); err != nil {
		return nil, err
	}
	r.LeaseJSON, err = parseRawJSON("runs.lease_json", leaseJSONStr)
	if err != nil {
		return nil, err
	}
	r.CommandJSON, err = parseRawJSON("runs.command_json", commandJSONStr)
	if err != nil {
		return nil, err
	}
	r.ControlJSON, err = parseRawJSON("runs.control_json", controlJSONStr)
	if err != nil {
		return nil, err
	}
	r.VerificationJSON, err = parseRawJSON("runs.verification_json", verificationJSONStr)
	if err != nil {
		return nil, err
	}
	r.MetadataJSON, err = parseRawJSON("runs.metadata_json", metadataStr)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func scanRunRows(rows *sql.Rows) (*Run, error) {
	return scanRun(rows)
}

func scanArtifact(row scanner) (*Artifact, error) {
	var a Artifact
	var contentStr, metadataStr, createdStr sql.NullString
	if err := row.Scan(&a.ID, &a.MissionID, &a.TaskID, &a.RunID, &a.Type, &a.Role,
		&a.RelativePath, &a.SHA256, &a.MediaType, &a.SizeBytes, &contentStr, &metadataStr, &createdStr); err != nil {
		return nil, err
	}
	var err error
	a.ContentJSON, err = parseRawJSON("artifacts.content_json", contentStr)
	if err != nil {
		return nil, err
	}
	a.MetadataJSON, err = parseRawJSON("artifacts.metadata_json", metadataStr)
	if err != nil {
		return nil, err
	}
	a.CreatedAt, err = parseTime("artifacts.created_at", createdStr)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func scanArtifactRows(rows *sql.Rows) (*Artifact, error) {
	return scanArtifact(rows)
}

func scanApproval(row scanner) (*Approval, error) {
	var a Approval
	var reqStr, respStr, metadataStr, createdStr sql.NullString
	var resolvedStr sql.NullString
	if err := row.Scan(&a.ID, &a.MissionID, &a.TaskID, &a.RunID, &a.Kind, &a.Status,
		&a.Approver, &a.Reason, &reqStr, &respStr, &metadataStr, &createdStr, &resolvedStr); err != nil {
		return nil, err
	}
	var err error
	a.RequestJSON, err = parseRawJSON("approvals.request_json", reqStr)
	if err != nil {
		return nil, err
	}
	a.ResponseJSON, err = parseRawJSON("approvals.response_json", respStr)
	if err != nil {
		return nil, err
	}
	a.MetadataJSON, err = parseRawJSON("approvals.metadata_json", metadataStr)
	if err != nil {
		return nil, err
	}
	a.CreatedAt, err = parseTime("approvals.created_at", createdStr)
	if err != nil {
		return nil, err
	}
	a.ResolvedAt, err = parseOptionalTime("approvals.resolved_at", resolvedStr)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func scanApprovalRows(rows *sql.Rows) (*Approval, error) {
	return scanApproval(rows)
}

func scanEvent(row scanner) (*Event, error) {
	var e Event
	var payloadStr, metadataStr, createdStr sql.NullString
	if err := row.Scan(&e.ID, &e.MissionID, &e.TaskID, &e.RunID, &e.Type, &e.SchemaVersion,
		&e.CorrelationID, &e.CausationID, &payloadStr, &metadataStr, &createdStr); err != nil {
		return nil, err
	}
	if e.SchemaVersion == 0 {
		e.SchemaVersion = defaultEventSchemaVersion
	}
	var err error
	e.PayloadJSON, err = parseRawJSON("events.payload_json", payloadStr)
	if err != nil {
		return nil, err
	}
	e.MetadataJSON, err = parseRawJSON("events.metadata_json", metadataStr)
	if err != nil {
		return nil, err
	}
	e.CreatedAt, err = parseTime("events.created_at", createdStr)
	if err != nil {
		return nil, err
	}
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

func parseTime(field string, s sql.NullString) (time.Time, error) {
	if !s.Valid || strings.TrimSpace(s.String) == "" {
		return time.Time{}, fmt.Errorf("missing %s", field)
	}
	t, err := time.Parse(timeFmt, s.String)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %s: %w", field, err)
	}
	return t, nil
}

func parseOptionalTime(field string, s sql.NullString) (*time.Time, error) {
	if !s.Valid || strings.TrimSpace(s.String) == "" {
		return nil, nil
	}
	t, err := time.Parse(timeFmt, s.String)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", field, err)
	}
	return &t, nil
}

func nullStringValue(s sql.NullString) string {
	if !s.Valid {
		return ""
	}
	return s.String
}

func parseRawJSON(field string, data sql.NullString) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(nullStringValue(data))
	if trimmed == "" {
		return nil, nil
	}
	if !json.Valid([]byte(trimmed)) {
		return nil, fmt.Errorf("decode %s: invalid JSON", field)
	}
	return cloneRawMessage(json.RawMessage(trimmed)), nil
}

func marshalOrDefault(raw json.RawMessage, def string) (string, error) {
	if len(raw) == 0 {
		return def, nil
	}
	if !json.Valid(raw) {
		return "", fmt.Errorf("encode raw JSON: invalid JSON")
	}
	return string(raw), nil
}

func marshalJSONOrDefault(v any, def string) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if len(data) == 0 || string(data) == "null" {
		return def, nil
	}
	return string(data), nil
}

func unmarshalJSONInto(field, data string, dst any) error {
	if strings.TrimSpace(data) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(data), dst); err != nil {
		return fmt.Errorf("decode %s: %w", field, err)
	}
	return nil
}
