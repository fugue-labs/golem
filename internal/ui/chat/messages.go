package chat

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fugue-labs/golem/internal/ui/common"
	"github.com/fugue-labs/golem/internal/ui/styles"
)

// MessageKind identifies the type of chat message.
type MessageKind int

const (
	KindUser MessageKind = iota
	KindAssistant
	KindToolCall
	KindToolResult
	KindThinking
	KindError
	KindSystem // system info (e.g., usage summaries)
)

// Message is a displayable chat entry.
type Message struct {
	Kind    MessageKind
	Content string

	// Tool-specific fields.
	CallID    string // Provider-assigned tool call ID for exact matching.
	ToolName  string
	ToolArgs  string
	RawArgs   string // Full JSON args for rich rendering (diffs, etc.)
	Status    ToolStatus
	StartedAt time.Time     // when the tool call started
	Duration  time.Duration // elapsed time for completed tool calls

	// Render cache — avoids re-rendering unchanged messages every frame.
	cachedRender   string
	cachedWidth    int
	cachedContent  string
	cachedStatus   ToolStatus
	cachedDuration time.Duration
	cachedLines    int // number of lines in cachedRender
}

// Render returns the rendered string for this message, using a cache to avoid
// re-rendering unchanged messages. The cache is invalidated when the message
// content, status, or rendering width changes.
func (msg *Message) Render(sty *styles.Styles, width int, allMessages []*Message) string {
	if msg.cachedRender != "" && msg.cachedWidth == width &&
		msg.cachedContent == msg.Content && msg.cachedStatus == msg.Status &&
		msg.cachedDuration == msg.Duration {
		return msg.cachedRender
	}
	rendered := RenderMessage(msg, sty, width, allMessages)
	msg.cachedRender = rendered
	msg.cachedWidth = width
	msg.cachedContent = msg.Content
	msg.cachedStatus = msg.Status
	msg.cachedDuration = msg.Duration
	msg.cachedLines = strings.Count(rendered, "\n") + 1
	if rendered == "" {
		msg.cachedLines = 0
	}
	return rendered
}

// Lines returns the cached line count for this message.
// Must call Render first.
func (msg *Message) Lines() int {
	return msg.cachedLines
}

// ToolStatus tracks tool execution state.
type ToolStatus int

const (
	ToolPending ToolStatus = iota
	ToolRunning
	ToolSuccess
	ToolError
)

// RenderMessage renders a single chat message with appropriate styling.
// allMessages is passed for context (e.g., finding the tool call for a result).
func RenderMessage(msg *Message, sty *styles.Styles, width int, allMessages []*Message) string {
	contentWidth := width - 4
	if contentWidth < 20 {
		contentWidth = 20
	}

	switch msg.Kind {
	case KindUser:
		return renderUserMessage(msg, sty, contentWidth)
	case KindAssistant:
		return renderAssistantMessage(msg, sty, contentWidth)
	case KindToolCall:
		return renderToolCall(msg, sty, contentWidth)
	case KindToolResult:
		return renderToolResult(msg, sty, contentWidth, allMessages)
	case KindThinking:
		return renderThinking(msg, sty, contentWidth)
	case KindError:
		return renderError(msg, sty, contentWidth)
	case KindSystem:
		return renderSystem(msg, sty, contentWidth)
	default:
		return msg.Content
	}
}

func renderUserMessage(msg *Message, sty *styles.Styles, width int) string {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return ""
	}

	header := "  " + sty.Chat.UserBorder.Render(styles.PromptIcon) + " " + sty.Chat.UserLabel.Render("You")
	body := lipgloss.NewStyle().Width(max(16, width-6)).Render(content)
	return header + "\n" + renderGuidedBlock(body, "    ", sty.Chat.UserBorder.Render("│"), width)
}

