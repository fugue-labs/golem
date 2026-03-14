package common

import (
	"strings"

	"charm.land/glamour/v2"
	"github.com/fugue-labs/golem/internal/ui/styles"
)

const markdownCodeFence = "```"

// RenderMarkdown renders markdown text using glamour with the app's theme.
func RenderMarkdown(sty *styles.Styles, content string, width int) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if width < 20 {
		width = 20
	}

	rendererWidth := width
	if strings.Contains(content, markdownCodeFence) {
		rendererWidth = max(width+2, width)
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(sty.Markdown),
		glamour.WithWordWrap(rendererWidth),
		glamour.WithTableWrap(false),
		glamour.WithEmoji(),
		glamour.WithPreservedNewLines(),
	)
	if err != nil {
		return content
	}
	result, err := r.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimSuffix(strings.TrimRight(result, "\n"), "\n")
}
