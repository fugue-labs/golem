package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

type EditParams struct {
	FilePath   string `json:"file_path" jsonschema:"description=Path to the file to modify"`
	OldString  string `json:"old_string" jsonschema:"description=The exact text to find and replace"`
	NewString  string `json:"new_string" jsonschema:"description=The replacement text"`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema:"description=Replace all occurrences (default false)"`
}

func EditTool(workingDir string) core.Tool {
	return core.FuncTool[EditParams](
		"edit",
		"Edit a file by replacing exact text. The old_string must match exactly (including whitespace). "+
			"For creating new files, use the write tool instead.",
		func(ctx context.Context, params EditParams) (string, error) {
			if params.FilePath == "" {
				return "", errors.New("file_path is required")
			}
			if params.OldString == "" {
				return "", errors.New("old_string is required")
			}

			path := resolvePath(workingDir, params.FilePath)

			content, err := os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("reading file: %w", err)
			}

			text := string(content)

			count := strings.Count(text, params.OldString)
			if count == 0 {
				return "old_string not found in file. Make sure it matches exactly, including whitespace and line breaks.", nil
			}
			if count > 1 && !params.ReplaceAll {
				return fmt.Sprintf("old_string appears %d times. Provide more context for a unique match, or set replace_all to true.", count), nil
			}

			var newText string
			if params.ReplaceAll {
				newText = strings.ReplaceAll(text, params.OldString, params.NewString)
			} else {
				newText = strings.Replace(text, params.OldString, params.NewString, 1)
			}

			if err := os.WriteFile(path, []byte(newText), writableMode(path)); err != nil {
				return "", fmt.Errorf("writing file: %w", err)
			}

			oldLines := strings.Count(params.OldString, "\n") + 1
			newLines := strings.Count(params.NewString, "\n") + 1
			return fmt.Sprintf("Edited %s: %d lines removed, %d lines added", params.FilePath, oldLines, newLines), nil
		},
		core.WithToolSequential(true),
	)
}