func renderAssistantMessage(msg *Message, sty *styles.Styles, width int) string {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return ""
	}

	header := "  " + sty.Chat.AssistantBorder.Render(styles.ModelIcon) + " " + sty.Chat.AssistantLabel.Render("Assistant")
	body := common.RenderMarkdown(sty, content, max(20, width-6))
	return header + "\n" + renderGuidedBlock(body, "    ", sty.Chat.AssistantBorder.Render("│"), width)
}

func renderToolCall(msg *Message, sty *styles.Styles, width int) string {
	var icon string
	switch msg.Status {
	case ToolPending, ToolRunning:
		icon = sty.Tool.IconPending.Render(styles.PendingIcon)
	case ToolSuccess:
		icon = sty.Tool.IconSuccess.Render(styles.CheckIcon)
	case ToolError:
		icon = sty.Tool.IconError.Render(styles.ErrorIcon)
	}

	header := fmt.Sprintf("  %s %s", icon, sty.Tool.NameNormal.Render(msg.ToolName))
	if meta := joinNonEmpty(toolStatusLabel(msg.Status), formatToolDuration(msg.Duration)); meta != "" {
		header += " " + renderMetaBadge(sty, meta)
	}

	var sections []string
	if detail := renderToolInvocation(msg, sty, width); detail != "" {
		sections = append(sections, sty.Tool.OutputMeta.Render("call")+"  "+detail)
	}
	if shouldRenderToolResult(msg) {
		body := renderToolCallResult(msg, sty, width)
		if msg.Status == ToolError {
			body = renderToolErrorOutput(msg, sty, width)
		}
		if body != "" {
			if len(sections) > 0 {
				sections = append(sections, "")
			}
			sections = append(sections, body)
		}
	}
	if len(sections) == 0 {
		return header
	}
	return header + "\n" + renderGuidedBlock(strings.Join(sections, "\n"), "    ", sty.Tool.OutputBorder.Render("│"), width)
}

func renderToolInvocation(msg *Message, sty *styles.Styles, width int) string {
	available := max(0, width-8)
	if msg.ToolName == "bash" {
		command := strings.TrimSpace(msg.ToolArgs)
		if command == "" {
			command = msg.ToolName
		}
		command = ansi.Truncate(command, max(0, available-2), "...")
		return sty.Tool.CommandPrompt.Render("$") + " " + sty.Tool.CommandText.Render(command)
	}

	if path := extractPrimaryToolPath(msg); path != "" {
		line := sty.Tool.ParamKey.Render("path") + " " + sty.Tool.ParamMain.Render(ansi.Truncate(path, max(0, available-6), "..."))
		if args := strings.TrimSpace(msg.ToolArgs); args != "" && args != path {
			line += "  " + sty.Tool.OutputMeta.Render(ansi.Truncate(args, max(0, available-lipgloss.Width(stripANSI(line))-2), "..."))
		}
		return line
	}

	if args := strings.TrimSpace(msg.ToolArgs); args != "" {
		return sty.Tool.ParamMain.Render(ansi.Truncate(args, available, "..."))
	}
	return sty.Tool.StateWaiting.Render("waiting for input")
}

func extractPrimaryToolPath(msg *Message) string {
	switch msg.ToolName {
	case "view", "edit", "write", "ls", "glob", "grep":
		if path := extractJSONField(msg.RawArgs, "path"); path != "" {
			return path
		}
		if path := extractJSONField(msg.RawArgs, "file_path"); path != "" {
			return path
		}
	}
	return ""
}

func toolStatusLabel(status ToolStatus) string {
	switch status {
	case ToolPending:
		return "pending"
	case ToolRunning:
		return "running"
	case ToolSuccess:
		return "done"
	case ToolError:
		return "failed"
	default:
		return ""
	}
}

func formatToolDuration(dur time.Duration) string {
	if dur <= 0 {
		return ""
	}
	if dur < time.Second {
		return fmt.Sprintf("%dms", dur.Milliseconds())
	}
	if dur < 10*time.Second {
		return fmt.Sprintf("%.1fs", dur.Seconds())
	}
	return dur.Round(100 * time.Millisecond).String()
}

