package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

// makeToolResponse creates a ModelResponse with a single tool call.
func makeToolResponse(toolName string) core.ModelResponse {
	return core.ModelResponse{
		Parts: []core.ModelResponsePart{
			core.ToolCallPart{ToolName: toolName, ArgsJSON: "{}"},
		},
	}
}

// noopNext is a middleware next function that records call count.
func noopNext(calls *int) func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error) {
	return func(_ context.Context, msgs []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		*calls++
		// Check if guidance was injected by looking for the warning text
		// in the last ModelRequest's parts.
		return &core.ModelResponse{}, nil
	}
}

func TestDiffuseReadLoop_TriggersAfterThreshold(t *testing.T) {
	mw := diffuseReadLoopMiddleware(4)
	ctx := context.Background()
	settings := &core.ModelSettings{}
	params := &core.ModelRequestParameters{}

	var injected bool

	next := func(_ context.Context, msgs []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		// Check if the warning was injected into messages.
		for _, msg := range msgs {
			if req, ok := msg.(core.ModelRequest); ok {
				for _, part := range req.Parts {
					if up, ok := part.(core.UserPromptPart); ok {
						if strings.Contains(up.Content, "analysis paralysis") {
							injected = true
						}
					}
				}
			}
		}
		return &core.ModelResponse{}, nil
	}

	// Simulate 4 consecutive read-only turns (each reading a different file).
	for i := 0; i < 4; i++ {
		messages := []core.ModelMessage{
			core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "do work"}}},
			makeToolResponse("view"),
		}
		_, err := mw(ctx, messages, settings, params, next)
		if err != nil {
			t.Fatalf("middleware returned error: %v", err)
		}
	}

	if !injected {
		t.Error("expected loop warning to be injected after 4 consecutive read-only turns")
	}
}

func TestDiffuseReadLoop_ResetByAction(t *testing.T) {
	mw := diffuseReadLoopMiddleware(4)
	ctx := context.Background()
	settings := &core.ModelSettings{}
	params := &core.ModelRequestParameters{}

	var injected bool

	next := func(_ context.Context, msgs []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		for _, msg := range msgs {
			if req, ok := msg.(core.ModelRequest); ok {
				for _, part := range req.Parts {
					if up, ok := part.(core.UserPromptPart); ok {
						if strings.Contains(up.Content, "analysis paralysis") {
							injected = true
						}
					}
				}
			}
		}
		return &core.ModelResponse{}, nil
	}

	// 3 reads, then an action, then 3 more reads — should NOT trigger.
	for i := 0; i < 3; i++ {
		messages := []core.ModelMessage{
			core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "do work"}}},
			makeToolResponse("view"),
		}
		mw(ctx, messages, settings, params, next)
	}

	// Action resets the counter.
	messages := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "do work"}}},
		makeToolResponse("edit"),
	}
	mw(ctx, messages, settings, params, next)

	// 3 more reads — still below threshold of 4.
	for i := 0; i < 3; i++ {
		messages := []core.ModelMessage{
			core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "do work"}}},
			makeToolResponse("view"),
		}
		mw(ctx, messages, settings, params, next)
	}

	if injected {
		t.Error("warning should not trigger — action reset the counter before threshold was reached")
	}
}

func TestDiffuseReadLoop_GrepAndGlobCount(t *testing.T) {
	mw := diffuseReadLoopMiddleware(3)
	ctx := context.Background()
	settings := &core.ModelSettings{}
	params := &core.ModelRequestParameters{}

	var injected bool

	next := func(_ context.Context, msgs []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		for _, msg := range msgs {
			if req, ok := msg.(core.ModelRequest); ok {
				for _, part := range req.Parts {
					if up, ok := part.(core.UserPromptPart); ok {
						if strings.Contains(up.Content, "analysis paralysis") {
							injected = true
						}
					}
				}
			}
		}
		return &core.ModelResponse{}, nil
	}

	// Mix of search tools — all count as read-only.
	for _, tool := range []string{"grep", "glob", "view"} {
		messages := []core.ModelMessage{
			core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "do work"}}},
			makeToolResponse(tool),
		}
		mw(ctx, messages, settings, params, next)
	}

	if !injected {
		t.Error("expected loop warning after 3 mixed search/read turns")
	}
}

func TestDiffuseReadLoop_BashIsAction(t *testing.T) {
	mw := diffuseReadLoopMiddleware(3)
	ctx := context.Background()
	settings := &core.ModelSettings{}
	params := &core.ModelRequestParameters{}

	var injected bool

	next := func(_ context.Context, msgs []core.ModelMessage, _ *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
		for _, msg := range msgs {
			if req, ok := msg.(core.ModelRequest); ok {
				for _, part := range req.Parts {
					if up, ok := part.(core.UserPromptPart); ok {
						if strings.Contains(up.Content, "analysis paralysis") {
							injected = true
						}
					}
				}
			}
		}
		return &core.ModelResponse{}, nil
	}

	// 2 reads, then bash (action), then 2 reads — should NOT trigger.
	for i := 0; i < 2; i++ {
		messages := []core.ModelMessage{
			core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "do work"}}},
			makeToolResponse("view"),
		}
		mw(ctx, messages, settings, params, next)
	}

	messages := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "do work"}}},
		makeToolResponse("bash"),
	}
	mw(ctx, messages, settings, params, next)

	for i := 0; i < 2; i++ {
		messages := []core.ModelMessage{
			core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "do work"}}},
			makeToolResponse("view"),
		}
		mw(ctx, messages, settings, params, next)
	}

	if injected {
		t.Error("bash should count as an action and reset the counter")
	}
}
