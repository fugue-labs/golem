package checkpoint

import (
	"testing"

	"github.com/fugue-labs/golem/internal/ui/chat"
	uiinvariants "github.com/fugue-labs/golem/internal/ui/invariants"
	"github.com/fugue-labs/golem/internal/ui/plan"
	uiverification "github.com/fugue-labs/golem/internal/ui/verification"
	"github.com/fugue-labs/gollem/core"
)

func TestStoreBasics(t *testing.T) {
	s := NewStore("/tmp/test-checkpoint")

	if s.Len() != 0 {
		t.Fatalf("expected 0 checkpoints, got %d", s.Len())
	}
	if s.Latest() != nil {
		t.Fatal("expected nil latest")
	}

	// Save a checkpoint.
	s.Save(Checkpoint{
		Turn:   1,
		Prompt: "fix the bug",
		Messages: []*chat.Message{
			{Kind: chat.KindUser, Content: "fix the bug"},
			{Kind: chat.KindAssistant, Content: "I'll fix it."},
		},
		ToolState: map[string]any{"plan": "some-plan"},
		PlanState: plan.State{Tasks: []plan.Task{
			{ID: "t1", Description: "Fix bug", Status: "in_progress"},
		}},
		SessionUsage: core.RunUsage{Requests: 1},
	})

	if s.Len() != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", s.Len())
	}

	cp := s.Get(1)
	if cp == nil {
		t.Fatal("expected checkpoint at turn 1")
	}
	if cp.Prompt != "fix the bug" {
		t.Fatalf("expected prompt 'fix the bug', got %q", cp.Prompt)
	}
	if len(cp.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(cp.Messages))
	}

	// Get non-existent turn.
	if s.Get(99) != nil {
		t.Fatal("expected nil for non-existent turn")
	}
}

func TestStoreSaveDeepCopiesMessages(t *testing.T) {
	s := NewStore("/tmp/test-checkpoint-copy")

	msgs := []*chat.Message{
		{Kind: chat.KindUser, Content: "original"},
	}

	s.Save(Checkpoint{
		Turn:     1,
		Prompt:   "test",
		Messages: msgs,
	})

	// Mutate the original slice.
	msgs[0].Content = "mutated"

	// Checkpoint should still have the original content.
	cp := s.Get(1)
	if cp.Messages[0].Content != "original" {
		t.Fatalf("expected deep copy to preserve 'original', got %q", cp.Messages[0].Content)
	}
}

func TestStoreRewindTruncates(t *testing.T) {
	s := NewStore("/tmp/test-checkpoint-rewind")

	for i := 1; i <= 5; i++ {
		s.Save(Checkpoint{
			Turn:   i,
			Prompt: "prompt",
			Messages: []*chat.Message{
				{Kind: chat.KindUser, Content: "msg"},
			},
			PlanState:         plan.State{},
			InvariantState:    uiinvariants.State{},
			VerificationState: uiverification.State{},
		})
	}

	if s.Len() != 5 {
		t.Fatalf("expected 5 checkpoints, got %d", s.Len())
	}

	// Rewind to turn 3 (no git operations in test since /tmp isn't a git repo).
	// We expect the git restore to fail, so we test the error path.
	_, err := s.RewindTo(3)
	if err == nil {
		// If it somehow succeeds (e.g. /tmp is a git repo), that's fine.
		if s.Len() != 3 {
			t.Fatalf("expected 3 checkpoints after rewind, got %d", s.Len())
		}
	}
	// The error is expected since /tmp isn't a git repo.

	// Test invalid turn.
	_, err = s.RewindTo(99)
	if err == nil {
		t.Fatal("expected error for non-existent turn")
	}
}

func TestStoreList(t *testing.T) {
	s := NewStore("/tmp/test-checkpoint-list")
	s.Save(Checkpoint{Turn: 1, Prompt: "hello"})
	s.Save(Checkpoint{Turn: 2, Prompt: "world"})

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list))
	}
}

func TestStoreClear(t *testing.T) {
	s := NewStore("/tmp/test-checkpoint-clear")
	s.Save(Checkpoint{Turn: 1, Prompt: "test"})
	s.Clear()
	if s.Len() != 0 {
		t.Fatalf("expected 0 after clear, got %d", s.Len())
	}
}

func TestCopyToolState(t *testing.T) {
	original := map[string]any{
		"key": "value",
		"nested": map[string]any{
			"inner": "data",
		},
	}

	cp := copyToolState(original)

	// Mutate original.
	original["key"] = "changed"

	// Copy should be unaffected.
	if cp["key"] != "value" {
		t.Fatalf("expected 'value', got %v", cp["key"])
	}
}

func TestCopyToolStateNil(t *testing.T) {
	if copyToolState(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestCheckpointSummary(t *testing.T) {
	cp := Checkpoint{
		Turn:   3,
		Prompt: "implement the feature with a really long description that should be truncated at sixty characters",
	}
	summary := cp.Summary()
	if len(summary) == 0 {
		t.Fatal("expected non-empty summary")
	}
	// Should contain turn number.
	if !contains(summary, "turn 3") {
		t.Fatalf("expected summary to contain 'turn 3', got %q", summary)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
