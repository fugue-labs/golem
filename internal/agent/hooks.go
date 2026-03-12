package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// HooksConfig defines user-configurable shell hooks that run before/after
// tool execution. Loaded from ~/.golem/hooks.json.
type HooksConfig struct {
	PreToolUse  []ShellHook `json:"pre_tool_use,omitempty"`
	PostToolUse []ShellHook `json:"post_tool_use,omitempty"`
}

// ShellHook is a shell command that runs for matching tool calls.
type ShellHook struct {
	Command string   `json:"command"`           // shell command to run
	Tools   []string `json:"tools,omitempty"`    // tool names to match (empty = all)
	Timeout int      `json:"timeout,omitempty"`  // timeout in seconds (default 10)
}

// LoadHooksConfig reads hooks configuration from ~/.golem/hooks.json.
// Returns an empty config if the file doesn't exist.
func LoadHooksConfig() HooksConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return HooksConfig{}
	}
	data, err := os.ReadFile(filepath.Join(home, ".golem", "hooks.json"))
	if err != nil {
		return HooksConfig{}
	}
	var cfg HooksConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return HooksConfig{}
	}
	return cfg
}

// BuildHook converts a HooksConfig into a gollem core.Hook that executes
// shell commands before and after tool calls.
func BuildHook(cfg HooksConfig, workDir string) core.Hook {
	return core.Hook{
		OnToolStart: func(ctx context.Context, _ *core.RunContext, _ string, toolName string, argsJSON string) {
			for _, h := range cfg.PreToolUse {
				if matchesHook(h, toolName) {
					runShellHook(ctx, h, workDir, toolName, argsJSON)
				}
			}
		},
		OnToolEnd: func(ctx context.Context, _ *core.RunContext, _ string, toolName string, result string, _ error) {
			for _, h := range cfg.PostToolUse {
				if matchesHook(h, toolName) {
					runShellHook(ctx, h, workDir, toolName, result)
				}
			}
		},
	}
}

func matchesHook(h ShellHook, toolName string) bool {
	if len(h.Tools) == 0 {
		return true
	}
	for _, t := range h.Tools {
		if strings.EqualFold(t, toolName) {
			return true
		}
	}
	return false
}

func runShellHook(ctx context.Context, h ShellHook, workDir, toolName, data string) {
	timeout := h.Timeout
	if timeout <= 0 {
		timeout = 10
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", h.Command)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("GOLEM_TOOL=%s", toolName),
		fmt.Sprintf("GOLEM_DATA=%s", truncateForEnv(data, 8192)),
	)
	// Fire and forget — hooks run synchronously but we don't block on output.
	_ = cmd.Run()
}

func truncateForEnv(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
