package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/golem/internal/mission"
	"github.com/fugue-labs/golem/internal/ui/chat"
	"github.com/fugue-labs/gollem/core"
)

func TestMissionPlanCommandMarksMissionPlanning(t *testing.T) {
	m, ctrl := testMissionModel(t)
	m.cfg.WorkingDir = t.TempDir()
	ctx := context.Background()

	created, err := ctrl.CreateMission(ctx, mission.CreateMissionRequest{
		Title:    "Plan command test",
		Goal:     "Add pagination to the API endpoints and update tests",
		RepoRoot: m.cfg.WorkingDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	m.activeMissionID = created.ID

	msg, cmd := m.handleMissionCommand("/mission plan")
	if cmd == nil {
		t.Fatal("expected planning command")
	}
	if msg == nil || msg.Kind != chat.KindUser || msg.Content != "/mission plan" {
		t.Fatalf("unexpected returned message: %#v", msg)
	}
	if m.missionPlanRun == nil {
		t.Fatal("expected missionPlanRun to be set")
	}
	if m.missionPlanRun.MissionID != created.ID {
		t.Fatalf("missionPlanRun mission ID = %q, want %q", m.missionPlanRun.MissionID, created.ID)
	}

	stored, err := ctrl.GetMission(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != mission.MissionPlanning {
		t.Fatalf("mission status = %s, want %s", stored.Status, mission.MissionPlanning)
	}
	if len(m.messages) == 0 || !strings.Contains(m.messages[len(m.messages)-1].Content, "Planning mission") {
		t.Fatalf("expected planning status message, got %#v", m.messages)
	}
}

func TestCompleteMissionPlanRunAppliesPlanAndEnablesApproval(t *testing.T) {
	m, ctrl := testMissionModel(t)
	ctx := context.Background()

	created, err := ctrl.CreateMission(ctx, mission.CreateMissionRequest{
		Title: "Apply plan test",
		Goal:  "Refactor auth middleware and add tests",
	})
	if err != nil {
		t.Fatal(err)
	}

	stored, err := ctrl.GetMission(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Status = mission.MissionPlanning
	stored.UpdatedAt = time.Now().UTC()
	if err := ctrl.Store().UpdateMission(ctx, stored); err != nil {
		t.Fatal(err)
	}

	m.activeMissionID = created.ID
	m.missionPlanRun = &missionPlanRun{MissionID: created.ID, PreviousStatus: mission.MissionDraft}
	m.currentRunMessages = []*chat.Message{{Kind: chat.KindAssistant, Content: validMissionPlanJSON()}}

	m.completeMissionPlanRun(nil, nil)

	updated, err := ctrl.GetMission(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != mission.MissionAwaitingApproval {
		t.Fatalf("mission status = %s, want %s", updated.Status, mission.MissionAwaitingApproval)
	}

	tasks, err := ctrl.Store().ListTasks(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(tasks))
	}
	statusByID := map[string]mission.TaskStatus{}
	for _, task := range tasks {
		statusByID[task.ID] = task.Status
	}
	if statusByID["t_impl"] != mission.TaskReady {
		t.Fatalf("t_impl status = %s, want %s", statusByID["t_impl"], mission.TaskReady)
	}
	if statusByID["t_test"] != mission.TaskPending {
		t.Fatalf("t_test status = %s, want %s", statusByID["t_test"], mission.TaskPending)
	}

	deps, err := ctrl.Store().ListDependencies(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 {
		t.Fatalf("dependencies = %d, want 1", len(deps))
	}
	if got := m.messages[len(m.messages)-1].Content; !strings.Contains(got, "Mission plan applied") {
		t.Fatalf("expected success message, got %q", got)
	}

	approveMsg, _ := m.handleMissionCommand("/mission approve")
	if approveMsg == nil || approveMsg.Kind != chat.KindAssistant || !strings.Contains(approveMsg.Content, "approved and started") {
		t.Fatalf("unexpected approve message: %#v", approveMsg)
	}
	updated, err = ctrl.GetMission(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != mission.MissionRunning {
		t.Fatalf("mission status after approve = %s, want %s", updated.Status, mission.MissionRunning)
	}
}

func TestCompleteMissionPlanRunUsesFinalModelMessages(t *testing.T) {
	m, ctrl := testMissionModel(t)
	ctx := context.Background()

	created, err := ctrl.CreateMission(ctx, mission.CreateMissionRequest{
		Title: "Final model message test",
		Goal:  "Plan from final response text",
	})
	if err != nil {
		t.Fatal(err)
	}

	stored, err := ctrl.GetMission(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Status = mission.MissionPlanning
	stored.UpdatedAt = time.Now().UTC()
	if err := ctrl.Store().UpdateMission(ctx, stored); err != nil {
		t.Fatal(err)
	}

	m.activeMissionID = created.ID
	m.missionPlanRun = &missionPlanRun{MissionID: created.ID, PreviousStatus: mission.MissionDraft}
	m.currentRunMessages = []*chat.Message{{Kind: chat.KindUser, Content: "/mission plan"}}
	m.messages = []*chat.Message{{Kind: chat.KindUser, Content: "/mission plan"}}

	m.completeMissionPlanRun(nil, []core.ModelMessage{
		core.ModelResponse{Parts: []core.ModelResponsePart{core.TextPart{Content: validMissionPlanJSON()}}},
	})

	updated, err := ctrl.GetMission(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != mission.MissionAwaitingApproval {
		t.Fatalf("mission status = %s, want %s", updated.Status, mission.MissionAwaitingApproval)
	}
}

func TestCompleteMissionPlanRunRestoresDraftOnParseFailure(t *testing.T) {
	m, ctrl := testMissionModel(t)
	ctx := context.Background()

	created, err := ctrl.CreateMission(ctx, mission.CreateMissionRequest{
		Title: "Parse failure test",
		Goal:  "Implement a mission planner",
	})
	if err != nil {
		t.Fatal(err)
	}

	stored, err := ctrl.GetMission(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Status = mission.MissionPlanning
	stored.UpdatedAt = time.Now().UTC()
	if err := ctrl.Store().UpdateMission(ctx, stored); err != nil {
		t.Fatal(err)
	}

	m.missionPlanRun = &missionPlanRun{MissionID: created.ID, PreviousStatus: mission.MissionDraft}
	m.currentRunMessages = []*chat.Message{{Kind: chat.KindAssistant, Content: "this is not valid plan json"}}

	m.completeMissionPlanRun(nil, nil)

	updated, err := ctrl.GetMission(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != mission.MissionDraft {
		t.Fatalf("mission status = %s, want %s", updated.Status, mission.MissionDraft)
	}
	tasks, err := ctrl.Store().ListTasks(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("tasks = %d, want 0", len(tasks))
	}
	if got := m.messages[len(m.messages)-1].Content; !strings.Contains(got, "Failed to parse mission plan") {
		t.Fatalf("expected parse failure message, got %q", got)
	}
}

func TestCompleteMissionPlanRunRestoresDraftOnApplyFailure(t *testing.T) {
	m, ctrl := testMissionModel(t)
	ctx := context.Background()
	now := time.Now().UTC()

	created, err := ctrl.CreateMission(ctx, mission.CreateMissionRequest{
		Title: "Apply failure test",
		Goal:  "Implement mission orchestration safely",
	})
	if err != nil {
		t.Fatal(err)
	}

	stored, err := ctrl.GetMission(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Status = mission.MissionPlanning
	stored.UpdatedAt = now
	if err := ctrl.Store().UpdateMission(ctx, stored); err != nil {
		t.Fatal(err)
	}

	if err := ctrl.Store().CreateTask(ctx, &mission.Task{
		ID:        "t_impl",
		MissionID: created.ID,
		Title:     "Existing task",
		Kind:      mission.TaskKindCode,
		Objective: "Preexisting work",
		Status:    mission.TaskPending,
		Priority:  1,
		RiskLevel: mission.RiskLow,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	m.missionPlanRun = &missionPlanRun{MissionID: created.ID, PreviousStatus: mission.MissionDraft}
	m.currentRunMessages = []*chat.Message{{Kind: chat.KindAssistant, Content: validMissionPlanJSON()}}

	m.completeMissionPlanRun(nil, nil)

	updated, err := ctrl.GetMission(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != mission.MissionDraft {
		t.Fatalf("mission status = %s, want %s", updated.Status, mission.MissionDraft)
	}
	if got := m.messages[len(m.messages)-1].Content; !strings.Contains(got, "Failed to apply mission plan") {
		t.Fatalf("expected apply failure message, got %q", got)
	}
}

func validMissionPlanJSON() string {
	return "```json\n{" +
		"\"summary\":\"Plan auth refactor\"," +
		"\"success_criteria\":[\"All tests pass\"]," +
		"\"tasks\":[" +
		"{" +
		"\"id\":\"t_impl\"," +
		"\"title\":\"Implement auth changes\"," +
		"\"kind\":\"code\"," +
		"\"objective\":\"Refactor the auth middleware\"," +
		"\"priority\":1," +
		"\"scope\":{\"write_paths\":[\"internal/ui\"],\"read_paths\":[\"internal/mission\"]}," +
		"\"acceptance_criteria\":[\"Code compiles\"]," +
		"\"estimated_effort\":\"small\"," +
		"\"risk_level\":\"low\"" +
		"}," +
		"{" +
		"\"id\":\"t_test\"," +
		"\"title\":\"Add regression tests\"," +
		"\"kind\":\"test\"," +
		"\"objective\":\"Add tests for the auth flow\"," +
		"\"priority\":2," +
		"\"scope\":{\"write_paths\":[\"internal/ui\"],\"read_paths\":[\"internal/mission\"]}," +
		"\"acceptance_criteria\":[\"Tests pass\"]," +
		"\"estimated_effort\":\"small\"," +
		"\"risk_level\":\"low\"" +
		"}" +
		"]," +
		"\"dependencies\":[{" +
		"\"task_id\":\"t_test\"," +
		"\"depends_on_id\":\"t_impl\"" +
		"}]" +
	"}\n```"
}
