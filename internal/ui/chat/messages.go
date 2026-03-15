package chat

import (
	"encoding/json"
	"fmt"
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
	Kind      MessageKind
	Content   string
	Streaming bool // true only while assistant text is actively streaming live

	// Tool-specific fields.
	CallID    string // Provider-assigned tool call ID for exact matching.
	ToolName  string
	ToolArgs  string
	RawArgs   string // Full JSON args for rich rendering (diffs, etc.)
	Status    ToolStatus
	StartedAt time.Time     // when the tool call started
	Duration  time.Duration // elapsed time for completed tool calls

	// Render cache — avoids re-rendering unchanged messages every frame.
	cachedRender    string
	cachedWidth     int
	cachedContent   string
	cachedStatus    ToolStatus
	cachedDuration  time.Duration
	cachedStreaming bool
	cachedLines     int // number of lines in cachedRender
}

// Render returns the rendered string for this message, using a cache to avoid
// re-rendering unchanged messages. The cache is invalidated when the message
// content, status, or rendering width changes.
func (msg *Message) Render(sty *styles.Styles, width int, allMessages []*Message) string {
	live := msg.Streaming || (msg.Kind == KindToolCall && msg.Status == ToolRunning)
	if !live && msg.cachedRender != "" && msg.cachedWidth == width &&
		msg.cachedContent == msg.Content && msg.cachedStatus == msg.Status &&
		msg.cachedDuration == msg.Duration && msg.cachedStreaming == msg.Streaming {
		return msg.cachedRender
	}
	rendered := RenderMessage(msg, sty, width, allMessages)
	msg.cachedLines = strings.Count(rendered, "\n") + 1
	if rendered == "" {
		msg.cachedLines = 0
	}
	if live {
		msg.cachedRender = ""
		return rendered
	}
	msg.cachedRender = rendered
	msg.cachedWidth = width
	msg.cachedContent = msg.Content
	msg.cachedStatus = msg.Status
	msg.cachedDuration = msg.Duration
	msg.cachedStreaming = msg.Streaming
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
	return renderRoleBlock(sty.Chat.UserTag.Render(" USER "), renderPlainBlock(sty.Base, msg.Content, width), "  ")
}

func renderAssistantMessage(msg *Message, sty *styles.Styles, width int) string {
	if msg.Content == "" {
		return ""
	}

	body := common.RenderMarkdown(sty, msg.Content, width-4)
	extras := []string{"    " + sty.Chat.AssistantMeta.Render("markdown response")}
	if msg.Streaming {
		extras = append([]string{"    " + sty.Chat.Streaming.Render("LIVE")}, extras...)
	}
	return renderRoleBlock(sty.Chat.AssistantTag.Render(" ASSISTANT "), body, "  ", extras...)
}

func renderToolCall(msg *Message, sty *styles.Styles, width int) string {
	stateLabel, iconStyle, stateStyle := toolStatusParts(msg, sty)
	icon := iconStyle.Render(toolStateIcon(msg.Status))
	state := stateStyle.Render(stateLabel)

	var summary string
	if msg.ToolName == "bash" {
		prompt := sty.Tool.CommandPrompt.Render("$")
		command := msg.ToolArgs
		if command == "" {
			command = msg.ToolName
		}
		available := max(0, width-lipgloss.Width(prompt)-lipgloss.Width(state)-18)
		command = ansi.Truncate(command, available, "...")
		summary = prompt + " " + sty.Tool.CommandText.Render(command)
	} else {
		name := sty.Tool.NameNormal.Render(msg.ToolName)
		summary = name
		if msg.ToolArgs != "" {
			available := max(0, width-lipgloss.Width(name)-lipgloss.Width(state)-14)
			param := ansi.Truncate(msg.ToolArgs, available, "...")
			summary += " " + sty.Tool.ParamMain.Render(param)
		}
	}
	if msg.Duration > 0 {
		summary += " " + sty.Muted.Render(formatDuration(msg.Duration))
	}

	header := fmt.Sprintf("%s %s %s", icon, state, summary)
	bodyLines := []string{}
	if statusLine := toolStatusLine(msg, sty); statusLine != "" {
		bodyLines = append(bodyLines, statusLine)
	}
	if msg.Content != "" {
		if body := renderToolCallResult(msg, sty, width); body != "" {
			bodyLines = append(bodyLines, body)
		}
	}
	body := strings.Join(bodyLines, "\n")
	return renderRoleBlock(sty.Tool.Summary.Render(" TOOL "), header, "  ", body)
}

