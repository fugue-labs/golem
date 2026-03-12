package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
)

type BashParams struct {
	Command     string `json:"command" jsonschema:"description=The shell command to execute"`
	Description string `json:"description,omitempty" jsonschema:"description=Brief description of what the command does"`
	WorkingDir  string `json:"working_dir,omitempty" jsonschema:"description=Working directory (defaults to project root)"`
}

const (
	bashTimeout     = 2 * time.Minute
	maxOutputLength = 30000
)

func BashTool(workingDir string) core.Tool {
	return core.FuncTool[BashParams](
		"bash",
		"Execute a shell command. Use for running tests, builds, git operations, and other terminal commands. "+
			"Commands run in a non-interactive shell. Avoid interactive commands (vim, less, etc.).",
		func(ctx context.Context, rc *core.RunContext, params BashParams) (string, error) {
			if params.Command == "" {
				return "", errors.New("command is required")
			}

			dir := workingDir
			if params.WorkingDir != "" {
				dir = params.WorkingDir
			}

			ctx, cancel := context.WithTimeout(ctx, bashTimeout)
			defer cancel()

			cmd := exec.CommandContext(ctx, "bash", "-c", params.Command)
			cmd.Dir = dir

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()

			output := stdout.String()
			if stderr.Len() > 0 {
				if output != "" {
					output += "\n"
				}
				output += stderr.String()
			}

			if output == "" {
				output = "(no output)"
			}

			// Truncate very long output.
			if len(output) > maxOutputLength {
				output = output[:maxOutputLength] + fmt.Sprintf("\n... (truncated, %d bytes total)", len(output))
			}

			if err != nil {
				exitCode := -1
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					exitCode = exitErr.ExitCode()
				}
				return fmt.Sprintf("exit code: %d\n%s", exitCode, strings.TrimSpace(output)), nil
			}

			return strings.TrimSpace(output), nil
		},
		core.WithToolSequential(true),
	)
}
