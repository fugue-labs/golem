package chat

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

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
)

// Message is a displayable chat entry.
type Message struct {
	Kind    MessageKind
	Content string

	// Tool-specific fields.
	ToolName string
	ToolArgs string
	RawArgs  string // Full JSON args for rich rendering (diffs, etc.)
	Status   ToolStatus

	// Render cache — avoids re-rendering unchanged messages every frame.
	cachedRender  string
	cachedWidth   int
	cachedContent string
	cachedStatus  ToolStatus
	cachedLines   int // number of lines in cachedRender
}

// Render returns the rendered string for this message, using a cache to avoid
// re-rendering unchanged messages. The cache is invalidated when the message
// content, status, or rendering width changes.
func (msg *Message) Render(sty *styles.Styles, width int, allMessages []*Message) string {
	if msg.cachedRender != "" && msg.cachedWidth == width &&
		msg.cachedContent == msg.Content && msg.cachedStatus == msg.Status {
		return msg.cachedRender
	}
	rendered := RenderMessage(msg, sty, width, allMessages)
	msg.cachedRender = rendered
	msg.cachedWidth = width
	msg.cachedContent = msg.Content
	msg.cachedStatus = msg.Status
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
	default:
		return msg.Content
	}
}

func renderUserMessage(msg *Message, sty *styles.Styles, width int) string {
	label := sty.Chat.UserLabel.Render("You")
	border := sty.Chat.UserBorder.Render(styles.BorderThick)
	content := sty.Base.Width(width - 3).Render(msg.Content)

	var lines []string
	lines = append(lines, border+" "+label)
	for _, line := range strings.Split(content, "\n") {
		lines = append(lines, border+" "+line)
	}
	return strings.Join(lines, "\n")
}

func renderAssistantMessage(msg *Message, sty *styles.Styles, width int) string {
	if msg.Content == "" {
		return ""
	}

	rendered := common.RenderMarkdown(sty, msg.Content, width-3)
	border := sty.Chat.AssistantBorder.Render(styles.BorderThick)

	var lines []string
	for _, line := range strings.Split(rendered, "\n") {
		lines = append(lines, border+" "+line)
	}
	return strings.Join(lines, "\n")
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

	if msg.ToolName == "bash" {
		prompt := sty.Tool.CommandPrompt.Render("$")
		command := msg.ToolArgs
		if command == "" {
			command = msg.ToolName
		}
		available := max(0, width-lipgloss.Width(icon)-lipgloss.Width(prompt)-6)
		command = ansi.Truncate(command, available, "...")
		return fmt.Sprintf("  %s %s %s", icon, prompt, sty.Tool.CommandText.Render(command))
	}

	name := sty.Tool.NameNormal.Render(msg.ToolName)
	header := fmt.Sprintf("  %s %s", icon, name)

	// Show primary parameter.
	if msg.ToolArgs != "" {
		available := max(0, width-lipgloss.Width(header)-2)
		param := ansi.Truncate(msg.ToolArgs, available, "...")
		header += " " + sty.Tool.ParamMain.Render(param)
	}

	return header
}

// findToolCallFor finds the tool call that corresponds to a given tool result.
// When the model makes parallel calls of the same tool (e.g., 3 "view" calls),
// results arrive in arbitrary order. We pair the Nth result of a given tool
// name with the Nth-from-last unclaimed call of that name, counting from the
// result's position backward. This ensures each result maps to a unique call.
func findToolCallFor(msg *Message, allMessages []*Message) *Message {
	// Find msg's index.
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

	// Count how many results with the same tool name appear between this
	// result and the preceding tool calls (i.e., this result's "rank" among
	// its siblings). Then match it to the call at the same rank.
	resultRank := 0
	for i := msgIdx - 1; i >= 0; i-- {
		m := allMessages[i]
		if m.Kind == KindToolCall && m.ToolName == msg.ToolName {
			break // Reached the tool call region — stop counting results.
		}
		if m.Kind == KindToolResult && m.ToolName == msg.ToolName {
			resultRank++
		}
	}

	// Now find the (resultRank)th tool call walking backward from before msg.
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
	if msg.Content == "" {
		return ""
	}

	// Try specialized rendering based on tool name.
	toolCall := findToolCallFor(msg, allMessages)
	if toolCall != nil {
		switch toolCall.ToolName {
		case "view":
			return renderViewResult(msg, toolCall, sty, width)
		case "edit":
			return renderEditResult(msg, toolCall, sty, width)
		case "write":
			return renderWriteResult(msg, toolCall, sty, width)
		case "bash":
			return renderBashResult(msg, toolCall, sty, width)
		}
	}

	return renderPlainResult(msg.Content, sty, width)
}