func toolStatusParts(msg *Message, sty *styles.Styles) (string, lipgloss.Style, lipgloss.Style) {
	switch msg.Status {
	case ToolPending:
		return "queued", sty.Tool.IconPending, sty.Tool.StateWaiting
	case ToolRunning:
		return "running", sty.Tool.IconPending, sty.Tool.StateRunning
	case ToolSuccess:
		return "done", sty.Tool.IconSuccess, sty.Tool.StateSuccess
	case ToolError:
		return "error", sty.Tool.IconError, sty.Tool.StateError
	default:
		return "tool", sty.Tool.IconPending, sty.Tool.StateWaiting
	}
}

func toolStateIcon(status ToolStatus) string {
	switch status {
	case ToolPending, ToolRunning:
		return styles.PendingIcon
	case ToolSuccess:
		return styles.CheckIcon
	case ToolError:
		return styles.ErrorIcon
	default:
		return styles.PendingIcon
	}
}

func toolStatusLine(msg *Message, sty *styles.Styles) string {
	switch msg.Status {
	case ToolPending:
		return "    " + sty.Tool.StateWaiting.Render("Waiting for approval or execution slot.")
	case ToolRunning:
		elapsed := ""
		if !msg.StartedAt.IsZero() {
			elapsed = " · " + sty.Chat.Running.Render(formatDuration(time.Since(msg.StartedAt)) + " elapsed")
		}
		return "    " + sty.Tool.StateRunning.Render("Working… result will appear inline") + elapsed
	case ToolSuccess:
		if msg.Duration > 0 {
			return "    " + sty.Tool.StateSuccess.Render("Completed inline in " + formatDuration(msg.Duration))
		}
	case ToolError:
		if strings.TrimSpace(msg.Content) == "" {
			return "    " + sty.Tool.StateError.Render("Tool failed before producing output.")
		}
	}
	return ""
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
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

func renderToolResult(msg *Message, sty *styles.Styles, width int, allMessages []*Message) string {
	// Results are now merged into their tool call messages and rendered
	// inline by renderToolCall. KindToolResult messages should be empty.
	// Render as plain result only if orphaned content somehow exists.
	if msg.Content == "" {
		return ""
	}
	return renderPlainResult(msg.Content, sty, width)
}

// renderViewResult renders file content with syntax highlighting.
func renderViewResult(content string, toolCall *Message, sty *styles.Styles, width int) string {
	// Extract file path from tool args for language detection.
	fileName := extractJSONField(toolCall.RawArgs, "path")
	if fileName == "" {
		fileName = extractJSONField(toolCall.RawArgs, "file_path")
	}

	// Separate line numbers from content for highlighting.
	rawLines := strings.Split(content, "\n")
	maxLines := 12
	truncated := len(rawLines) > maxLines
	if truncated {
		rawLines = rawLines[:maxLines]
	}

	// Strip line number prefixes, highlight, then re-add.
	// Gollem's view tool formats lines as "%6d\t%s" (6-wide number + tab).
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

	codeBlock := strings.Join(codeLines, "\n")
	highlighted := common.SyntaxHighlight(codeBlock, fileName)
	highlightedLines := strings.Split(highlighted, "\n")

	prefix := sty.Tool.ResultPrefix.Render(styles.ResultPrefix)
	var rendered []string
	for i, hline := range highlightedLines {
		num := ""
		if i < len(lineNums) && lineNums[i] != "" {
			num = sty.Tool.DiffContext.Render(fmt.Sprintf("%4s│ ", lineNums[i]))
		} else {
			num = sty.Tool.DiffContext.Render("     │ ")
		}
		available := max(0, width-10)
		hline = ansi.Truncate(hline, available, "")
		if i == 0 {
			rendered = append(rendered, "  "+prefix+" "+num+hline)
		} else {
			rendered = append(rendered, "    "+num+hline)
		}
	}

	if truncated {
		rendered = append(rendered, "    "+sty.Tool.Truncation.Render(
			fmt.Sprintf("... (%d more lines)", len(strings.Split(content, "\n"))-maxLines),
		))
	}

	return strings.Join(rendered, "\n")
}

func renderBashResult(content string, sty *styles.Styles, width int) string {
	content = strings.TrimRight(content, "\n")
	prefix := sty.Tool.ResultPrefix.Render(styles.ResultPrefix)
	available := max(0, width-8)

	if strings.TrimSpace(content) == "" {
		return "  " + prefix + " " + sty.Tool.Truncation.Render("(No output)")
	}

	lines := strings.Split(content, "\n")
	maxLines := 8
	truncated := len(lines) > maxLines
	if truncated {
		lines = lines[len(lines)-maxLines:]
	}

	var rendered []string
	for i, line := range lines {
		line = ansi.Truncate(line, available, "...")
		if i == 0 {
			rendered = append(rendered, "  "+prefix+" "+sty.Tool.ContentCode.Render(line))
		} else {
			rendered = append(rendered, "    "+sty.Tool.ContentCode.Render(line))
		}
	}
	if truncated {
		rendered = append(rendered, "    "+sty.Tool.Truncation.Render(
			fmt.Sprintf("... (%d lines hidden)", len(strings.Split(content, "\n"))-maxLines),
		))
	}
	return strings.Join(rendered, "\n")
}

// renderGrepResult renders grep output with relative paths and syntax highlighting.
func renderGrepResult(content string, sty *styles.Styles, width int) string {
	rawLines := strings.Split(strings.TrimRight(content, "\n"), "\n")

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

	maxLines := 10
	truncated := len(contentLines) > maxLines
	if truncated {
		contentLines = contentLines[:maxLines]
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

	prefix := sty.Tool.ResultPrefix.Render(styles.ResultPrefix)
	var rendered []string
	first := true
	for _, gl := range parsed {
		if gl.filePath == "" && gl.code == "" {
			continue
		}
		if gl.filePath == "" {
			available := max(0, width-8)
			linePrefix := "    "
			if first {
				linePrefix = "  " + prefix + " "
				first = false
			}
			rendered = append(rendered, linePrefix+sty.Tool.ContentCode.Render(
				ansi.Truncate(gl.code, available, "..."),
			))
			continue
		}

		relPath := strings.TrimPrefix(gl.filePath, commonDir)
		locStr := relPath + ":" + gl.lineNum + ":"
		loc := sty.Tool.DiffContext.Render(locStr)
		highlighted := common.SyntaxHighlight(gl.code, gl.filePath)
		highlighted = strings.TrimRight(highlighted, "\n")

		available := max(0, width-lipgloss.Width(loc)-8)
		highlighted = ansi.Truncate(highlighted, available, "...")

		linePrefix := "    "
		if first {
			linePrefix = "  " + prefix + " "
			first = false
		}
		rendered = append(rendered, linePrefix+loc+" "+highlighted)
	}

	if truncated {
		total := len(strings.Split(strings.TrimRight(content, "\n"), "\n"))
		rendered = append(rendered, "    "+sty.Tool.Truncation.Render(
			fmt.Sprintf("... (%d lines hidden)", total-maxLines),
		))
	}
	if footer != "" {
		rendered = append(rendered, "    "+sty.Tool.Truncation.Render(footer))
	}

	return strings.Join(rendered, "\n")
}

// parseGrepLine parses "path:linenum: code" or "path:linenum:code".
func parseGrepLine(line string) (path, lineNum, code string, ok bool) {
	i := strings.IndexByte(line, ':')
	if i <= 0 {
		return "", "", "", false
	}
	path = line[:i]

	rest := line[i+1:]
	j := strings.IndexByte(rest, ':')
	if j <= 0 {
		return "", "", "", false
	}
	lineNum = rest[:j]

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

	prefix := sty.Tool.ResultPrefix.Render(styles.ResultPrefix)
	available := max(0, width-8)

	var rendered []string
	summary := content
	if summary == "" {
		path := args.Path
		if path == "" {
			path = args.FilePath
		}
		oldCount := len(strings.Split(args.OldString, "\n"))
		newCount := len(strings.Split(args.NewString, "\n"))
		summary = fmt.Sprintf("Edited %s (-%d +%d lines)", path, oldCount, newCount)
	}
	rendered = append(rendered, "  "+prefix+" "+sty.Tool.ContentLine.Render(summary))

	oldLines := strings.Split(args.OldString, "\n")
	maxDiffLines := 4
	for i, line := range oldLines {
		if i >= maxDiffLines {
			rendered = append(rendered, "    "+sty.Tool.Truncation.Render(
				fmt.Sprintf("... (%d more removed)", len(oldLines)-maxDiffLines),
			))
			break
		}
		line = ansi.Truncate(line, available, "...")
		rendered = append(rendered, "    "+sty.Tool.DiffDel.Render("- "+line))
	}

	newLines := strings.Split(args.NewString, "\n")
	for i, line := range newLines {
		if i >= maxDiffLines {
			rendered = append(rendered, "    "+sty.Tool.Truncation.Render(
				fmt.Sprintf("... (%d more added)", len(newLines)-maxDiffLines),
			))
			break
		}
		line = ansi.Truncate(line, available, "...")
		rendered = append(rendered, "    "+sty.Tool.DiffAdd.Render("+ "+line))
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

	prefix := sty.Tool.ResultPrefix.Render(styles.ResultPrefix)
	available := max(0, width-8)

	var rendered []string
	summary := content
	if summary == "" {
		summary = fmt.Sprintf("%d edits applied", len(args.Edits))
	}
	rendered = append(rendered, "  "+prefix+" "+sty.Tool.ContentLine.Render(summary))

	maxEdits := min(3, len(args.Edits))
	for i := range maxEdits {
		e := args.Edits[i]
		rendered = append(rendered, "    "+sty.Tool.ParamKey.Render(e.Path))
		oldLines := strings.Split(e.OldString, "\n")
		for j, line := range oldLines {
			if j >= 2 {
				rendered = append(rendered, "    "+sty.Tool.Truncation.Render(
					fmt.Sprintf("  ... (%d more removed)", len(oldLines)-2)))
				break
			}
			line = ansi.Truncate(line, available, "...")
			rendered = append(rendered, "    "+sty.Tool.DiffDel.Render("- "+line))
		}
		newLines := strings.Split(e.NewString, "\n")
		for j, line := range newLines {
			if j >= 2 {
				rendered = append(rendered, "    "+sty.Tool.Truncation.Render(
					fmt.Sprintf("  ... (%d more added)", len(newLines)-2)))
				break
			}
			line = ansi.Truncate(line, available, "...")
			rendered = append(rendered, "    "+sty.Tool.DiffAdd.Render("+ "+line))
		}
	}
	if len(args.Edits) > maxEdits {
		rendered = append(rendered, "    "+sty.Tool.Truncation.Render(
			fmt.Sprintf("... +%d more edits", len(args.Edits)-maxEdits)))
	}

	return strings.Join(rendered, "\n")
}

// renderGlobResult renders glob results as a compact file list with directory grouping.
func renderGlobResult(content string, sty *styles.Styles, width int) string {
	content = strings.TrimRight(content, "\n")
	if content == "(no matches)" {
		prefix := sty.Tool.ResultPrefix.Render(styles.ResultPrefix)
		return "  " + prefix + " " + sty.Tool.Truncation.Render("(no matches)")
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

	prefix := sty.Tool.ResultPrefix.Render(styles.ResultPrefix)
	available := max(0, width-8)

	summary := fmt.Sprintf("%d files", len(files))
	if footer != "" {
		summary += " (truncated)"
	}
	var rendered []string
	rendered = append(rendered, "  "+prefix+" "+sty.Tool.ContentLine.Render(summary))

	maxLines := 12
	shown := 0
	for _, f := range files {
		if shown >= maxLines {
			break
		}
		f = ansi.Truncate(f, available, "...")
		rendered = append(rendered, "    "+sty.Tool.ContentCode.Render(f))
		shown++
	}
	remaining := len(files) - shown
	if remaining > 0 {
		rendered = append(rendered, "    "+sty.Tool.Truncation.Render(
			fmt.Sprintf("... +%d more files", remaining)))
	}
	if footer != "" {
		rendered = append(rendered, "    "+sty.Tool.Truncation.Render(footer))
	}

	return strings.Join(rendered, "\n")
}

// renderLsResult renders directory listing with clean formatting.
func renderLsResult(content string, sty *styles.Styles, width int) string {
	content = strings.TrimRight(content, "\n")
	lines := strings.Split(content, "\n")
	prefix := sty.Tool.ResultPrefix.Render(styles.ResultPrefix)
	available := max(0, width-8)

	maxLines := 15
	truncated := len(lines) > maxLines+1
	if truncated {
		lines = lines[:maxLines+1]
	}

	var rendered []string
	for i, line := range lines {
		line = ansi.Truncate(line, available, "...")
		if i == 0 {
			rendered = append(rendered, "  "+prefix+" "+sty.Tool.ParamKey.Render(line))
		} else {
			rendered = append(rendered, "    "+sty.Tool.ContentCode.Render(line))
		}
	}
	if truncated {
		rendered = append(rendered, "    "+sty.Tool.Truncation.Render(
			fmt.Sprintf("... (%d more entries)", len(strings.Split(content, "\n"))-maxLines-1)))
	}

	return strings.Join(rendered, "\n")
}

// renderPlainResult renders a generic tool result with truncation.
func renderPlainResult(content string, sty *styles.Styles, width int) string {
	lines := strings.Split(content, "\n")
	maxLines := 6
	truncated := false
	if len(lines) > maxLines {
		truncated = true
		lines = lines[:maxLines]
	}

	prefix := sty.Tool.ResultPrefix.Render(styles.ResultPrefix)
	var rendered []string
	for i, line := range lines {
		available := max(0, width-8)
		line = ansi.Truncate(line, available, "...")
		if i == 0 {
			rendered = append(rendered, "  "+prefix+" "+sty.Tool.ContentCode.Render(line))
		} else {
			rendered = append(rendered, "    "+sty.Tool.ContentCode.Render(line))
		}
	}
	if truncated {
		rendered = append(rendered, "    "+sty.Tool.Truncation.Render(
			fmt.Sprintf("... (%d lines hidden)", len(strings.Split(content, "\n"))-maxLines),
		))
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

	body := sty.Chat.Thinking.Width(width - 4).Render(content)
	label := "thinking"
	if words > 0 {
		label = fmt.Sprintf("thinking · %d words", words)
	}
	if truncated {
		label += " · latest excerpt"
	}
	footer := sty.Chat.ThinkingFooter.Render("    " + label)
	return renderRoleBlock(sty.Chat.ThinkingTag.Render(" THINKING "), body, "  ", footer)
}

func renderSystem(msg *Message, sty *styles.Styles, width int) string {
	kind := classifySystemMessage(msg.Content)
	meta := sty.Chat.Summary.Render(kind)
	body := renderPlainBlock(sty.Chat.SystemText, msg.Content, width)
	return renderRoleBlock(sty.Chat.SystemTag.Render(" SUMMARY "), body, "  ", "    "+meta)
}

func renderError(msg *Message, sty *styles.Styles, width int) string {
	content := msg.Content
	lines := strings.Split(content, "\n")
	available := max(0, width-12)

	if len(lines) <= 1 {
		title := ansi.Truncate(content, available, "...")
		return renderRoleBlock(sty.Chat.ErrorTag.Render(" ERROR "), sty.Chat.ErrorTitle.Render(title), "  ")
	}

	var rendered []string
	firstLine := ansi.Truncate(lines[0], available, "...")
	rendered = append(rendered, sty.Chat.ErrorTitle.Render(firstLine))
	maxDetail := 6
	for i := 1; i < len(lines) && i <= maxDetail; i++ {
		detail := ansi.Truncate(lines[i], available, "...")
		rendered = append(rendered, sty.Chat.ErrorDetails.Render(detail))
	}
	if len(lines) > maxDetail+1 {
		rendered = append(rendered, sty.Chat.ErrorDetails.Render(
			fmt.Sprintf("... (%d more lines)", len(lines)-maxDetail-1)))
	}
	return renderRoleBlock(sty.Chat.ErrorTag.Render(" ERROR "), strings.Join(rendered, "\n"), "  ")
}

func renderRoleBlock(tag, body, indent string, extras ...string) string {
	var rendered []string
	rendered = append(rendered, indent+tag)
	if strings.TrimSpace(body) != "" {
		for _, line := range strings.Split(body, "\n") {
			rendered = append(rendered, indent+"  "+line)
		}
	}
	for _, extra := range extras {
		if strings.TrimSpace(extra) == "" {
			continue
		}
		for _, line := range strings.Split(extra, "\n") {
			rendered = append(rendered, indent+line)
		}
	}
	return strings.Join(rendered, "\n")
}

func renderPlainBlock(style lipgloss.Style, content string, width int) string {
	if width <= 0 {
		width = 20
	}
	return style.Width(width - 4).Render(content)
}

func classifySystemMessage(content string) string {
	lower := strings.ToLower(content)
	switch {
	case strings.Contains(lower, "replay"):
		return "replay"
	case strings.Contains(lower, "budget") || strings.Contains(lower, "context window"):
		return "status"
	case strings.Contains(lower, "tools") || strings.Contains(lower, "↓") || strings.Contains(lower, "↑"):
		return "usage"
	default:
		return "summary"
	}
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
