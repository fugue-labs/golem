package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

type GlobParams struct {
	Pattern string `json:"pattern" jsonschema:"description=Glob pattern to match (e.g. **/*.go or src/**/*.ts)"`
	Path    string `json:"path,omitempty" jsonschema:"description=Directory to search in (defaults to working directory)"`
}

const maxGlobResults = 200

func GlobTool(workingDir string) core.Tool {
	return core.FuncTool[GlobParams](
		"glob",
		"Find files matching a glob pattern. Supports ** for recursive matching. "+
			"Use to discover project structure and find files by name.",
		func(ctx context.Context, params GlobParams) (string, error) {
			if params.Pattern == "" {
				return "", fmt.Errorf("pattern is required")
			}

			dir := workingDir
			if params.Path != "" {
				dir = resolvePath(workingDir, params.Path)
			}

			var matches []string
			err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return nil // skip errors
				}
				if d.IsDir() {
					name := d.Name()
					if name == ".git" || name == "node_modules" || name == ".next" || name == "__pycache__" {
						return filepath.SkipDir
					}
					return nil
				}

				rel, err := filepath.Rel(dir, path)
				if err != nil {
					return nil
				}

				matched, err := filepath.Match(params.Pattern, filepath.Base(rel))
				if err != nil {
					// Try doublestar-style matching by checking suffix.
					matched = matchGlob(params.Pattern, rel)
				}
				if !matched {
					matched = matchGlob(params.Pattern, rel)
				}

				if matched {
					matches = append(matches, rel)
				}
				return nil
			})
			if err != nil {
				return "", fmt.Errorf("walking directory: %w", err)
			}

			if len(matches) == 0 {
				return "(no matches)", nil
			}

			if len(matches) > maxGlobResults {
				result := strings.Join(matches[:maxGlobResults], "\n")
				return fmt.Sprintf("%s\n... (%d more results truncated)", result, len(matches)-maxGlobResults), nil
			}

			return strings.Join(matches, "\n"), nil
		},
	)
}

// matchGlob does simple ** glob matching.
func matchGlob(pattern, path string) bool {
	// Handle **/ prefix.
	if strings.HasPrefix(pattern, "**/") {
		suffix := pattern[3:]
		if matched, _ := filepath.Match(suffix, filepath.Base(path)); matched {
			return true
		}
		// Also try matching against the full relative path segments.
		parts := strings.Split(path, string(filepath.Separator))
		for i := range parts {
			sub := strings.Join(parts[i:], string(filepath.Separator))
			if matched, _ := filepath.Match(suffix, sub); matched {
				return true
			}
		}
	}
	// Direct match.
	matched, _ := filepath.Match(pattern, path)
	return matched
}