func renderIndentedLines(content, firstPrefix, nextPrefix string) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		prefix := nextPrefix
		if i == 0 {
			prefix = firstPrefix
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func renderGuidedBlock(content, firstIndent, guide string, width int) string {
	if content == "" {
		return ""
	}
	prefix := firstIndent + guide + " "
	contentWidth := max(1, width-lipgloss.Width(stripANSI(prefix)))
	content = common.ClampANSI(content, contentWidth)
	return renderIndentedLines(content, prefix, prefix)
}

func renderMetaBadge(sty *styles.Styles, label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	return sty.Tool.OutputMeta.Render("[" + label + "]")
}

func renderResultHeader(sty *styles.Styles, label, meta string) string {
	line := sty.Tool.ResultPrefix.Render(styles.ResultPrefix) + " " + sty.Tool.NameNormal.Render(label)
	if meta != "" {
		line += " " + renderMetaBadge(sty, meta)
	}
	return line
}

func formatDisplayPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	cleaned := filepath.Clean(path)
	if cleaned == "." {
		return path
	}
	return cleaned
}

func joinNonEmpty(parts ...string) string {
	filtered := parts[:0]
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			filtered = append(filtered, strings.TrimSpace(part))
		}
	}
	return strings.Join(filtered, " · ")
}

func stripANSI(s string) string {
	return ansi.Strip(s)
}

// renderToolCallResult renders the result body for a completed tool call.
// The result content is stored in msg.Content after the tool finishes.
func renderToolCallResult(msg *Message, sty *styles.Styles, width int) string {
	switch msg.ToolName {
	case "view":
		return renderViewResult(msg.Content, msg, sty, width)
	case "edit":
		return renderEditResult(msg.Content, msg, sty, width)
	case "write":
		return renderWriteResult(msg.Content, msg, sty, width)
	case "bash":
		return renderBashResult(msg.Content, sty, width)
	case "multi_edit":
		return renderMultiEditResult(msg.Content, msg, sty, width)
	case "grep":
		return renderGrepResult(msg.Content, sty, width)
	case "glob":
		return renderGlobResult(msg.Content, sty, width)
	case "ls":
		return renderLsResult(msg.Content, sty, width)
	default:
		return renderPlainResult(msg.Content, sty, width)
	}
}

// findToolCallFor finds the tool call that corresponds to a given tool result.
// When a CallID is present (from the provider's tool_call_id), we match
// exactly. Otherwise, we fall back to rank-based pairing for backward compat.
func findToolCallFor(msg *Message, allMessages []*Message) *Message {
	// Fast path: exact match by call ID.
	if msg.CallID != "" {
		for i := len(allMessages) - 1; i >= 0; i-- {
			m := allMessages[i]
			if m.Kind == KindToolCall && m.CallID == msg.CallID {
				return m
			}
		}
		return nil
	}

	// Fallback: rank-based pairing for when call IDs aren't available.
	msgIdx := -1
	for i := len(allMessages) - 1; i >= 0; i-- {
		if allMessages[i] == msg {
			msgIdx = i
			break
		}
	}
	if msgIdx < 0 {
		return nil
	}

	resultRank := 0
	for i := msgIdx - 1; i >= 0; i-- {
		m := allMessages[i]
		if m.Kind == KindToolCall && m.ToolName == msg.ToolName {
			break
		}
		if m.Kind == KindToolResult && m.ToolName == msg.ToolName {
			resultRank++
		}
	}

	callIdx := 0
	for i := msgIdx - 1; i >= 0; i-- {
		m := allMessages[i]
		if m.Kind == KindToolCall && m.ToolName == msg.ToolName {
			if callIdx == resultRank {
				return m
			}
			callIdx++
		}
	}
	return nil
}

