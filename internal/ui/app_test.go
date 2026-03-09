package ui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/ui/chat"
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

func TestStaleStreamingEventsAreIgnored(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", APIKey: "test-key"})
	m.runID = 2
	m.runtime = agent.RuntimeState{CodeModeStatus: "on", OpenImageStatus: "off"}
	m.messages = []*chat.Message{{Kind: chat.KindAssistant, Content: "current"}}
	m.busy = true
	m.agent = &core.Agent[string]{}
	m.runCtx = context.Background()
	m.cancel = func() {}

	staleEvents := []struct {
		name string
		msg  tea.Msg
	}{
		{name: "text", msg: textDeltaMsg{runID: 1, text: "stale text"}},
		{name: "thinking", msg: thinkingDeltaMsg{runID: 1, text: "stale thinking"}},
		{name: "tool call", msg: toolCallMsg{runID: 1, name: "grep", args: "{}", rawArgs: "{}"}},
		{name: "runtime prepared", msg: runtimePreparedMsg{runID: 1, prompt: "old", runtime: agent.RuntimeState{CodeModeStatus: "off", OpenImageStatus: "on"}}},
		{name: "done", msg: agentDoneMsg{runID: 1, err: context.Canceled}},
	}

	for _, tt := range staleEvents {
		t.Run(tt.name, func(t *testing.T) {
			updated, _ := m.Update(tt.msg)
			model := updated.(*Model)
			if len(model.messages) != 1 || model.messages[0].Content != "current" {
				t.Fatalf("messages mutated by stale %s event: %+v", tt.name, model.messages)
			}
			if !model.busy {
				t.Fatalf("busy cleared by stale %s event", tt.name)
			}
			if model.agent == nil {
				t.Fatalf("agent cleared by stale %s event", tt.name)
			}
		})
	}
}

func TestHandleRuntimePreparedErrorReturnsAgentDoneError(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.runID = 3

	updated, cmd := m.handleRuntimePrepared(runtimePreparedMsg{runID: 3, err: context.DeadlineExceeded})
	if updated != m {
		t.Fatal("expected original model to be returned")
	}
	if cmd == nil {
		t.Fatal("expected follow-up command")
	}
	msg := cmd()
	done, ok := msg.(agentDoneMsg)
	if !ok {
		t.Fatalf("cmd() returned %T", msg)
	}
	if done.runID != 3 || done.err == nil || !strings.Contains(done.err.Error(), "deadline") {
		t.Fatalf("unexpected agentDoneMsg: %+v", done)
	}
}

func TestAgentDoneCapturesRunSummary(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", APIKey: "test-key"})
	m.runID = 9
	m.lastPrompt = "inspect repo"
	m.messages = []*chat.Message{{Kind: chat.KindUser, Content: "earlier run"}, {Kind: chat.KindUser, Content: "inspect repo"}}
	m.currentRunMessages = []*chat.Message{{Kind: chat.KindUser, Content: "inspect repo"}}
	m.busy = true

	updated, _ := m.Update(agentDoneMsg{runID: 9, usage: core.RunUsage{Requests: 1}, messages: []core.ModelMessage{}})
	model := updated.(*Model)
	if model.lastRunSummary == nil {
		t.Fatal("expected run summary to be captured")
	}
	if model.lastRunSummary.Prompt != "inspect repo" {
		t.Fatalf("prompt = %q", model.lastRunSummary.Prompt)
	}
	if model.lastRunSummary.FinalStatus != "success" {
		t.Fatalf("final status = %q", model.lastRunSummary.FinalStatus)
	}
	if got := len(model.lastRunSummary.Transcript); got != 1 {
		t.Fatalf("transcript entries=%d, want 1", got)
	}
}
