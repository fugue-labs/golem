package tools

import (
	"github.com/fugue-labs/gollem/core"
)

// CodingTools returns the standard set of coding agent tools.
func CodingTools(workingDir string) []core.Tool {
	return []core.Tool{
		BashTool(workingDir),
		ViewTool(workingDir),
		EditTool(workingDir),
		WriteTool(workingDir),
		GlobTool(workingDir),
		GrepTool(workingDir),
		LsTool(workingDir),
	}
}
