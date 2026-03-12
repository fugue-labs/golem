package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

type WriteParams struct {
	FilePath string `json:"file_path" jsonschema:"description=Path for the new file"`
	Content  string `json:"content" jsonschema:"description=The file contents to write"`
}

func WriteTool(workingDir string) core.Tool {
	return core.FuncTool[WriteParams](
		"write",
		"Create a new file or completely overwrite an existing file. "+
			"For targeted edits to existing files, use the edit tool instead.",
		func(ctx context.Context, params WriteParams) (string, error) {
			if params.FilePath == "" {
				return "", errors.New("file_path is required")
			}

			path := resolvePath(workingDir, params.FilePath)

			// Ensure parent directories exist.
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", fmt.Errorf("creating directories: %w", err)
			}

			if err := os.WriteFile(path, []byte(params.Content), writableMode(path)); err != nil {
				return "", fmt.Errorf("writing file: %w", err)
			}

			lines := strings.Count(params.Content, "\n") + 1
			return fmt.Sprintf("Wrote %s (%d lines)", params.FilePath, lines), nil
		},
		core.WithToolSequential(true),
	)
}
