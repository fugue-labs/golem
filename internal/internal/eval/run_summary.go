package eval

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/ui/chat"
	uiverification "github.com/fugue-labs/golem/internal/ui/verification"
	"github.com/fugue-labs/gollem/core"
)

type TranscriptEntry struct {
	Kind       string `json:"kind"`
	Content    string `json:"content,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	ToolArgs   string `json:"tool_args,omitempty"`
	ToolStatus string `json:"tool_status,omitempty"`
}

type ToolTraceEntry struct {
	CallID string `json:"call_id,omitempty"`
	Name   string `json:"name"`
	Args   string `json:"args,omitempty"`
	Result string `json:"result,omitempty"`
	Status string `json:"status,omitempty"`
}

type VerificationSummary struct {
	Badge      string   `json:"badge,omitempty"`
	Total      int      `json:"total,omitempty"`
	Pass       int      `json:"pass,omitempty"`
	Fail       int      `json:"fail,omitempty"`
	Stale      int      `json:"stale,omitempty"`
	InProgress int      `json:"in_progress,omitempty"`
	Commands   []string `json:"commands,omitempty"`
}

type RunSummary struct {
	Prompt               string              `json:"prompt,omitempty"`
	Runtime              agent.RuntimeReport `json:"runtime"`
	Transcript           []TranscriptEntry   `json:"transcript,omitempty"`
	ToolTrace            []ToolTraceEntry    `json:"tool_trace,omitempty"`
	ChangedFiles         []string            `json:"changed_files,omitempty"`
	Verification         VerificationSummary `json:"verification,omitempty"`
	FinalStatus          string              `json:"final_status"`
	Error                string              `json:"error,omitempty"`
	Usage                core.RunUsage       `json:"usage,omitempty"`
	VerificationCommands []string            `json:"verification_commands,omitempty"`
}

func BuildRunSummary(prompt string, runtime agent.RuntimeReport, messages []*chat.Message, verification uiverification.State, usage core.RunUsage, runErr error) RunSummary {
	summary := RunSummary{
		Prompt:      strings.TrimSpace(prompt),
		Runtime:     runtime,
		FinalStatus: finalStatus(runErr),
		Usage:       usage,
	}
	if runErr != nil {
		summary.Error = runErr.Error()
	}

	changedFiles := map[string]struct{}{}
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		summary.Transcript = append(summary.Transcript, TranscriptEntry{
			Kind:       messageKind(msg.Kind),
			Content:    strings.TrimSpace(msg.Content),
			ToolName:   msg.ToolName,
			ToolArgs:   msg.ToolArgs,
			ToolStatus: toolStatus(msg.Status),
		})
		if msg.Kind == chat.KindToolCall {
			summary.ToolTrace = append(summary.ToolTrace, ToolTraceEntry{
				CallID: msg.CallID,
				Name:   msg.ToolName,
				Args:   strings.TrimSpace(msg.RawArgs),
				Result: strings.TrimSpace(msg.Content),
				Status: toolStatus(msg.Status),
			})
			for _, path := range changedFilesFromArgs(msg.ToolName, msg.RawArgs) {
				changedFiles[path] = struct{}{}
			}
		}
	}

	summary.ChangedFiles = collectSorted(changedFiles)
	total, pass, fail, stale, inProgress := verification.Counts()
	commands := make([]string, 0, len(verification.Entries))
	for _, entry := range verification.Entries {
		commands = append(commands, entry.Command)
	}
	summary.Verification = VerificationSummary{
		Badge:      verification.Badge(),
		Total:      total,
		Pass:       pass,
		Fail:       fail,
		Stale:      stale,
		InProgress: inProgress,
		Commands:   commands,
	}
	summary.VerificationCommands = append([]string(nil), commands...)
	return summary
}

func finalStatus(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	return "error"
}

func messageKind(kind chat.MessageKind) string {
	switch kind {
	case chat.KindUser:
		return "user"
	case chat.KindAssistant:
		return "assistant"
	case chat.KindToolCall:
		return "tool_call"
	case chat.KindThinking:
		return "thinking"
	case chat.KindError:
		return "error"
	default:
		return "unknown"
	}
}

func toolStatus(status chat.ToolStatus) string {
	switch status {
	case chat.ToolRunning:
		return "running"
	case chat.ToolSuccess:
		return "success"
	case chat.ToolError:
		return "error"
	default:
		return "pending"
	}
}

func changedFilesFromArgs(toolName, rawArgs string) []string {
	if !isMutatingTool(toolName) || strings.TrimSpace(rawArgs) == "" {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return nil
	}
	var paths []string
	for _, key := range []string{"file_path", "path"} {
		if value, ok := args[key].(string); ok && strings.TrimSpace(value) != "" {
			paths = append(paths, strings.TrimSpace(value))
		}
	}
	return paths
}

func isMutatingTool(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "edit", "write", "multi_edit":
		return true
	default:
		return false
	}
}

func collectSorted(items map[string]struct{}) []string {
	result := make([]string, 0, len(items))
	for item := range items {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}
