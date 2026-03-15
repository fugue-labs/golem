package search

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/ui/chat"
	"github.com/fugue-labs/gollem/core"
)

func TestTokenize(t *testing.T) {
	tokens := tokenize("Hello World! This is a test_123.")
	// "this", "is", "a" are stop words; "hello", "world", "test_123" should remain.
	want := map[string]bool{"hello": true, "world": true, "test_123": true}
	got := make(map[string]bool)
	for _, tok := range tokens {
		got[tok] = true
	}
	for w := range want {
		if !got[w] {
			t.Errorf("expected token %q, not found in %v", w, tokens)
		}
	}
}

func TestIndexSearchBasic(t *testing.T) {
	idx := NewIndex()
	idx.Add("doc1", "How to fix a flaky test in the CI pipeline")
	idx.Add("doc2", "Database migration guide for PostgreSQL")
	idx.Add("doc3", "Authentication bug with OAuth tokens expiring")

	results := idx.Search("flaky test", 10)
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'flaky test'")
	}
	if results[0].DocID != "doc1" {
		t.Errorf("expected doc1 as top result, got %s", results[0].DocID)
	}
}

func TestIndexSearchNoResults(t *testing.T) {
	idx := NewIndex()
	idx.Add("doc1", "Hello world program in Go")

	results := idx.Search("quantum physics", 10)
	if len(results) != 0 {
		t.Errorf("expected no results, got %d", len(results))
	}
}

func TestIndexSearchMultipleTerms(t *testing.T) {
	idx := NewIndex()
	idx.Add("doc1", "OAuth authentication error handling")
	idx.Add("doc2", "OAuth token refresh implementation")
	idx.Add("doc3", "Basic error handling patterns in Go")

	results := idx.Search("OAuth error", 10)
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	// doc1 has both "OAuth" and "error", should rank highest.
	if results[0].DocID != "doc1" {
		t.Errorf("expected doc1 as top result, got %s", results[0].DocID)
	}
}

func TestIndexSearchMaxResults(t *testing.T) {
	idx := NewIndex()
	for i := 0; i < 20; i++ {
		idx.Add("doc"+string(rune('A'+i)), "common search term found here")
	}
	results := idx.Search("common search", 5)
	if len(results) > 5 {
		t.Errorf("expected at most 5 results, got %d", len(results))
	}
}

func TestIndexEmpty(t *testing.T) {
	idx := NewIndex()
	results := idx.Search("anything", 10)
	if len(results) != 0 {
		t.Errorf("expected no results from empty index, got %d", len(results))
	}
}

func TestIndexLen(t *testing.T) {
	idx := NewIndex()
	if idx.Len() != 0 {
		t.Errorf("expected 0, got %d", idx.Len())
	}
	idx.Add("d1", "hello")
	idx.Add("d2", "world")
	if idx.Len() != 2 {
		t.Errorf("expected 2, got %d", idx.Len())
	}
}

func TestExtractSnippet(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog and finds a bug in the authentication module"
	snippet := extractSnippet(text, "authentication", 60)
	if snippet == "" {
		t.Error("expected non-empty snippet")
	}
	// Snippet should contain the query term area.
	if len(snippet) > 80 { // Allow some padding for ellipsis.
		t.Errorf("snippet too long: %d chars", len(snippet))
	}
}

func TestExtractSessionTextStructuredMessages(t *testing.T) {
	messages := []map[string]any{
		{
			"kind": "request",
			"data": map[string]any{
				"parts": []map[string]any{{
					"type": "user-prompt",
					"data": map[string]any{"content": "Investigate flaky authentication test"},
				}},
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			},
		},
		{
			"kind": "response",
			"data": map[string]any{
				"parts": []map[string]any{{
					"type": "text",
					"data": map[string]any{"content": "The bug is in token refresh handling."},
				}, {
					"type": "tool-call",
					"data": map[string]any{"tool_name": "grep", "args_json": `{"pattern":"token"}`},
				}},
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			},
		},
	}
	raw, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}

	text := extractSessionText(&sessionData{Prompt: "Fix auth", Messages: raw})
	for _, want := range []string{"Fix auth", "Investigate flaky authentication test", "The bug is in token refresh handling.", "grep", `{"pattern":"token"}`} {
		if !strings.Contains(text, want) {
			t.Fatalf("extractSessionText() missing %q in %q", want, text)
		}
	}
}

