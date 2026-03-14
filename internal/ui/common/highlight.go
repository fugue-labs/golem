package common

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
)

// SyntaxHighlight applies syntax highlighting to source code.
// fileName is used to detect the language; falls back to content analysis.
func SyntaxHighlight(source, fileName string) string {
	source = normalizeHighlightedSource(source)
	if strings.TrimSpace(source) == "" {
		return source
	}

	fileName = strings.TrimSpace(fileName)
	l := lexers.Match(fileName)
	if l == nil && fileName != "" {
		l = lexers.Match(filepath.Base(fileName))
	}
	if l == nil {
		l = lexers.Analyse(source)
	}
	if l == nil {
		return source
	}
	l = chroma.Coalesce(l)

	f := formatters.Get("terminal256")
	if f == nil {
		f = formatters.Get("terminal16m")
	}
	if f == nil {
		f = formatters.Fallback
	}

	style := chromastyles.Get("dracula")
	if style == nil {
		style = chromastyles.Fallback
	}

	it, err := l.Tokenise(nil, source)
	if err != nil {
		return source
	}

	var buf bytes.Buffer
	if err := f.Format(&buf, style, it); err != nil {
		return source
	}

	return strings.TrimRight(buf.String(), "\n")
}

// SyntaxHighlightLines applies syntax highlighting while preserving the
// original logical line count so callers can render stable gutters.
func SyntaxHighlightLines(source, fileName string) []string {
	source = normalizeHighlightedSource(source)
	if source == "" {
		return nil
	}

	rawLines := strings.Split(source, "\n")
	highlighted := SyntaxHighlight(source, fileName)
	if highlighted == "" {
		return rawLines
	}

	lines := strings.Split(highlighted, "\n")
	for len(lines) < len(rawLines) {
		lines = append(lines, "")
	}
	if len(lines) > len(rawLines) {
		lines = lines[:len(rawLines)]
	}
	return lines
}

func normalizeHighlightedSource(source string) string {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\t", "    ")
	return strings.TrimRight(source, "\n")
}
