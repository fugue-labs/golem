package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/fugue-labs/golem/internal/ui/common"
	"github.com/fugue-labs/golem/internal/ui/styles"
)

func renderWriteResult(content string, toolCall *Message, sty *styles.Styles, width int) string {
	var args struct {
		Path     string `json:"path"`
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal([]byte(toolCall.RawArgs), &args); err != nil {
		return renderPlainResult(content, sty, width)
	}

	path := formatDisplayPath(args.Path)
	if path == "" {
		path = formatDisplayPath(args.FilePath)
	}
	codeLines := strings.Split(strings.TrimRight(args.Content, "\n"), "\n")
	if len(codeLines) == 1 && codeLines[0] == "" {
		codeLines = nil
	}

	summary := content
	if summary == "" {
		summary = fmt.Sprintf("created %d lines", len(codeLines))
	}

	rendered := []string{renderResultHeader(sty, "write", joinNonEmpty(path, summary))}
	if len(codeLines) == 0 {
		rendered = append(rendered, sty.Tool.Truncation.Render("(empty file)"))
		return strings.Join(rendered, "\n")
	}

	preview := codeLines
	maxLines := 8
	truncated := len(preview) > maxLines
	if truncated {
		preview = preview[:maxLines]
	}

	highlighted := strings.Split(common.SyntaxHighlight(strings.Join(preview, "\n"), path), "\n")
	for i, line := range highlighted {
		prefix := sty.Tool.DiffAdd.Render("+ ")
		if i >= len(preview) {
			prefix = sty.Tool.ContentLine.Render("  ")
		}
		rendered = append(rendered, prefix+ansi.Truncate(line, max(0, width-14), "..."))
	}
	if truncated {
		rendered = append(rendered, sty.Tool.Truncation.Render(fmt.Sprintf("... (%d more lines)", len(codeLines)-maxLines)))
	}
	return strings.Join(rendered, "\n")
}