func TestSearchSessionsPrefersTranscriptSnippet(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("Setenv HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	projectDir := t.TempDir()
	sessionDir, err := sessionsBaseDir()
	if err != nil {
		t.Fatalf("sessionsBaseDir: %v", err)
	}
	projectHashDir := filepath.Join(sessionDir, strings.Repeat("a", 16))
	if err := os.MkdirAll(projectHashDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	messages := []map[string]any{
		{
			"kind": "request",
			"data": map[string]any{
				"parts": []map[string]any{{
					"type": "user-prompt",
					"data": map[string]any{"content": "Search for authentication fix"},
				}},
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			},
		},
	}
	transcript := []map[string]any{
		{"kind": 0, "content": "search for authentication fix"},
		{"kind": 1, "content": "Found the authentication fix in refresh token validation."},
		{"kind": 2, "tool_name": "grep", "tool_args": "token refresh validation"},
	}
	session := map[string]any{
		"messages":   messages,
		"transcript": transcript,
		"work_dir":   projectDir,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"prompt":     "Authentication bug",
		"model":      "gpt-5.4",
		"provider":   "openai",
		"usage":      map[string]any{"requests": 2},
	}
	raw, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectHashDir, "2026-03-15T12-00-00.json"), raw, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	results, err := SearchSessions("authentication fix", "", 5)
	if err != nil {
		t.Fatalf("SearchSessions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%d, want 1", len(results))
	}
	if !strings.Contains(results[0].Summary, "gpt-5.4 via openai") {
		t.Fatalf("summary=%q, want model/provider context", results[0].Summary)
	}
	if !strings.Contains(results[0].Summary, "2 requests") {
		t.Fatalf("summary=%q, want request count", results[0].Summary)
	}
	if !strings.Contains(results[0].Snippet, "Assistant: Found the authentication fix") {
		t.Fatalf("snippet=%q, want transcript-aware assistant line", results[0].Snippet)
	}
	if !strings.Contains(results[0].Snippet, "User: search for authentication fix") {
		t.Fatalf("snippet=%q, want neighboring transcript context", results[0].Snippet)
	}
}

func TestSearchSessionsSummaryIncludesWorkflowState(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("Setenv HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	projectDir := t.TempDir()
	sessionDir, err := sessionsBaseDir()
	if err != nil {
		t.Fatalf("sessionsBaseDir: %v", err)
	}
	projectHashDir := filepath.Join(sessionDir, strings.Repeat("b", 16))
	if err := os.MkdirAll(projectHashDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	session := map[string]any{
		"messages": []map[string]any{
			{
				"kind": "request",
				"data": map[string]any{
					"parts": []map[string]any{
						{
							"type": "user-prompt",
							"data": map[string]any{"content": "resume the workflow"},
						},
					},
				},
			},
		},
		"work_dir":           projectDir,
		"timestamp":          time.Now().UTC().Format(time.RFC3339),
		"prompt":             "Workflow resume memory",
		"model":              "gpt-5.4",
		"provider":           "openai",
		"usage":              map[string]any{"requests": 3},
		"plan_state":         map[string]any{"tasks": []map[string]any{{"id": "T1", "description": "ship it", "status": "in_progress"}}},
		"invariant_state":    map[string]any{"items": []map[string]any{{"id": "I1", "description": "tests pass", "status": "pass"}}},
		"verification_state": map[string]any{"entries": []map[string]any{{"id": "V1", "command": "go test ./...", "status": "pass"}}},
		"spec_state":         map[string]any{"file": "spec.md", "phase": "approved"},
	}
	raw, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectHashDir, "2026-03-15T13-00-00.json"), raw, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	results, err := SearchSessions("workflow", "", 5)
	if err != nil {
		t.Fatalf("SearchSessions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%d, want 1", len(results))
	}
	for _, want := range []string{"gpt-5.4 via openai", "3 requests", "plan", "invariants", "verification", "spec"} {
		if !strings.Contains(results[0].Summary, want) {
			t.Fatalf("summary=%q, want %q", results[0].Summary, want)
		}
	}
}

func TestSearchSessionsDecodesRealSavedTranscript(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("Setenv HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	projectDir := t.TempDir()
	msgs := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "Investigate replay memory surface"}}},
	}
	transcript := []*chat.Message{
		{Kind: chat.KindUser, Content: "Investigate replay memory surface"},
		{Kind: chat.KindAssistant, Content: "I found the replay memory surface issue in session snippets."},
		{Kind: chat.KindToolCall, ToolName: "grep", ToolArgs: "session snippets", RawArgs: `{"pattern":"session snippets"}`},
	}

	if err := agent.SaveSession(projectDir, msgs, transcript, nil, core.RunUsage{Requests: 1}, "gpt-5.4", "openai", "Replay memory surface", nil, nil, nil, nil); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	results, err := SearchSessions("memory surface", projectDir, 5)
	if err != nil {
		t.Fatalf("SearchSessions: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if !strings.Contains(results[0].Snippet, "Assistant: I found the replay memory surface issue") {
		t.Fatalf("snippet=%q, want assistant transcript content", results[0].Snippet)
	}
	if !strings.Contains(results[0].Snippet, "User: Investigate replay memory surface") {
		t.Fatalf("snippet=%q, want user transcript context", results[0].Snippet)
	}
}