func renderToolErrorOutput(msg *Message, sty *styles.Styles, width int) string {
	content := strings.TrimRight(msg.Content, "\n")
	if strings.TrimSpace(content) == "" {
		return ""
	}

	switch msg.ToolName {
	case "bash":
		return renderBashResult(content, sty, width)
	case "view", "grep", "glob", "ls":
		return renderPlainResult(content, sty, width)
	default:
		return renderPlainResult(content, sty, width)
	}
}

func renderToolResult(msg *Message, sty *styles.Styles, width int, allMessages []*Message) string {
	// Results are now merged into their tool call messages and rendered
	// inline by renderToolCall. KindToolResult messages should be empty.
	// Render as plain result only if orphaned content somehow exists.
	_ = allMessages
	if !shouldRenderToolResult(msg) {
		return ""
	}
	if msg.ToolName != "" {
		return renderToolCallResult(msg, sty, width)
	}
	return renderPlainResult(msg.Content, sty, width)
}

func shouldRenderToolResult(msg *Message) bool {
	if msg == nil {
		return false
	}
	if msg.Status != ToolSuccess && msg.Status != ToolError {
		return false
	}
	if strings.TrimSpace(msg.Content) != "" {
		return true
	}
	if msg.Status == ToolError {
		return false
	}
	switch msg.ToolName {
	case "view", "write", "bash", "grep", "glob", "ls":
		return true
	default:
		return false
	}
}

// renderViewResult renders file content with syntax highlighting.
func renderViewResult(content string, toolCall *Message, sty *styles.Styles, width int) string {
	fileName := extractJSONField(toolCall.RawArgs, "path")
	if fileName == "" {
		fileName = extractJSONField(toolCall.RawArgs, "file_path")
	}
	fileName = formatDisplayPath(fileName)

	content = strings.TrimRight(content, "\n")
	if content == "" {
		return renderResultHeader(sty, "file", fileName) + "\n" + sty.Tool.Truncation.Render("(empty file)")
	}

	rawLines := strings.Split(content, "\n")
	totalLines := len(rawLines)
	maxLines := 12
	truncated := totalLines > maxLines
	if truncated {
		rawLines = rawLines[:maxLines]
	}

	var codeLines []string
	var lineNums []string
	for _, line := range rawLines {
		if idx := strings.IndexByte(line, '\t'); idx != -1 && idx <= 8 {
			num := strings.TrimSpace(line[:idx])
			if num != "" && num[0] >= '0' && num[0] <= '9' {
				lineNums = append(lineNums, num)
				codeLines = append(codeLines, line[idx+1:])
				continue
			}
		}
		lineNums = append(lineNums, "")
		codeLines = append(codeLines, line)
	}

	for i, line := range codeLines {
		if strings.ContainsRune(line, '\t') {
			codeLines[i] = strings.ReplaceAll(line, "\t", "    ")
		}
	}

	highlightedLines := common.SyntaxHighlightLines(strings.Join(codeLines, "\n"), fileName)
	if len(highlightedLines) == 0 {
		highlightedLines = codeLines
	}
	meta := joinNonEmpty(fileName, fmt.Sprintf("%d lines", totalLines))
	if truncated {
		meta = joinNonEmpty(meta, fmt.Sprintf("showing first %d", len(rawLines)))
	}

	rendered := []string{renderResultHeader(sty, "file", meta)}
	available := max(0, width-18)
	highlightedLines = common.ClampANSILines(highlightedLines, available)
	for i, hline := range highlightedLines {
		num := sty.Tool.DiffContext.Render("     │ ")
		if i < len(lineNums) && lineNums[i] != "" {
			num = sty.Tool.DiffContext.Render(fmt.Sprintf("%4s│ ", lineNums[i]))
		}
		rendered = append(rendered, num+hline)
	}
	if truncated {
		rendered = append(rendered, sty.Tool.Truncation.Render(fmt.Sprintf("... (%d more lines)", totalLines-maxLines)))
	}
	return strings.Join(rendered, "\n")
}

