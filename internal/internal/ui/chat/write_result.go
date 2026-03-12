package chat

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fugue-labs/golem/internal/ui/styles"
)

func renderWriteResult(content string, toolCall *Message, sty *styles.Styles, width int) string {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(toolCall.RawArgs), &args); err != nil {
		return renderPlainResult(content, sty, width)
	}

	base := filepath.Base(args.Path)
	lines := strings.Split(args.Content, "\n")
	maxLines := 8
	truncated := len(lines) > maxLines
	if truncated {
		lines = lines[:maxLines]
	}

	rendered := []string{
		"  " + sty.Tool.DiffHeader.Render("+++ " + base),
	}
	for _, line := range lines {
		if len(line) > width-6 {
			line = line[:width-6]
		}
		rendered = append(rendered, "  "+sty.Tool.DiffAdd.Render("+ "+line))
	}
	if truncated {
		rendered = append(rendered, "  "+sty.Tool.Truncation.Render(
			fmt.Sprintf("... (%d more lines)", len(strings.Split(args.Content, "\n"))-maxLines),
		))
	}
	if content != "" {
		rendered = append(rendered, "  "+sty.Tool.ContentLine.Render(content))
	}
	return strings.Join(rendered, "\n")
}
