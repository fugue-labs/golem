package agent

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/provider/openai"
)

func TestInitialRuntimeStateSeparatesConfigFromRuntime(t *testing.T) {
	cfg := &config.Config{TeamMode: "on", DisableCodeMode: true, Model: "gpt-5.4-mini", RouterModel: "router-mini"}

	state := InitialRuntimeState(cfg)
	if !state.EffectiveTeamMode || state.TeamModeReason != "forced on" {
		t.Fatalf("unexpected team runtime state: %+v", state)
	}
	if state.CodeModeStatus != "off" || state.CodeModeError != "" {
		t.Fatalf("unexpected code runtime state: %+v", state)
	}
	if state.RouterModelName != "router-mini" {
		t.Fatalf("router model name = %q", state.RouterModelName)
	}
	if state.OpenImageStatus != "pending" {
		t.Fatalf("open image status = %q", state.OpenImageStatus)
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

func TestBuildRuntimePromptListsToolSurfaces(t *testing.T) {
	cfg := &config.Config{Provider: config.ProviderOpenAI, Model: "gpt-test", TeamMode: "auto"}
	state := RuntimeState{CodeModeStatus: "on", OpenImageStatus: "off", RouterModelName: "router-mini"}

	prompt := buildRuntimePrompt(cfg, state, nil)
	for _, want := range []string{
		"## Tool surfaces",
		"- guaranteed repo tools: bash, bash_status, bash_kill, view, edit, write, multi_edit, glob, grep, ls, lsp",
		"- guaranteed workflow tools: planning, invariants",
		"- delegate: on",
		"- execute_code: on",
		"- open_image: off",
		"- effective router model: router-mini",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("runtime prompt missing %q\n%s", want, prompt)
		}
	}
}

func TestBuildRuntimeStateUsesResolvedRouterModelName(t *testing.T) {
	cfg := &config.Config{Model: "leader", RouterModel: "router"}
	model := core.NewTestModel(core.TextResponse("unused"))
	model.SetName("router-resolved")
	state := buildRuntimeState(context.Background(), cfg, "", model)
	if state.RouterModelName != "router-resolved" {
		t.Fatalf("router model name = %q", state.RouterModelName)
	}
}

func TestPrepareRuntimePreservesResolvedOpenImageStatus(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "vision model", model: "gpt-5.4", want: "on"},
		{name: "text only model", model: "gpt-4-turbo-2024-04-09", want: "off"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Provider: config.ProviderOpenAI, Model: tt.model, TeamMode: "off"}
			state, err := PrepareRuntime(context.Background(), cfg, "inspect runtime")
			if err != nil {
				t.Fatalf("PrepareRuntime() error = %v", err)
			}
			if state.OpenImageStatus != tt.want {
				t.Fatalf("open image status = %q, want %q", state.OpenImageStatus, tt.want)
			}
		})
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
