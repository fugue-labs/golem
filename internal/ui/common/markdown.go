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
	switch {
	case strings.Contains(content, markdownCodeFence):
		rendererWidth = max(width+4, width)
	case hasMarkdownTable(content):
		rendererWidth = max(width+8, width)
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
	return normalizeRenderedMarkdown(result)
}

func hasMarkdownTable(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Count(trimmed, "|") < 2 {
			continue
		}
		if strings.Contains(trimmed, "---") || strings.HasPrefix(trimmed, "|") {
			return true
		}
	}
	return false
}

func normalizeRenderedMarkdown(result string) string {
	result = strings.ReplaceAll(result, "\r\n", "\n")
	result = strings.TrimRight(result, "\n")
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return result
}
