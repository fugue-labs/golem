package tools

import (
	"os"

	"github.com/fugue-labs/gollem/core"
)

// CodingTools returns the standard set of coding agent tools.
func CodingTools(workingDir string) []core.Tool {
	tools := []core.Tool{
		BashTool(workingDir),
		ViewTool(workingDir),
		EditTool(workingDir),
		WriteTool(workingDir),
		GlobTool(workingDir),
		GrepTool(workingDir),
		LsTool(workingDir),
		SessionSearchTool(workingDir),
	}
	tools = append(tools, CodeGraphTools(workingDir)...)
	return tools
}

func writableMode(path string) os.FileMode {
	info, err := os.Stat(path)
	if err == nil {
		return info.Mode().Perm()
	}
	return 0o600
}
