package chat

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/ansi"
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

	prefix := sty.Tool.ResultPrefix.Render(styles.ResultPrefix)
	available := max(0, width-8)
	codeLines := strings.Split(args.Content, "\n")
	maxLines := 8
	truncated := len(codeLines) > maxLines
	if truncated {
		codeLines = codeLines[:maxLines]
	}

	var rendered []string
	// Summary on first line with ⎿ prefix.
	summary := content
	if summary == "" {
		summary = fmt.Sprintf("Created %s (%d lines)", filepath.Base(args.Path), len(strings.Split(args.Content, "\n")))
	}
	rendered = append(rendered, "  "+prefix+" "+sty.Tool.ContentLine.Render(summary))

	for _, line := range codeLines {
		line = ansi.Truncate(line, available, "...")
		rendered = append(rendered, "    "+sty.Tool.DiffAdd.Render("+ "+line))
	}
	if truncated {
		rendered = append(rendered, "    "+sty.Tool.Truncation.Render(
			fmt.Sprintf("... (%d more lines)", len(strings.Split(args.Content, "\n"))-maxLines),
		))
	}
	return strings.Join(rendered, "\n")
}
