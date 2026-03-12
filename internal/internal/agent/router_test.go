package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/gollem/core"
)

func TestBuildRuntimeStateUsesRouterDecision(t *testing.T) {
	cfg := &config.Config{TeamMode: "auto"}
	model := core.NewTestModel(core.TextResponse("unused"))

	orig := promptRouterFunc
	promptRouterFunc = func(_ context.Context, _ *config.Config, gotModel core.Model, gotPrompt string) (promptRoute, error) {
		if gotModel != model {
			t.Fatalf("router model = %v, want active test model", gotModel)
		}
		if gotPrompt != "Coordinate changes across the repo" {
			t.Fatalf("prompt = %q", gotPrompt)
		}
		return promptRoute{
			Complexity:           "complex",
			ShouldEnableTeamMode: true,
			Confidence:           "high",
			Summary:              "repo-wide coordination needed",
		}, nil
	}
	t.Cleanup(func() { promptRouterFunc = orig })

	state := buildRuntimeState(context.Background(), cfg, "Coordinate changes across the repo", model)
	if !state.EffectiveTeamMode {
		t.Fatalf("expected team mode on, got off: %s", state.TeamModeReason)
	}
	if !strings.Contains(state.TeamModeReason, "auto router model=test-model complexity=complex confidence=high") {
		t.Fatalf("unexpected team mode reason: %q", state.TeamModeReason)
	}
}

func TestBuildRuntimeStateFallsBackOffWhenRouterFails(t *testing.T) {
	cfg := &config.Config{TeamMode: "auto"}
	model := core.NewTestModel(core.TextResponse("unused"))

	orig := promptRouterFunc
	promptRouterFunc = func(context.Context, *config.Config, core.Model, string) (promptRoute, error) {
		return promptRoute{}, errors.New("classifier offline")
	}
	t.Cleanup(func() { promptRouterFunc = orig })

	state := buildRuntimeState(context.Background(), cfg, "Investigate the whole service", model)
	if state.EffectiveTeamMode {
		t.Fatalf("expected conservative fallback off, got on: %s", state.TeamModeReason)
	}
	if !strings.Contains(state.TeamModeReason, "auto router unavailable: classifier offline") {
		t.Fatalf("unexpected fallback reason: %q", state.TeamModeReason)
	}
}

func TestPrepareRuntimeFallsBackWhenRouterModelResolutionFails(t *testing.T) {
	cfg := &config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", RouterModel: "gpt-5.4-mini", TeamMode: "auto"}

	origResolve := resolveRouterModelFunc
	origPrompt := promptRouterFunc
	resolveRouterModelFunc = func(_ *config.Config, activeModel core.Model) (core.Model, error) {
		return nil, errors.New("router model unavailable")
	}
	promptRouterFunc = func(_ context.Context, _ *config.Config, gotModel core.Model, _ string) (promptRoute, error) {
		if gotModel == nil || gotModel.ModelName() != "gpt-5.4" {
			t.Fatalf("expected active model fallback, got %v", gotModel)
		}
		return promptRoute{Complexity: "simple", Confidence: "high", Summary: "fallback active model"}, nil
	}
	t.Cleanup(func() {
		resolveRouterModelFunc = origResolve
		promptRouterFunc = origPrompt
	})

	state, err := PrepareRuntime(context.Background(), cfg, "Find the bug")
	if err != nil {
		t.Fatalf("PrepareRuntime() error = %v", err)
	}
	if !strings.Contains(state.TeamModeReason, `router_fallback="router model unavailable"`) {
		t.Fatalf("expected router fallback note, got %q", state.TeamModeReason)
	}
}

func TestSupportsNativeRouterOutput(t *testing.T) {
	if !supportsNativeRouterOutput(&config.Config{Provider: config.ProviderVertexAI}) {
		t.Fatal("expected vertexai native router output to be enabled")
	}
	if supportsNativeRouterOutput(&config.Config{Provider: config.ProviderOpenAI}) {
		t.Fatal("expected openai native router output to be disabled for router compatibility")
	}
}

func TestResolveRouterModelUsesOverrideWhenConfigured(t *testing.T) {
	cfg := &config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", RouterModel: "gpt-5.4-mini"}

	activeModel, err := createModel(cfg)
	if err != nil {
		t.Fatalf("createModel() error = %v", err)
	}
	routerModel, err := resolveRouterModel(cfg, activeModel)
	if err != nil {
		t.Fatalf("resolveRouterModel() error = %v", err)
	}
	if routerModel == activeModel {
		t.Fatal("expected a distinct router model when RouterModel is configured")
	}
	if got := routerModel.ModelName(); got != "gpt-5.4-mini" {
		t.Fatalf("router model name = %q", got)
	}
}