// renderViewResult renders file content with syntax highlighting.
func renderViewResult(msg *Message, toolCall *Message, sty *styles.Styles, width int) string {
	// Extract file path from tool args for language detection.
	fileName := extractJSONField(toolCall.RawArgs, "path")
	if fileName == "" {
		fileName = extractJSONField(toolCall.RawArgs, "file_path")
	}

	// Separate line numbers from content for highlighting.
	rawLines := strings.Split(msg.Content, "\n")
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

	// Expand tabs to spaces before highlighting. Terminal tab stops have
	// variable width, which breaks ansi.StringWidth / ansi.Truncate (they
	// treat tabs as zero-width). Four spaces matches Go convention and is
	// close enough for other languages.
	for i, line := range codeLines {
		if strings.ContainsRune(line, '\t') {
			codeLines[i] = strings.ReplaceAll(line, "\t", "    ")
		}
	}

	// Highlight all code as a block for consistent tokenization.
	codeBlock := strings.Join(codeLines, "\n")
	highlighted := common.SyntaxHighlight(codeBlock, fileName)
	highlightedLines := strings.Split(highlighted, "\n")

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
		rendered = append(rendered, "  "+num+hline)
	}

	if truncated {
		rendered = append(rendered, "  "+sty.Tool.Truncation.Render(
			fmt.Sprintf("... (%d more lines)", len(strings.Split(msg.Content, "\n"))-maxLines),
		))
	}

	return strings.Join(rendered, "\n")
}

func renderBashResult(msg *Message, toolCall *Message, sty *styles.Styles, width int) string {
	content := strings.TrimRight(msg.Content, "\n")
	prefix := sty.Tool.ResultPrefix.Render("⎿")
	available := max(0, width-8)

	// Empty or whitespace-only output.
	if strings.TrimSpace(content) == "" {
		return "  " + prefix + " " + sty.Tool.Truncation.Render("(No output)")
	}

	lines := strings.Split(content, "\n")
	maxLines := 6
	truncated := len(lines) > maxLines
	if truncated {
		lines = lines[len(lines)-maxLines:] // Show tail, not head
	}

	var rendered []string
	for _, line := range lines {
		line = ansi.Truncate(line, available, "...")
		rendered = append(rendered, "  "+prefix+" "+sty.Tool.ContentCode.Render(line))
	}
	if truncated {
		rendered = append(rendered, "  "+prefix+" "+sty.Tool.Truncation.Render(
			fmt.Sprintf("... (%d lines hidden)", len(strings.Split(content, "\n"))-maxLines),
		))
	}
	return strings.Join(rendered, "\n")
}

// renderEditResult renders a diff-style view showing old and new strings.
func renderEditResult(msg *Message, toolCall *Message, sty *styles.Styles, width int) string {
	var args struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal([]byte(toolCall.RawArgs), &args); err != nil {
		return renderPlainResult(msg.Content, sty, width)
	}

	var rendered []string

	// File header.
	base := filepath.Base(args.Path)
	rendered = append(rendered, "  "+sty.Tool.DiffHeader.Render("--- "+base))
	rendered = append(rendered, "  "+sty.Tool.DiffHeader.Render("+++ "+base))

	// Removed lines.
	oldLines := strings.Split(args.OldString, "\n")
	maxDiffLines := 8
	for i, line := range oldLines {
		if i >= maxDiffLines {
			rendered = append(rendered, "  "+sty.Tool.Truncation.Render(
				fmt.Sprintf("  ... (%d more removed)", len(oldLines)-maxDiffLines),
			))
			break
		}
		if len(line) > width-6 {
			line = line[:width-6]
		}
		rendered = append(rendered, "  "+sty.Tool.DiffDel.Render("- "+line))
	}

	// Added lines.
	newLines := strings.Split(args.NewString, "\n")
	for i, line := range newLines {
		if i >= maxDiffLines {
			rendered = append(rendered, "  "+sty.Tool.Truncation.Render(
				fmt.Sprintf("  ... (%d more added)", len(newLines)-maxDiffLines),
			))
			break
		}
		if len(line) > width-6 {
			line = line[:width-6]
		}
		rendered = append(rendered, "  "+sty.Tool.DiffAdd.Render("+ "+line))
	}

	// Summary line from the tool result.
	rendered = append(rendered, "  "+sty.Tool.ContentLine.Render(msg.Content))

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

	prefix := sty.Tool.ResultPrefix.Render("⎿")
	var rendered []string
	for _, line := range lines {
		available := max(0, width-8)
		if len(line) > available {
			line = line[:available] + "..."
		}
		rendered = append(rendered, "  "+prefix+" "+sty.Tool.ContentCode.Render(line))
	}
	if truncated {
		rendered = append(rendered, "  "+prefix+" "+sty.Tool.Truncation.Render(
			fmt.Sprintf("... (%d lines hidden)", len(strings.Split(content, "\n"))-maxLines),
		))
	}

	return strings.Join(rendered, "\n")
}

func renderThinking(msg *Message, sty *styles.Styles, width int) string {
	content := msg.Content
	lines := strings.Split(content, "\n")
	maxLines := 10
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
		content = "... " + strings.Join(lines, "\n")
	}

	rendered := sty.Chat.Thinking.Width(width - 4).Render(content)
	footer := sty.Chat.ThinkingFooter.Render("Thinking...")
	return rendered + "\n" + footer
}

func renderError(msg *Message, sty *styles.Styles, width int) string {
	tag := sty.Chat.ErrorTag.Render("ERROR")
	title := sty.Chat.ErrorTitle.Render(msg.Content)
	return fmt.Sprintf("  %s %s", tag, title)
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
