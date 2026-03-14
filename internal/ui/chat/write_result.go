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

	trimmedContent := strings.TrimRight(args.Content, "\n")
	var codeLines []string
	if trimmedContent != "" {
		codeLines = strings.Split(trimmedContent, "\n")
	}

	summary := strings.TrimSpace(content)
	if summary == "" {
		if len(codeLines) == 0 {
			summary = "wrote empty file"
		} else {
			summary = fmt.Sprintf("wrote %d lines", len(codeLines))
		}
	}

	rendered := []string{renderResultHeader(sty, "write", joinNonEmpty(path, summary))}
	if len(codeLines) == 0 {
		rendered = append(rendered, sty.Tool.Truncation.Render("(empty file)"))
		return strings.Join(rendered, "\n")
	}

	preview := codeLines
	maxLines := 10
	truncated := len(preview) > maxLines
	if truncated {
		preview = preview[:maxLines]
	}

	highlighted := common.SyntaxHighlightLines(strings.Join(preview, "\n"), path)
	if len(highlighted) == 0 {
		highlighted = preview
	}
	for _, line := range highlighted {
		rendered = append(rendered, sty.Tool.DiffAdd.Render("+ ")+ansi.Truncate(line, max(0, width-14), "..."))
	}
	if truncated {
		rendered = append(rendered, sty.Tool.Truncation.Render(fmt.Sprintf("... (%d more lines)", len(codeLines)-maxLines)))
	}
	return strings.Join(rendered, "\n")
}
