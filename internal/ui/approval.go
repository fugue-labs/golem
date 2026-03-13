package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/fugue-labs/gollem/core"
)

// mutatingTools lists tool names that modify the filesystem or run commands.
var mutatingTools = map[string]bool{
	"bash":       true,
	"edit":       true,
	"multi_edit": true,
	"write":      true,
}

// toolApprovalRequest is sent from the ToolApprovalFunc to the TUI.
type toolApprovalRequest struct {
	runID    int
	toolName string
	argsJSON string
	response chan<- bool
}

// toolApprovalShutdownMsg signals that the approval listener should stop.
type toolApprovalShutdownMsg struct{}

// makeToolApprovalFunc creates a core.ToolApprovalFunc that blocks until the
// TUI user approves or denies the tool call. Read-only tools are auto-approved.
// It follows the same channel-based pattern as ask_mode.go.
func makeToolApprovalFunc(runID int, ch chan toolApprovalRequest) core.ToolApprovalFunc {
	return func(ctx context.Context, toolName string, argsJSON string) (bool, error) {
		// Auto-approve read-only tools.
		if !mutatingTools[toolName] {
			return true, nil
		}
		resp := make(chan bool, 1)
		select {
		case ch <- toolApprovalRequest{runID: runID, toolName: toolName, argsJSON: argsJSON, response: resp}:
		case <-ctx.Done():
			return false, ctx.Err()
		}
		select {
		case approved := <-resp:
			return approved, nil
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
}

func (m *Model) waitForToolApproval() tea.Cmd {
	return func() tea.Msg {
		select {
		case req := <-m.approvalCh:
			if req.response == nil {
				return toolApprovalShutdownMsg{}
			}
			return req
		case <-m.approvalDone:
			return toolApprovalShutdownMsg{}
		}
	}
}

func (m *Model) shutdownApprovalLoop() {
	select {
	case <-m.approvalDone:
	default:
		close(m.approvalDone)
	}
}

func (m *Model) beginApprovalMode(req toolApprovalRequest) {
	m.approvalMode = true
	m.approvalTool = req.toolName
	m.approvalArgs = req.argsJSON
	m.approvalRespCh = req.response
}

func (m *Model) resolveApproval(approved bool) {
	if m.approvalRespCh != nil {
		select {
		case m.approvalRespCh <- approved:
		default:
		}
	}
	if approved && m.approvalTool != "" {
		m.approvalAlways[m.approvalTool] = true
	}
	m.approvalMode = false
	m.approvalTool = ""
	m.approvalArgs = ""
	m.approvalRespCh = nil
}

func (m *Model) handleApprovalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "y", "Y", "enter":
		m.resolveApproval(true)
		return m, m.input.Focus()
	case "n", "N", "escape":
		m.resolveApproval(false)
		return m, m.input.Focus()
	case "a", "A":
		// Always allow this tool for the session.
		m.resolveApproval(true)
		return m, m.input.Focus()
	}
	return m, nil
}

func (m *Model) renderApproval() string {
	if !m.approvalMode {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.sty.TagWarning.Render(" Tool approval required "))
	b.WriteString("\n")

	if m.approvalTool == "bash" {
		// For bash, show the command prominently with $ prompt.
		summary := formatApprovalArgs(m.approvalTool, m.approvalArgs, m.width-6)
		if summary != "" {
			b.WriteString("  ")
			b.WriteString(m.sty.Bold.Render("$ "))
			b.WriteString(summary)
			b.WriteString("\n")
		}
	} else {
		fmt.Fprintf(&b, "  Tool: %s\n", m.sty.Bold.Render(m.approvalTool))
		summary := formatApprovalArgs(m.approvalTool, m.approvalArgs, m.width-4)
		if summary != "" {
			b.WriteString("  ")
			b.WriteString(summary)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(m.sty.Subtle.Render("  [y]es  [n]o  [a]lways allow"))
	return b.String()
}

// formatApprovalArgs produces a compact, human-readable summary of tool args.
func formatApprovalArgs(toolName string, argsJSON string, maxWidth int) string {
	if argsJSON == "" || argsJSON == "{}" {
		return ""
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return truncate(argsJSON, maxWidth)
	}

	switch toolName {
	case "bash":
		if cmd, ok := args["command"].(string); ok {
			return truncate(cmd, maxWidth)
		}
	case "edit":
		if path, ok := args["file_path"].(string); ok {
			return truncate("file: "+path, maxWidth)
		}
	case "multi_edit":
		if path, ok := args["file_path"].(string); ok {
			return truncate("file: "+path, maxWidth)
		}
	case "write":
		if path, ok := args["file_path"].(string); ok {
			return truncate("file: "+path, maxWidth)
		}
	}

	// Generic: show first key=value.
	for k, v := range args {
		s := fmt.Sprintf("%s: %v", k, v)
		return truncate(s, maxWidth)
	}
	return ""
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 80
	}
	// Take first line only.
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
