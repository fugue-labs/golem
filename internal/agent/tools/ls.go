package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

type LsParams struct {
	Path string `json:"path,omitempty" jsonschema:"description=Directory to list (defaults to working directory)"`
}

func LsTool(workingDir string) core.Tool {
	return core.FuncTool[LsParams](
		"ls",
		"List directory contents. Shows files and directories with sizes.",
		func(ctx context.Context, params LsParams) (string, error) {
			dir := workingDir
			if params.Path != "" {
				dir = resolvePath(workingDir, params.Path)
			}

			entries, err := os.ReadDir(dir)
			if err != nil {
				return "", fmt.Errorf("reading directory: %w", err)
			}

			var lines []string
			for _, e := range entries {
				name := e.Name()
				if name == ".git" {
					continue
				}

				info, err := e.Info()
				if err != nil {
					continue
				}

				if e.IsDir() {
					lines = append(lines, fmt.Sprintf("  %s/", name))
				} else {
					size := humanSize(info.Size())
					lines = append(lines, fmt.Sprintf("  %-40s %s", name, size))
				}
			}

			rel, _ := filepath.Rel(workingDir, dir)
			if rel == "." {
				rel = dir
			}

			if len(lines) == 0 {
				return rel + "/ (empty)", nil
			}

			return fmt.Sprintf("%s/\n%s", rel, strings.Join(lines, "\n")), nil
		},
	)
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
