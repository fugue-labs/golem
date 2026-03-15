package common

import (
	"strings"

	"charm.land/glamour/v2"
	"github.com/fugue-labs/golem/internal/ui/styles"
)

// RenderMarkdown renders markdown text using glamour with the app's theme.
func RenderMarkdown(sty *styles.Styles, content string, width int) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if strings.TrimSpace(content) == "" {
		return ""
	}
	if width <= 0 {
		width = 80
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(sty.Markdown),
		glamour.WithWordWrap(width),
		glamour.WithTableWrap(true),
		glamour.WithEmoji(),
	)
	if err != nil {
		return content
	}
	result, err := r.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimRight(result, "\n")
}
