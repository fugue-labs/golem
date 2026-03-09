package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

type GrepParams struct {
	Pattern string `json:"pattern" jsonschema:"description=Regex pattern to search for"`
	Path    string `json:"path,omitempty" jsonschema:"description=File or directory to search (defaults to working directory)"`
	Glob    string `json:"glob,omitempty" jsonschema:"description=File glob filter (e.g. *.go or *.ts)"`
}

const maxGrepResults = 100

func GrepTool(workingDir string) core.Tool {
	return core.FuncTool[GrepParams](
		"grep",
		"Search file contents using regex patterns. Returns matching lines with file paths and line numbers. "+
			"Use glob parameter to filter by file type.",
		func(ctx context.Context, params GrepParams) (string, error) {
			if params.Pattern == "" {
				return "", errors.New("pattern is required")
			}

			re, err := regexp.Compile(params.Pattern)
			if err != nil {
				return "", fmt.Errorf("invalid regex: %w", err)
			}

			dir := workingDir
			if params.Path != "" {
				dir = resolvePath(workingDir, params.Path)
			}

			var results []string
			err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					if d != nil && d.IsDir() {
						name := d.Name()
						if name == ".git" || name == "node_modules" || name == ".next" || name == "__pycache__" {
							return filepath.SkipDir
						}
					}
					return nil
				}

				if len(results) >= maxGrepResults {
					return filepath.SkipAll
				}

				// Apply glob filter.
				if params.Glob != "" {
					matched, _ := filepath.Match(params.Glob, filepath.Base(path))
					if !matched {
						return nil
					}
				}

				rel, _ := filepath.Rel(dir, path)
				searchFile(re, path, rel, &results)
				return nil
			})
			if err != nil {
				return "", fmt.Errorf("searching: %w", err)
			}

			if len(results) == 0 {
				return "(no matches)", nil
			}

			output := strings.Join(results, "\n")
			if len(results) >= maxGrepResults {
				output += fmt.Sprintf("\n... (results truncated at %d matches)", maxGrepResults)
			}
			return output, nil
		},
	)
}

func searchFile(re *regexp.Regexp, path, rel string, results *[]string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			if len(line) > 200 {
				line = line[:200] + "..."
			}
			*results = append(*results, fmt.Sprintf("%s:%d: %s", rel, lineNum, line))
			if len(*results) >= maxGrepResults {
				return
			}
		}
	}
}
