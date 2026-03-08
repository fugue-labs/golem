package agent

import (
	"reflect"
	"strings"
	"testing"

	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/gollem/provider/openai"
)

func TestDecideTeamModeIgnoresNegatedHints(t *testing.T) {
	cfg := &config.Config{TeamMode: "auto"}

	enabled, reason := decideTeamMode(cfg, "I don't want to refactor this and please don't investigate unrelated issues.")
	if enabled {
		t.Fatalf("expected team mode off for negated hints, got on: %s", reason)
	}
}

func TestDecideTeamModeEnablesOnStrongAffirmedSignals(t *testing.T) {
	cfg := &config.Config{TeamMode: "auto"}
	prompt := strings.Join([]string{
		"Please parallelize the work across the repo.",
		"Use a subagent to investigate architecture risks.",
	}, "\n")

	enabled, reason := decideTeamMode(cfg, prompt)
	if !enabled {
		t.Fatalf("expected team mode on, got off: %s", reason)
	}
	if !strings.Contains(reason, "matched=") {
		t.Fatalf("expected matched hints in reason, got %q", reason)
	}
}

func TestInitialRuntimeStateSeparatesConfigFromRuntime(t *testing.T) {
	cfg := &config.Config{TeamMode: "on", DisableCodeMode: true}

	state := InitialRuntimeState(cfg)
	if !state.EffectiveTeamMode || state.TeamModeReason != "forced on" {
		t.Fatalf("unexpected team runtime state: %+v", state)
	}
	if state.CodeModeStatus != "off" || state.CodeModeError != "" {
		t.Fatalf("unexpected code runtime state: %+v", state)
	}
}

func TestBuildRuntimePromptAvoidsRedundantShowcaseBullets(t *testing.T) {
	cfg := &config.Config{Provider: config.ProviderOpenAI, Model: "gpt-test", TeamMode: "auto"}
	state := RuntimeState{CodeModeStatus: "pending"}

	prompt := buildRuntimePrompt(cfg, state, nil)
	if strings.Contains(prompt, "Compete with the best coding agents") || strings.Contains(prompt, "Showcase expectations") {
		t.Fatalf("runtime prompt still contains redundant showcase copy: %q", prompt)
	}
}

func TestCreateModelOpenAIUsesWebSocketForOfficialAPI(t *testing.T) {
	cfg := &config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"}

	model, err := createModel(cfg)
	if err != nil {
		t.Fatalf("createModel() error = %v", err)
	}
	provider, ok := model.(*openai.Provider)
	if !ok {
		t.Fatalf("model type = %T, want *openai.Provider", model)
	}

	fields := reflect.ValueOf(provider).Elem()
	if got := fields.FieldByName("transport").String(); got != "websocket" {
		t.Fatalf("transport = %q, want websocket", got)
	}
	if !fields.FieldByName("wsHTTPFallback").Bool() {
		t.Fatal("expected websocket HTTP fallback enabled for official OpenAI API")
	}
}

func TestCreateModelOpenAIBaseURLOverrideForcesHTTP(t *testing.T) {
	cfg := &config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", BaseURL: "https://proxy.example.test/v1"}

	model, err := createModel(cfg)
	if err != nil {
		t.Fatalf("createModel() error = %v", err)
	}
	provider, ok := model.(*openai.Provider)
	if !ok {
		t.Fatalf("model type = %T, want *openai.Provider", model)
	}

	if got := reflect.ValueOf(provider).Elem().FieldByName("transport").String(); got != "http" {
		t.Fatalf("transport = %q, want http", got)
	}
}
