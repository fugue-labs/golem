package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

type ViewParams struct {
	FilePath string `json:"file_path" jsonschema:"description=Path to the file to read"`
	Offset   int    `json:"offset,omitempty" jsonschema:"description=Line number to start from (1-based)"`
	Limit    int    `json:"limit,omitempty" jsonschema:"description=Number of lines to read (default 2000)"`
}

const (
	defaultReadLimit = 2000
	maxLineLength    = 2000
)

func ViewTool(workingDir string) core.Tool {
	return core.FuncTool[ViewParams](
		"view",
		"Read a file's contents. Returns the file with line numbers. "+
			"Use offset and limit for large files. Supports all text file formats.",
		func(ctx context.Context, params ViewParams) (string, error) {
			if params.FilePath == "" {
				return "", fmt.Errorf("file_path is required")
			}

			path := resolvePath(workingDir, params.FilePath)
			limit := params.Limit
			if limit <= 0 {
				limit = defaultReadLimit
			}

			f, err := os.Open(path)
			if err != nil {
				return "", fmt.Errorf("opening file: %w", err)
			}
			defer f.Close()

			var lines []string
			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			lineNum := 0
			for scanner.Scan() {
				lineNum++
				if params.Offset > 0 && lineNum < params.Offset {
					continue
				}
				if len(lines) >= limit {
					break
				}
				line := scanner.Text()
				if len(line) > maxLineLength {
					line = line[:maxLineLength] + "..."
				}
				lines = append(lines, fmt.Sprintf("%4d│ %s", lineNum, line))
			}

			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("reading file: %w", err)
			}

			if len(lines) == 0 {
				return "(empty file)", nil
			}

			return strings.Join(lines, "\n"), nil
		},
	)
}

func resolvePath(workingDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(workingDir, path)
}
