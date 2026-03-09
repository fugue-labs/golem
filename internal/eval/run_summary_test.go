package eval

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/ui/chat"
	uiverification "github.com/fugue-labs/golem/internal/ui/verification"
	"github.com/fugue-labs/gollem/core"
)

func TestBuildRunSummaryCapturesToolTraceAndVerification(t *testing.T) {
	runtime := agent.RuntimeReport{Provider: "openai", Model: "gpt-5.4"}
	messages := []*chat.Message{
		{Kind: chat.KindUser, Content: "implement feature"},
		{Kind: chat.KindToolCall, CallID: "call-1", ToolName: "edit", RawArgs: `{"file_path":"main.go"}`, Status: chat.ToolSuccess, Content: "edited"},
		{Kind: chat.KindAssistant, Content: "done"},
	}
	verification := uiverification.State{Entries: []uiverification.Entry{{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"}}}

	summary := BuildRunSummary("implement feature", runtime, messages, verification, core.RunUsage{Requests: 1}, nil)
	if summary.FinalStatus != "success" {
		t.Fatalf("final status = %q", summary.FinalStatus)
	}
	if len(summary.ToolTrace) != 1 || summary.ToolTrace[0].Name != "edit" {
		t.Fatalf("tool trace = %+v", summary.ToolTrace)
	}
	if len(summary.ChangedFiles) != 1 || summary.ChangedFiles[0] != "main.go" {
		t.Fatalf("changed files = %+v", summary.ChangedFiles)
	}
	if len(summary.Verification.Commands) != 1 || summary.Verification.Commands[0] != "go test ./..." {
		t.Fatalf("verification commands = %+v", summary.Verification.Commands)
	}
	if summary.Runtime.Provider != "openai" {
		t.Fatalf("runtime provider = %q", summary.Runtime.Provider)
	}
}

func TestBuildRunSummaryMarksCanceledRuns(t *testing.T) {
	summary := BuildRunSummary("prompt", agent.RuntimeReport{}, nil, uiverification.State{}, core.RunUsage{}, context.Canceled)
	if summary.FinalStatus != "canceled" {
		t.Fatalf("final status = %q", summary.FinalStatus)
	}
}

func TestRunSummaryJSONRoundTrip(t *testing.T) {
	original := BuildRunSummary(
		"prompt",
		agent.RuntimeReport{Provider: "openai", Model: "gpt-5.4", ToolSurfaces: agent.ToolSurfaceReport{ExecuteCode: "on"}},
		[]*chat.Message{{Kind: chat.KindAssistant, Content: "answer"}},
		uiverification.State{},
		core.RunUsage{Requests: 2},
		nil,
	)

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded RunSummary
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if decoded.Runtime.ToolSurfaces.ExecuteCode != "on" {
		t.Fatalf("execute_code = %q", decoded.Runtime.ToolSurfaces.ExecuteCode)
	}
	if decoded.Usage.Requests != 2 {
		t.Fatalf("requests = %d", decoded.Usage.Requests)
	}
	if decoded.FinalStatus != "success" {
		t.Fatalf("final status = %q", decoded.FinalStatus)
	}
}
