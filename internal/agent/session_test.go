package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
)

func TestSessionRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Build messages with a request.
	msgs := []core.ModelMessage{
		core.ModelRequest{
			Parts:     []core.ModelRequestPart{core.UserPromptPart{Content: "hello"}},
			Timestamp: time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC),
		},
	}
	transcript := []*struct {
		Kind    string `json:"kind"`
		Content string `json:"content"`
	}{
		{Kind: "user", Content: "hello"},
		{Kind: "assistant", Content: "hi there"},
	}
	toolState := map[string]any{"planning": map[string]any{"tasks": []any{}}}
	usage := core.RunUsage{Requests: 5, ToolCalls: 3}
	planState := map[string]any{"tasks": []map[string]any{{"id": "T1", "description": "plan task", "status": "completed"}}}
	invariantState := map[string]any{"extracted": true, "items": []map[string]any{{"id": "I1", "description": "inv1", "kind": "hard", "status": "pass"}}}
	verificationState := map[string]any{"entries": []map[string]any{{"id": "V1", "command": "go test", "status": "pass", "freshness": "fresh"}}}
	specState := map[string]any{"file_path": "spec.md", "phase": "approved"}

	err := SaveSession(dir, msgs, transcript, toolState, usage, "test-model", "test-provider", "test prompt", planState, invariantState, verificationState, specState)
	if err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	loaded, err := LoadLatestSession(dir)
	if err != nil {
		t.Fatalf("LoadLatestSession: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadLatestSession returned nil")
	}

	// Verify scalar fields.
	if loaded.Model != "test-model" {
		t.Fatalf("model=%q", loaded.Model)
	}
	if loaded.Provider != "test-provider" {
		t.Fatalf("provider=%q", loaded.Provider)
	}
	if loaded.Prompt != "test prompt" {
		t.Fatalf("prompt=%q", loaded.Prompt)
	}
	if loaded.Usage.Requests != 5 {
		t.Fatalf("usage.requests=%d", loaded.Usage.Requests)
	}
	if loaded.Usage.ToolCalls != 3 {
		t.Fatalf("usage.tool_calls=%d", loaded.Usage.ToolCalls)
	}

	// Verify messages round-trip.
	restoredMsgs, err := loaded.RestoreMessages()
	if err != nil {
		t.Fatalf("RestoreMessages: %v", err)
	}
	if len(restoredMsgs) != 1 {
		t.Fatalf("messages=%d, want 1", len(restoredMsgs))
	}

	// Verify transcript round-trip.
	if len(loaded.Transcript) == 0 {
		t.Fatal("transcript is empty after round-trip")
	}
	var restoredTranscript []map[string]string
	if err := json.Unmarshal(loaded.Transcript, &restoredTranscript); err != nil {
		t.Fatalf("unmarshal transcript: %v", err)
	}
	if len(restoredTranscript) != 2 {
		t.Fatalf("transcript entries=%d, want 2", len(restoredTranscript))
	}
	if restoredTranscript[0]["content"] != "hello" {
		t.Fatalf("transcript[0].content=%q", restoredTranscript[0]["content"])
	}

	// Verify tool state round-trip.
	if loaded.ToolState == nil {
		t.Fatal("tool_state is nil")
	}
	if _, ok := loaded.ToolState["planning"]; !ok {
		t.Fatal("tool_state missing 'planning'")
	}

	// Verify plan state round-trip.
	if len(loaded.PlanState) == 0 {
		t.Fatal("plan_state is empty")
	}
	var restoredPlan map[string]any
	if err := json.Unmarshal(loaded.PlanState, &restoredPlan); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}
	if _, ok := restoredPlan["tasks"]; !ok {
		t.Fatal("plan_state missing 'tasks'")
	}

	// Verify invariant state round-trip.
	if len(loaded.InvariantState) == 0 {
		t.Fatal("invariant_state is empty")
	}

	// Verify verification state round-trip.
	if len(loaded.VerificationState) == 0 {
		t.Fatal("verification_state is empty")
	}

	// Verify spec state round-trip.
	if len(loaded.SpecState) == 0 {
		t.Fatal("spec_state is empty")
	}
	var restoredSpec map[string]any
	if err := json.Unmarshal(loaded.SpecState, &restoredSpec); err != nil {
		t.Fatalf("unmarshal spec: %v", err)
	}
	if restoredSpec["phase"] != "approved" {
		t.Fatalf("spec phase=%v", restoredSpec["phase"])
	}
}