func renderBashResult(content string, sty *styles.Styles, width int) string {
	content = strings.TrimRight(content, "\n")
	if strings.TrimSpace(content) == "" {
		return renderResultHeader(sty, "shell output", "no output")
	}

	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	maxLines := 10
	truncated := totalLines > maxLines
	if truncated {
		lines = lines[totalLines-maxLines:]
	}

	meta := fmt.Sprintf("%d lines", totalLines)
	if truncated {
		meta = joinNonEmpty(meta, fmt.Sprintf("showing last %d", len(lines)))
	}
	rendered := []string{renderResultHeader(sty, "shell output", meta)}
	available := max(0, width-14)
	for _, line := range common.ClampANSILines(lines, available) {
		glyph := sty.Tool.ContentLine.Render("│ ")
		trimmed := strings.TrimSpace(stripANSI(line))
		switch {
		case strings.HasPrefix(trimmed, "$") || strings.HasPrefix(trimmed, ">"):
			glyph = sty.Tool.CommandPrompt.Render("$ ")
		case strings.HasPrefix(strings.ToLower(trimmed), "error"), strings.HasPrefix(strings.ToLower(trimmed), "failed"):
			glyph = sty.Tool.IconError.Render("! ")
		case strings.HasPrefix(strings.ToLower(trimmed), "warning"):
			glyph = sty.Tool.IconPending.Render("! ")
		}
		rendered = append(rendered, glyph+line)
	}
	return strings.Join(rendered, "\n")
}

// renderGrepResult renders grep output with relative paths and syntax highlighting.
func renderGrepResult(content string, sty *styles.Styles, width int) string {
	trimmed := strings.TrimRight(content, "\n")
	if trimmed == "" {
		return renderResultHeader(sty, "search results", "no matches")
	}

	rawLines := strings.Split(trimmed, "\n")
	var contentLines []string
	var footer string
	for _, line := range rawLines {
		if strings.HasPrefix(line, "(") && strings.HasSuffix(line, ")") {
			footer = line
		} else if strings.HasPrefix(line, "... (results truncated") {
			footer = line
		} else {
			contentLines = append(contentLines, line)
		}
	}

	type grepLine struct {
		prefix   string
		filePath string
		lineNum  string
		code     string
	}

	var parsed []grepLine
	var paths []string
	for _, line := range contentLines {
		if line == "---" || line == "--" {
			parsed = append(parsed, grepLine{})
			continue
		}
		indicator := ""
		rest := line
		if len(line) > 0 && (line[0] == ' ' || line[0] == '>') {
			indicator = string(line[0])
			rest = line[1:]
		}
		if p, num, code, ok := parseGrepLine(rest); ok {
			parsed = append(parsed, grepLine{prefix: indicator, filePath: p, lineNum: num, code: code})
			paths = append(paths, p)
			continue
		}
		parsed = append(parsed, grepLine{code: line})
	}

	commonDir := longestCommonDirPrefix(paths)
	totalMatches := 0
	for _, line := range parsed {
		if line.filePath != "" {
			totalMatches++
		}
	}
	maxMatches := 12
	shownMatches := 0
	currentFile := ""
	meta := joinNonEmpty(footer, fmt.Sprintf("%d hits", totalMatches))
	rendered := []string{renderResultHeader(sty, "search results", meta)}
	metaWidth := max(0, width-12)

	for _, gl := range parsed {
		if gl.filePath == "" && gl.code == "" {
			continue
		}
		if gl.filePath == "" {
			rendered = append(rendered, sty.Tool.OutputMeta.Render(common.ClampANSI(gl.code, metaWidth)))
			continue
		}
		if shownMatches >= maxMatches {
			break
		}

		relPath := strings.TrimPrefix(gl.filePath, commonDir)
		if relPath == "" {
			relPath = gl.filePath
		}
		if relPath != currentFile {
			currentFile = relPath
			rendered = append(rendered, sty.Tool.NameNormal.Render(common.ClampANSI(relPath, max(0, width-6))))
		}

		marker := sty.Tool.ContentLine.Render("  ")
		if gl.prefix == ">" {
			marker = sty.Tool.IconSuccess.Render("→ ")
		}
		loc := sty.Tool.DiffContext.Render("L" + gl.lineNum)
		code := strings.TrimRight(common.SyntaxHighlight(gl.code, gl.filePath), "\n")
		available := max(0, width-lipgloss.Width(marker)-lipgloss.Width(loc)-14)
		code = common.ClampANSI(code, available)
		rendered = append(rendered, marker+loc+"  "+code)
		shownMatches++
	}

	if totalMatches > shownMatches {
		rendered = append(rendered, sty.Tool.Truncation.Render(fmt.Sprintf("... (%d more hits)", totalMatches-shownMatches)))
	}
	if totalMatches == 0 && footer == "" {
		rendered = append(rendered, sty.Tool.Truncation.Render("(no matches)"))
	}
	return strings.Join(rendered, "\n")
}

