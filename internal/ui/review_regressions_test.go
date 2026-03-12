package ui

import (
	"strings"
	"testing"

	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/gollem/core"
)

func TestRenderCostSummaryDoesNotDoubleCountAgentTrackedCost(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-4o"})
	usage := core.RunUsage{Requests: 1}
	usage.InputTokens = 1000
	usage.OutputTokens = 500
	m.sessionUsage = usage
	m.costTracker.Record("gpt-4o", m.sessionUsage)

	msg := m.renderCostSummaryMessage()
	if !strings.Contains(msg.Content, "**Total: $0.0075**") {
		t.Fatalf("unexpected total cost summary:\n%s", msg.Content)
	}
}

func TestHandleUndoRejectsBusyState(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", WorkingDir: t.TempDir()})
	m.busy = true

	got := m.handleUndo("/undo").Content
	if !strings.Contains(got, "Cannot undo while agent is running") {
		t.Fatalf("expected busy rejection, got: %q", got)
	}
}
