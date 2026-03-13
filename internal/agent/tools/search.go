package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/fugue-labs/golem/internal/search"
	"github.com/fugue-labs/gollem/core"
)

type SessionSearchParams struct {
	Query      string `json:"query" jsonschema:"description=Search query — keywords or natural language question about past sessions"`
	ProjectDir string `json:"project_dir,omitempty" jsonschema:"description=Limit search to sessions from this working directory. Leave empty to search all projects."`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"description=Maximum number of results to return (default 10)"`
}

// SessionSearchTool returns a tool that searches across saved golem sessions.
func SessionSearchTool(workingDir string) core.Tool {
	return core.FuncTool[SessionSearchParams](
		"session_search",
		"Search across all saved golem sessions using full-text search. "+
			"Use this to recall prior solutions, find how something was fixed before, "+
			"or locate past conversations about a topic. "+
			"Returns matching sessions ranked by relevance with text snippets.",
		func(ctx context.Context, params SessionSearchParams) (string, error) {
			if params.Query == "" {
				return "", errors.New("query is required")
			}
			maxResults := params.MaxResults
			if maxResults <= 0 {
				maxResults = 10
			}

			results, err := search.SearchSessions(params.Query, params.ProjectDir, maxResults)
			if err != nil {
				return "", fmt.Errorf("search failed: %w", err)
			}

			if len(results) == 0 {
				return "No matching sessions found.", nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "Found %d matching session(s):\n\n", len(results))
			for i, r := range results {
				fmt.Fprintf(&b, "--- Result %d (score: %.2f) ---\n", i+1, r.Score)
				fmt.Fprintf(&b, "Time: %s\n", r.Timestamp.Format("2006-01-02 15:04"))
				if r.ProjectDir != "" {
					fmt.Fprintf(&b, "Project: %s\n", r.ProjectDir)
				}
				if r.Prompt != "" {
					prompt := r.Prompt
					if len(prompt) > 200 {
						prompt = prompt[:200] + "…"
					}
					fmt.Fprintf(&b, "Prompt: %s\n", prompt)
				}
				fmt.Fprintf(&b, "Snippet: %s\n\n", r.Snippet)
			}
			return b.String(), nil
		},
	)
}