// parseGrepLine parses "path:linenum: code" or "path:linenum:code".
func parseGrepLine(line string) (path, lineNum, code string, ok bool) {
	// Find first colon (end of path).
	i := strings.IndexByte(line, ':')
	if i <= 0 {
		return "", "", "", false
	}
	path = line[:i]

	// Find second colon (end of line number).
	rest := line[i+1:]
	j := strings.IndexByte(rest, ':')
	if j <= 0 {
		return "", "", "", false
	}
	lineNum = rest[:j]

	// Verify line number is numeric.
	for _, c := range lineNum {
		if c < '0' || c > '9' {
			return "", "", "", false
		}
	}

	code = rest[j+1:]
	if len(code) > 0 && code[0] == ' ' {
		code = code[1:]
	}
	return path, lineNum, code, true
}

// longestCommonDirPrefix returns the longest shared directory prefix among paths.
func longestCommonDirPrefix(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	prefix := paths[0]
	if !strings.HasSuffix(prefix, "/") {
		idx := strings.LastIndexByte(prefix, '/')
		if idx < 0 {
			prefix = ""
		} else {
			prefix = prefix[:idx+1]
		}
	}

	for _, p := range paths[1:] {
		for prefix != "" && !strings.HasPrefix(p, prefix) {
			trimmed := strings.TrimRight(prefix, "/")
			idx := strings.LastIndexByte(trimmed, '/')
			switch {
			case idx >= 0:
				prefix = trimmed[:idx+1]
			case strings.HasPrefix(prefix, "/"):
				prefix = "/"
			default:
				prefix = ""
			}
		}
	}

	return prefix
}

