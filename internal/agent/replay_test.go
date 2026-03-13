package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewTrace(t *testing.T) {
	tr := NewTrace("claude-sonnet-4", "anthropic", "/tmp/test")
	if tr.Version != traceVersion {
		t.Errorf("expected version %d, got %d", traceVersion, tr.Version)
	}
	if tr.Model != "claude-sonnet-4" {
		t.Errorf("expected model claude-sonnet-4, got %s", tr.Model)
	}
	if tr.Provider != "anthropic" {
		t.Errorf("expected provider anthropic, got %s", tr.Provider)
	}
	if len(tr.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(tr.Events))
	}
}

func TestTraceRecord(t *testing.T) {
	tr := NewTrace("test-model", "test-provider", "/tmp/test")

	tr.Record(EventUserInput, UserInputData{Text: "hello"})
	tr.Record(EventTextDelta, TextDeltaData{Text: "world"})
	tr.Record(EventToolCall, ToolCallData{
		CallID:  "call_1",
		Name:    "bash",
		Args:    `{"command":"ls"}`,
		RawArgs: `{"command":"ls"}`,
	})
	tr.Record(EventToolResult, ToolResultData{
		CallID: "call_1",
		Name:   "bash",
		Result: "file.txt",
	})
	tr.Record(EventAgentDone, AgentDoneData{
		InputTokens:  100,
		OutputTokens: 50,
		ToolCalls:    1,
	})

	if len(tr.Events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(tr.Events))
	}

	// Verify event kinds in order.
	kinds := []ReplayEventKind{EventUserInput, EventTextDelta, EventToolCall, EventToolResult, EventAgentDone}
	for i, expected := range kinds {
		if tr.Events[i].Kind != expected {
			t.Errorf("event %d: expected kind %s, got %s", i, expected, tr.Events[i].Kind)
		}
	}

	// Verify offsets are non-negative and non-decreasing.
	for i := 1; i < len(tr.Events); i++ {
		if tr.Events[i].OffsetMs < tr.Events[i-1].OffsetMs {
			t.Errorf("event %d offset %d < event %d offset %d", i, tr.Events[i].OffsetMs, i-1, tr.Events[i-1].OffsetMs)
		}
	}
}

func TestDecodeEvent(t *testing.T) {
	tr := NewTrace("test", "test", "/tmp")
	tr.Record(EventUserInput, UserInputData{Text: "test input"})

	data, err := DecodeEvent[UserInputData](tr.Events[0])
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if data.Text != "test input" {
		t.Errorf("expected 'test input', got %q", data.Text)
	}
}

func TestTraceSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, ".golem", "sessions", "testhash")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	tr := NewTrace("claude-sonnet-4", "anthropic", dir)
	tr.Record(EventUserInput, UserInputData{Text: "test prompt"})
	tr.Record(EventTextDelta, TextDeltaData{Text: "response"})
	tr.Record(EventAgentDone, AgentDoneData{InputTokens: 100, OutputTokens: 50})

	// Save manually to the known directory.
	raw, err := json.Marshal(tr)
	if err != nil {
		t.Fatal(err)
	}
	filename := tr.StartTime.Format("2006-01-02T15-04-05") + ".replay.json"
	if err := os.WriteFile(filepath.Join(sessDir, filename), raw, 0644); err != nil {
		t.Fatal(err)
	}

	// Load it back.
	loaded, err := LoadTrace(filepath.Join(sessDir, filename))
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if loaded.Model != "claude-sonnet-4" {
		t.Errorf("expected model claude-sonnet-4, got %s", loaded.Model)
	}
	if len(loaded.Events) != 3 {
		t.Errorf("expected 3 events, got %d", len(loaded.Events))
	}
}

func TestTraceRoundtrip(t *testing.T) {
	tr := NewTrace("gpt-4o", "openai", "/home/user/project")
	tr.Record(EventToolCall, ToolCallData{
		CallID:  "tc_abc",
		Name:    "edit",
		Args:    `{"file_path":"/tmp/f.go","old_string":"foo","new_string":"bar"}`,
		RawArgs: `{"file_path":"/tmp/f.go","old_string":"foo","new_string":"bar"}`,
	})
	tr.Record(EventToolResult, ToolResultData{
		CallID: "tc_abc",
		Name:   "edit",
		Result: "OK: edited /tmp/f.go",
	})

	// Marshal and unmarshal.
	raw, err := json.Marshal(tr)
	if err != nil {
		t.Fatal(err)
	}

	var loaded ReplayTrace
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}

	if len(loaded.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(loaded.Events))
	}

	// Decode the tool call.
	tc, err := DecodeEvent[ToolCallData](loaded.Events[0])
	if err != nil {
		t.Fatal(err)
	}
	if tc.CallID != "tc_abc" || tc.Name != "edit" {
		t.Errorf("unexpected tool call data: %+v", tc)
	}

	// Decode the tool result.
	tr2, err := DecodeEvent[ToolResultData](loaded.Events[1])
	if err != nil {
		t.Fatal(err)
	}
	if tr2.Result != "OK: edited /tmp/f.go" {
		t.Errorf("unexpected result: %s", tr2.Result)
	}
}

func TestListTracesEmpty(t *testing.T) {
	dir := t.TempDir()
	traces, err := ListTraces(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) != 0 {
		t.Errorf("expected empty traces, got %d", len(traces))
	}
}

func TestTraceTimingOffsets(t *testing.T) {
	tr := NewTrace("test", "test", "/tmp")

	// Record events with small delays to verify offsets increase.
	tr.Record(EventUserInput, UserInputData{Text: "start"})
	time.Sleep(5 * time.Millisecond)
	tr.Record(EventTextDelta, TextDeltaData{Text: "response"})

	if len(tr.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(tr.Events))
	}

	// Second event should have a larger offset.
	if tr.Events[1].OffsetMs <= tr.Events[0].OffsetMs {
		t.Errorf("expected increasing offsets: %d <= %d", tr.Events[1].OffsetMs, tr.Events[0].OffsetMs)
	}
}

func TestTraceErrorEvent(t *testing.T) {
	tr := NewTrace("test", "test", "/tmp")
	tr.Record(EventToolResult, ToolResultData{
		CallID: "tc_1",
		Name:   "bash",
		Result: "",
		Error:  "command not found",
	})

	data, err := DecodeEvent[ToolResultData](tr.Events[0])
	if err != nil {
		t.Fatal(err)
	}
	if data.Error != "command not found" {
		t.Errorf("expected error 'command not found', got %q", data.Error)
	}
}
