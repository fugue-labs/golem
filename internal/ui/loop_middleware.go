// loop_middleware.go — Diffuse read-loop detection.
//
// Gollem's LoopDetectionMiddleware catches per-file repetition (the same file
// read 8+ times). This middleware catches a different pattern: the agent reads
// MANY DIFFERENT files without ever taking an action (edit, write, bash). Each
// file is read only once or twice, so no per-file counter triggers, but the
// aggregate behavior is clearly a read-only loop burning turns.

package ui

import (
	"context"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// readOnlyTools are tools that read/search without modifying state.
var readOnlyTools = map[string]bool{
	"view": true,
	"grep": true,
	"glob": true,
	"ls":   true,
}

// actionTools are tools that modify state or execute commands.
var actionTools = map[string]bool{
	"edit":       true,
	"multi_edit": true,
	"write":      true,
	"bash":       true,
}

// diffuseReadLoopMiddleware detects when the agent reads many different files
// across consecutive turns without taking any action. After threshold turns of
// read-only behavior, it injects a message telling the agent to stop reading
// and start acting.
func diffuseReadLoopMiddleware(threshold int) core.AgentMiddleware {
	if threshold <= 0 {
		threshold = 6
	}

	var mu sync.Mutex
	consecutiveReadOnlyTurns := 0

	return core.RequestOnlyMiddleware(func(
		ctx context.Context,
		messages []core.ModelMessage,
		settings *core.ModelSettings,
		params *core.ModelRequestParameters,
		next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error),
	) (*core.ModelResponse, error) {
		mu.Lock()
		// Scan recent messages for tool calls.
		for _, msg := range messages[max(0, len(messages)-2):] {
			resp, ok := msg.(core.ModelResponse)
			if !ok {
				continue
			}
			hasRead := false
			hasAction := false
			for _, part := range resp.Parts {
				tc, ok := part.(core.ToolCallPart)
				if !ok {
					continue
				}
				if readOnlyTools[tc.ToolName] {
					hasRead = true
				}
				if actionTools[tc.ToolName] {
					hasAction = true
				}
			}
			if hasAction {
				consecutiveReadOnlyTurns = 0
			} else if hasRead {
				consecutiveReadOnlyTurns++
			}
		}

		shouldWarn := consecutiveReadOnlyTurns >= threshold
		if shouldWarn {
			// Reduce instead of resetting — makes persistent loops trigger
			// faster on recurrence.
			consecutiveReadOnlyTurns = consecutiveReadOnlyTurns / 2
		}
		mu.Unlock()

		if shouldWarn {
			guidance := "WARNING: You have spent " +
				"multiple consecutive turns reading and searching files " +
				"without taking any action (editing, writing, or running commands). " +
				"This is analysis paralysis — you are burning turns without making progress.\n\n" +
				"STOP reading and START acting:\n" +
				"- If you have enough context to make a change, make it NOW\n" +
				"- If you're unsure how to proceed, try the simplest possible approach\n" +
				"- If the task is unclear, state what's blocking you instead of reading more files\n" +
				"- Write code based on what you already know — you can iterate after"
			messages = injectGuidanceIntoLastRequest(messages, guidance)
		}

		return next(ctx, messages, settings, params)
	})
}

// injectGuidanceIntoLastRequest appends a user prompt part to the last
// ModelRequest in the message list. This avoids consecutive user-role messages
// that some providers reject.
func injectGuidanceIntoLastRequest(messages []core.ModelMessage, content string) []core.ModelMessage {
	result := make([]core.ModelMessage, len(messages))
	copy(result, messages)
	for i := len(result) - 1; i >= 0; i-- {
		req, ok := result[i].(core.ModelRequest)
		if !ok {
			continue
		}
		newParts := make([]core.ModelRequestPart, len(req.Parts)+1)
		copy(newParts, req.Parts)
		newParts[len(req.Parts)] = core.UserPromptPart{
			Content:   content,
			Timestamp: time.Now(),
		}
		req.Parts = newParts
		result[i] = req
		return result
	}
	// Fallback: no ModelRequest found, append a new one.
	return append(result, core.ModelRequest{
		Parts:     []core.ModelRequestPart{core.UserPromptPart{Content: content}},
		Timestamp: time.Now(),
	})
}