// renderEditResult renders a diff-style view showing old and new strings.
func renderEditResult(content string, toolCall *Message, sty *styles.Styles, width int) string {
	var args struct {
		Path      string `json:"path"`
		FilePath  string `json:"file_path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal([]byte(toolCall.RawArgs), &args); err != nil {
		return renderPlainResult(content, sty, width)
	}

	path := formatDisplayPath(args.Path)
	if path == "" {
		path = formatDisplayPath(args.FilePath)
	}
	oldLines := strings.Split(args.OldString, "\n")
	newLines := strings.Split(args.NewString, "\n")
	summary := content
	if summary == "" {
		summary = fmt.Sprintf("replaced %d → %d lines", len(oldLines), len(newLines))
	}

	rendered := []string{renderResultHeader(sty, "edit", joinNonEmpty(path, summary))}
	maxDiffLines := 4
	for i, line := range oldLines {
		if i >= maxDiffLines {
			rendered = append(rendered, sty.Tool.Truncation.Render(fmt.Sprintf("... (%d more removed)", len(oldLines)-maxDiffLines)))
			break
		}
		rendered = append(rendered, sty.Tool.DiffDel.Render("- ")+ansi.Truncate(strings.ReplaceAll(line, "\t", "    "), max(0, width-14), "..."))
	}
	for i, line := range newLines {
		if i >= maxDiffLines {
			rendered = append(rendered, sty.Tool.Truncation.Render(fmt.Sprintf("... (%d more added)", len(newLines)-maxDiffLines)))
			break
		}
		rendered = append(rendered, sty.Tool.DiffAdd.Render("+ ")+ansi.Truncate(strings.ReplaceAll(line, "\t", "    "), max(0, width-14), "..."))
	}
	return strings.Join(rendered, "\n")
}

func renderMultiEditResult(content string, toolCall *Message, sty *styles.Styles, width int) string {
	type editEntry struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	var args struct {
		Edits []editEntry `json:"edits"`
	}
	if err := json.Unmarshal([]byte(toolCall.RawArgs), &args); err != nil || len(args.Edits) == 0 {
		return renderPlainResult(content, sty, width)
	}

	summary := content
	if summary == "" {
		summary = fmt.Sprintf("%d edits applied", len(args.Edits))
	}
	rendered := []string{renderResultHeader(sty, "multi edit", summary)}
	maxEdits := min(3, len(args.Edits))
	for i := range maxEdits {
		e := args.Edits[i]
		rendered = append(rendered, sty.Tool.NameNormal.Render(formatDisplayPath(e.Path)))
		oldLines := strings.Split(e.OldString, "\n")
		for j, line := range oldLines {
			if j >= 2 {
				rendered = append(rendered, sty.Tool.Truncation.Render(fmt.Sprintf("... (%d more removed)", len(oldLines)-2)))
				break
			}
			rendered = append(rendered, sty.Tool.DiffDel.Render("- ")+ansi.Truncate(strings.ReplaceAll(line, "\t", "    "), max(0, width-14), "..."))
		}
		newLines := strings.Split(e.NewString, "\n")
		for j, line := range newLines {
			if j >= 2 {
				rendered = append(rendered, sty.Tool.Truncation.Render(fmt.Sprintf("... (%d more added)", len(newLines)-2)))
				break
			}
			rendered = append(rendered, sty.Tool.DiffAdd.Render("+ ")+ansi.Truncate(strings.ReplaceAll(line, "\t", "    "), max(0, width-14), "..."))
		}
	}
	if len(args.Edits) > maxEdits {
		rendered = append(rendered, sty.Tool.Truncation.Render(fmt.Sprintf("... +%d more edits", len(args.Edits)-maxEdits)))
	}
	return strings.Join(rendered, "\n")
}

// renderGlobResult renders glob results as a compact file list with directory grouping.
func renderGlobResult(content string, sty *styles.Styles, width int) string {
	content = strings.TrimRight(content, "\n")
	if content == "" || content == "(no matches)" {
		return renderResultHeader(sty, "files", "no matches")
	}

	lines := strings.Split(content, "\n")
	var files []string
	var footer string
	for _, line := range lines {
		if strings.HasPrefix(line, "... (") {
			footer = line
		} else {
			files = append(files, line)
		}
	}

	rendered := []string{renderResultHeader(sty, "files", joinNonEmpty(fmt.Sprintf("%d found", len(files)), footer))}
	maxLines := 12
	shown := 0
	for _, f := range files {
		if shown >= maxLines {
			break
		}
		rendered = append(rendered, "    "+sty.Tool.ContentCode.Render(ansi.Truncate(formatDisplayPath(f), max(0, width-8), "...")))
		shown++
	}
	if len(files) > shown {
		rendered = append(rendered, "    "+sty.Tool.Truncation.Render(fmt.Sprintf("... +%d more files", len(files)-shown)))
	}
	return strings.Join(rendered, "\n")
}

// renderLsResult renders directory listing with clean formatting.
func renderLsResult(content string, sty *styles.Styles, width int) string {
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return renderResultHeader(sty, "directory", "empty")
	}

	lines := strings.Split(content, "\n")
	root := formatDisplayPath(lines[0])
	entries := lines[1:]
	maxLines := 15
	truncated := len(entries) > maxLines
	if truncated {
		entries = entries[:maxLines]
	}

	rendered := []string{renderResultHeader(sty, "directory", joinNonEmpty(root, fmt.Sprintf("%d entries", len(lines)-1)))}
	for _, line := range entries {
		rendered = append(rendered, "    "+sty.Tool.ContentCode.Render(ansi.Truncate(line, max(0, width-8), "...")))
	}
	if truncated {
		rendered = append(rendered, "    "+sty.Tool.Truncation.Render(fmt.Sprintf("... (%d more entries)", len(lines)-1-maxLines)))
	}
	return strings.Join(rendered, "\n")
}

// renderPlainResult renders a generic tool result with truncation.
func renderPlainResult(content string, sty *styles.Styles, width int) string {
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return renderResultHeader(sty, "result", "empty")
	}

	lines := strings.Split(content, "\n")
	maxLines := 6
	truncated := len(lines) > maxLines
	if truncated {
		lines = lines[:maxLines]
	}

	rendered := []string{renderResultHeader(sty, "result", fmt.Sprintf("%d lines", len(strings.Split(content, "\n"))))}
	for _, line := range lines {
		rendered = append(rendered, "    "+sty.Tool.ContentCode.Render(ansi.Truncate(line, max(0, width-8), "...")))
	}
	if truncated {
		rendered = append(rendered, "    "+sty.Tool.Truncation.Render(fmt.Sprintf("... (%d lines hidden)", len(strings.Split(content, "\n"))-maxLines)))
	}
	return strings.Join(rendered, "\n")
}

func renderThinking(msg *Message, sty *styles.Styles, width int) string {
	content := msg.Content
	words := len(strings.Fields(content))
	lines := strings.Split(content, "\n")
	maxLines := 4
	truncated := false
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
		content = strings.Join(lines, "\n")
		truncated = true
	}

	rendered := sty.Chat.Thinking.Width(width - 4).Render(content)
	label := "Thinking"
	if words > 0 {
		label = fmt.Sprintf("Thinking (%d words)", words)
	}
	if truncated {
		label += " ..."
	}
	footer := sty.Chat.ThinkingFooter.Render("  " + label)
	return rendered + "\n" + footer
}

func renderSystem(msg *Message, sty *styles.Styles, width int) string {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return ""
	}
	line := sty.Subtle.Render(styles.Separator) + " " + sty.Muted.Render(ansi.Truncate(content, max(0, width-8), "..."))
	return "  " + sty.Subtle.Render("system") + " " + line
}

func renderError(msg *Message, sty *styles.Styles, width int) string {
	tag := sty.Chat.ErrorTag.Render("ERROR")
	content := strings.TrimSpace(msg.Content)
	lines := strings.Split(content, "\n")
	available := max(0, width-14)
	if len(lines) == 0 || content == "" {
		return "  " + tag
	}

	rendered := []string{fmt.Sprintf("  %s %s", tag, sty.Chat.ErrorTitle.Render(ansi.Truncate(lines[0], available, "...")))}
	maxDetail := 6
	for i := 1; i < len(lines) && i <= maxDetail; i++ {
		rendered = append(rendered, "    "+sty.Chat.ErrorDetails.Render(styles.BorderThin+" "+ansi.Truncate(lines[i], available, "...")))
	}
	if len(lines) > maxDetail+1 {
		rendered = append(rendered, "    "+sty.Tool.Truncation.Render(fmt.Sprintf("... (%d more lines)", len(lines)-maxDetail-1)))
	}
	return strings.Join(rendered, "\n")
}

// extractJSONField extracts a string field from a JSON object.
func extractJSONField(jsonStr, field string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return ""
	}
	if v, ok := m[field]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}