func TestRestoreJSON(t *testing.T) {
	t.Run("nil raw message leaves target unchanged", func(t *testing.T) {
		var target struct{ Name string }
		target.Name = "original"

		err := RestoreJSON[struct{ Name string }](nil, &target)
		if err != nil {
			t.Fatalf("RestoreJSON: %v", err)
		}
		if target.Name != "original" {
			t.Fatalf("target.Name=%q, want original", target.Name)
		}
	})

	t.Run("empty raw message leaves target unchanged", func(t *testing.T) {
		var target struct{ Name string }
		target.Name = "original"

		err := RestoreJSON(json.RawMessage{}, &target)
		if err != nil {
			t.Fatalf("RestoreJSON: %v", err)
		}
		if target.Name != "original" {
			t.Fatalf("target.Name=%q, want original", target.Name)
		}
	})

	t.Run("valid raw message populates target", func(t *testing.T) {
		raw := json.RawMessage(`{"Name":"restored","Value":42}`)
		var target struct {
			Name  string
			Value int
		}
		err := RestoreJSON(raw, &target)
		if err != nil {
			t.Fatalf("RestoreJSON: %v", err)
		}
		if target.Name != "restored" || target.Value != 42 {
			t.Fatalf("target={Name:%q, Value:%d}", target.Name, target.Value)
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		raw := json.RawMessage(`{invalid`)
		var target struct{ Name string }
		err := RestoreJSON(raw, &target)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})
}

func TestBackwardCompatibility(t *testing.T) {
	dir := t.TempDir()
	hash := projectHash(dir)
	sessionDir := filepath.Join(os.TempDir(), ".golem-test-compat", hash)
	defer os.RemoveAll(filepath.Join(os.TempDir(), ".golem-test-compat"))

	// Create a minimal session JSON WITHOUT new fields (pre-upgrade format).
	// The messages field uses gollem's envelope format:
	// [{"kind":"request","data":{"parts":[{"type":"user-prompt","data":{"content":"..."}}]}}]
	minimalSession := `{
		"messages": [{"kind":"request","data":{"parts":[{"type":"user-prompt","data":{"content":"old message"}}],"timestamp":"0001-01-01T00:00:00Z"}}],
		"tool_state": {"key": "value"},
		"usage": {"requests": 2, "tool_calls": 1},
		"model": "old-model",
		"provider": "old-provider",
		"work_dir": "` + dir + `",
		"timestamp": "2026-01-01T00:00:00Z"
	}`

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "2026-01-01T00-00-00.json"), []byte(minimalSession), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Temporarily override SessionDir by loading from the right path.
	raw, err := os.ReadFile(filepath.Join(sessionDir, "2026-01-01T00-00-00.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var data SessionData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify loading does NOT error.
	if data.Model != "old-model" {
		t.Fatalf("model=%q", data.Model)
	}
	if data.Usage.Requests != 2 {
		t.Fatalf("usage.requests=%d", data.Usage.Requests)
	}

	// New fields should be zero/nil.
	if len(data.Transcript) != 0 {
		t.Fatalf("transcript should be empty, got %d bytes", len(data.Transcript))
	}
	if len(data.PlanState) != 0 {
		t.Fatalf("plan_state should be empty, got %d bytes", len(data.PlanState))
	}
	if len(data.InvariantState) != 0 {
		t.Fatalf("invariant_state should be empty, got %d bytes", len(data.InvariantState))
	}
	if len(data.VerificationState) != 0 {
		t.Fatalf("verification_state should be empty, got %d bytes", len(data.VerificationState))
	}
	if len(data.SpecState) != 0 {
		t.Fatalf("spec_state should be empty, got %d bytes", len(data.SpecState))
	}
	if data.Prompt != "" {
		t.Fatalf("prompt should be empty, got %q", data.Prompt)
	}

	// Verify RestoreJSON with nil new fields doesn't error (backward compat).
	type planT struct {
		Tasks []struct {
			ID string `json:"id"`
		} `json:"tasks"`
	}
	var plan planT
	if err := RestoreJSON(data.PlanState, &plan); err != nil {
		t.Fatalf("RestoreJSON on nil PlanState: %v", err)
	}
	if len(plan.Tasks) != 0 {
		t.Fatalf("expected zero tasks from nil PlanState, got %d", len(plan.Tasks))
	}

	// Verify messages still deserialize.
	msgs, err := data.RestoreMessages()
	if err != nil {
		t.Fatalf("RestoreMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages=%d, want 1", len(msgs))
	}
}

func TestLoadLatestSession_NoSession(t *testing.T) {
	dir := t.TempDir()
	session, err := LoadLatestSession(filepath.Join(dir, "nonexistent"))
	if err != nil {
		t.Fatalf("LoadLatestSession: %v", err)
	}
	if session != nil {
		t.Fatal("expected nil session for nonexistent dir")
	}
}

func TestLoadLatestSession_PicksLatest(t *testing.T) {
	dir := t.TempDir()

	// Save two sessions with different models to distinguish them.
	msgs := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "test"}}},
	}

	if err := SaveSession(dir, msgs, nil, nil, core.RunUsage{}, "first-model", "p", "", nil, nil, nil, nil); err != nil {
		t.Fatalf("first save: %v", err)
	}
	// Ensure different timestamp in filename.
	time.Sleep(1100 * time.Millisecond)
	if err := SaveSession(dir, msgs, nil, nil, core.RunUsage{}, "second-model", "p", "", nil, nil, nil, nil); err != nil {
		t.Fatalf("second save: %v", err)
	}

	loaded, err := LoadLatestSession(dir)
	if err != nil {
		t.Fatalf("LoadLatestSession: %v", err)
	}
	if loaded.Model != "second-model" {
		t.Fatalf("model=%q, want second-model", loaded.Model)
	}
}

func TestSaveSession_NilOptionalFields(t *testing.T) {
	dir := t.TempDir()

	msgs := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "test"}}},
	}

	// Save with all optional state fields nil.
	err := SaveSession(dir, msgs, nil, nil, core.RunUsage{}, "model", "provider", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("SaveSession with nils: %v", err)
	}

	loaded, err := LoadLatestSession(dir)
	if err != nil {
		t.Fatalf("LoadLatestSession: %v", err)
	}
	if len(loaded.Transcript) != 0 {
		t.Fatal("expected nil transcript")
	}
	if len(loaded.PlanState) != 0 {
		t.Fatal("expected nil plan_state")
	}
}

func TestProjectHash_Deterministic(t *testing.T) {
	h1 := projectHash("/some/path")
	h2 := projectHash("/some/path")
	h3 := projectHash("/other/path")
	if h1 != h2 {
		t.Fatal("same path should produce same hash")
	}
	if h1 == h3 {
		t.Fatal("different paths should produce different hashes")
	}
	if len(h1) != 16 {
		t.Fatalf("hash length=%d, want 16 hex chars", len(h1))
	}
}
