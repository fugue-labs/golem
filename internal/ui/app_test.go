package ui

import (
	"context"
	"testing"

	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/codetool"
)

func TestHandleRuntimePreparedReusesSessionAndPreparesFreshAgent(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", TeamMode: "auto"})
	oldSession := &codetool.Session{}
	m.runtime.Session = oldSession

	msg := runtimePreparedMsg{
		runID:  1,
		prompt: "inspect repo",
		runtime: agent.RuntimeState{
			Session:           nil,
			EffectiveTeamMode: false,
			TeamModeReason:    "auto router model=test-model complexity=simple confidence=high",
			CodeModeStatus:    "pending",
		},
	}
	m.runID = 1
	m.runCtx = context.Background()

	updated, cmd := m.handleRuntimePrepared(msg)
	model, ok := updated.(*Model)
	if !ok {
		t.Fatalf("handleRuntimePrepared returned %T", updated)
	}
	if model.runtime.Session != oldSession {
		t.Fatal("expected session reuse across runtime refresh")
	}
	if model.agent == nil {
		t.Fatal("expected fresh agent to be constructed")
	}
	if cmd == nil {
		t.Fatal("expected run command after runtime preparation")
	}
}

func TestAgentDoneMsgClearsAgentForNextPromptRouting(t *testing.T) {
	m := New(&config.Config{})
	m.runID = 7
	m.agent = &core.Agent[string]{}
	m.busy = true
	m.runCtx = context.Background()
	m.cancel = func() {}

	updated, _ := m.Update(agentDoneMsg{runID: 7})
	model := updated.(*Model)
	if model.agent != nil {
		t.Fatal("expected agent to be cleared after run completion")
	}
	if model.runCtx != nil {
		t.Fatal("expected run context to be cleared after run completion")
	}
}
