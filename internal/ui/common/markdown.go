package common

import (
	"strings"

	"charm.land/glamour/v2"
	"github.com/fugue-labs/golem/internal/ui/styles"
)

// RenderMarkdown renders markdown text using glamour with the app's theme.
func RenderMarkdown(sty *styles.Styles, content string, width int) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(sty.Markdown),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return content
	}
	result, err := r.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimSuffix(result, "\n")
}
