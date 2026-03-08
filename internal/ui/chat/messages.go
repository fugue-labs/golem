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

// findToolCallFor finds the most recent tool call for a given tool result.
func findToolCallFor(msg *Message, allMessages []*Message) *Message {
	// Walk backwards from this message to find its tool call.
	found := false
	for i := len(allMessages) - 1; i >= 0; i-- {
		if allMessages[i] == msg {
			found = true
			continue
		}
		if found && allMessages[i].Kind == KindToolCall && allMessages[i].ToolName == msg.ToolName {
			return allMessages[i]
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
	maxLines := 15
	truncated := len(rawLines) > maxLines
	if truncated {
		rawLines = rawLines[:maxLines]
	}

	// Strip line number prefixes, highlight, then re-add.
	var codeLines []string
	var lineNums []string
	for _, line := range rawLines {
		if idx := strings.Index(line, "│ "); idx != -1 && idx < 8 {
			lineNums = append(lineNums, strings.TrimSpace(line[:idx]))
			codeLines = append(codeLines, line[idx+len("│ "):])
		} else {
			lineNums = append(lineNums, "")
			codeLines = append(codeLines, line)
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
		if len(hline) > width-10 {
			hline = hline[:width-10]
		}
		rendered = append(rendered, "    "+num+hline)
	}

	if truncated {
		rendered = append(rendered, "    "+sty.Tool.Truncation.Render(
			fmt.Sprintf("... (%d more lines)", len(strings.Split(msg.Content, "\n"))-maxLines),
		))
	}

	return strings.Join(rendered, "\n")
}

// renderEditResult renders a diff-style view showing old and new strings.
func renderEditResult(msg *Message, toolCall *Message, sty *styles.Styles, width int) string {
	var args struct {
		FilePath  string `json:"file_path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal([]byte(toolCall.RawArgs), &args); err != nil {
		return renderPlainResult(msg.Content, sty, width)
	}

	var rendered []string

	// File header.
	base := filepath.Base(args.FilePath)
	rendered = append(rendered, "    "+sty.Tool.DiffHeader.Render("--- "+base))
	rendered = append(rendered, "    "+sty.Tool.DiffHeader.Render("+++ "+base))

	// Removed lines.
	oldLines := strings.Split(args.OldString, "\n")
	maxDiffLines := 8
	for i, line := range oldLines {
		if i >= maxDiffLines {
			rendered = append(rendered, "    "+sty.Tool.Truncation.Render(
				fmt.Sprintf("  ... (%d more removed)", len(oldLines)-maxDiffLines),
			))
			break
		}
		if len(line) > width-8 {
			line = line[:width-8]
		}
		rendered = append(rendered, "    "+sty.Tool.DiffDel.Render("- "+line))
	}

	// Added lines.
	newLines := strings.Split(args.NewString, "\n")
	for i, line := range newLines {
		if i >= maxDiffLines {
			rendered = append(rendered, "    "+sty.Tool.Truncation.Render(
				fmt.Sprintf("  ... (%d more added)", len(newLines)-maxDiffLines),
			))
			break
		}
		if len(line) > width-8 {
			line = line[:width-8]
		}
		rendered = append(rendered, "    "+sty.Tool.DiffAdd.Render("+ "+line))
	}

	// Summary line from the tool result.
	rendered = append(rendered, "    "+sty.Tool.ContentLine.Render(msg.Content))

	return strings.Join(rendered, "\n")
}

// renderPlainResult renders a generic tool result with truncation.
func renderPlainResult(content string, sty *styles.Styles, width int) string {
	lines := strings.Split(content, "\n")
	maxLines := 10
	truncated := false
	if len(lines) > maxLines {
		truncated = true
		lines = lines[:maxLines]
	}

	var rendered []string
	for _, line := range lines {
		if len(line) > width-4 {
			line = line[:width-4] + "..."
		}
		rendered = append(rendered, "    "+sty.Tool.ContentLine.Render(line))
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
