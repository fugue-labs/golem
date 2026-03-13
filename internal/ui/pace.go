package ui

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/fugue-labs/golem/internal/ui/chat"
	"github.com/fugue-labs/golem/internal/ui/styles"
)

// PaceMode controls how the agent paces its work relative to the human.
type PaceMode string

const (
	PaceOff        PaceMode = "off"        // Default — agent runs freely
	PaceCheckpoint PaceMode = "checkpoint" // Pause every N tool calls
	PacePingPong   PaceMode = "pingpong"   // Approve each mutating tool individually
	PaceReview     PaceMode = "review"     // Agent works, then shows diff for review
)

// paceCheckpointRequest is sent from the agent goroutine (OnToolEnd hook)
// to the TUI when a checkpoint interval is reached. The goroutine blocks
// on the response channel until the user continues.
type paceCheckpointRequest struct {
	runID    int
	count    int             // how many tools were run since last checkpoint
	response chan<- struct{} // close or send to resume the agent
}

// paceState tracks pace-mode runtime state.
type paceState struct {
	mode               PaceMode
	checkpointInterval int  // tool calls between checkpoints (default 5)
	clarifyFirst       bool // ask clarifying questions before executing
}

func defaultPaceState() paceState {
	return paceState{
		mode:               PaceOff,
		checkpointInterval: 5,
	}
}

func parsePaceMode(s string) (PaceMode, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "off":
		return PaceOff, true
	case "checkpoint", "cp":
		return PaceCheckpoint, true
	case "pingpong", "pp":
		return PacePingPong, true
	case "review", "rev":
		return PaceReview, true
	default:
		return PaceOff, false
	}
}

func (m *Model) handlePaceCommand(text string) *chat.Message {
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/pace"))

	if arg == "" {
		// Show current pace mode.
		var b strings.Builder
		b.WriteString("**Pace mode**\n\n")
		fmt.Fprintf(&b, "- Mode: `%s`\n", m.pace.mode)
		if m.pace.mode == PaceCheckpoint {
			fmt.Fprintf(&b, "- Checkpoint interval: every %d tool calls\n", m.pace.checkpointInterval)
		}
		fmt.Fprintf(&b, "- Clarify first: `%t`\n", m.pace.clarifyFirst)
		b.WriteString("\n**Available modes:**\n")
		b.WriteString("- `off` — agent runs freely (default)\n")
		b.WriteString("- `checkpoint [N]` — pause every N tool calls for review (default: 5)\n")
		b.WriteString("- `pingpong` — approve each mutating edit individually\n")
		b.WriteString("- `review` — show git diff summary after each run\n")
		b.WriteString("- `clarify` — toggle: agent asks questions before implementing\n")
		b.WriteString("\nUsage: `/pace <mode>`, `/pace checkpoint 10`, `/pace clarify`\n")
		return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
	}

	// Handle "clarify" toggle separately.
	if strings.ToLower(arg) == "clarify" {
		m.pace.clarifyFirst = !m.pace.clarifyFirst
		if m.pace.clarifyFirst {
			return &chat.Message{
				Kind:    chat.KindAssistant,
				Content: "Clarify mode **enabled**. The agent will ask clarifying questions before implementing.",
			}
		}
		return &chat.Message{
			Kind:    chat.KindAssistant,
			Content: "Clarify mode **disabled**. The agent will execute immediately.",
		}
	}

	parts := strings.Fields(arg)
	mode, ok := parsePaceMode(parts[0])
	if !ok {
		return &chat.Message{
			Kind:    chat.KindError,
			Content: fmt.Sprintf("Unknown pace mode: %q. Options: off, checkpoint, pingpong, review, clarify", parts[0]),
		}
	}

	m.pace.mode = mode

	// Parse optional interval for checkpoint mode.
	if mode == PaceCheckpoint && len(parts) > 1 {
		var interval int
		if _, err := fmt.Sscanf(parts[1], "%d", &interval); err == nil && interval > 0 {
			m.pace.checkpointInterval = interval
		}
	}

	switch mode {
	case PaceOff:
		return &chat.Message{Kind: chat.KindAssistant, Content: "Pace mode **disabled**. Agent runs freely."}
	case PaceCheckpoint:
		return &chat.Message{
			Kind:    chat.KindAssistant,
			Content: fmt.Sprintf("**Checkpoint** mode enabled. Agent will pause every %d tool calls for your review.", m.pace.checkpointInterval),
		}
	case PacePingPong:
		return &chat.Message{
			Kind:    chat.KindAssistant,
			Content: "**Ping-pong** mode enabled. Every mutating tool call requires your explicit approval.",
		}
	case PaceReview:
		return &chat.Message{
			Kind:    chat.KindAssistant,
			Content: "**Review** mode enabled. A git diff summary will be shown after each agent run.",
		}
	}
	return nil
}

func (m *Model) handleCheckpointKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "enter":
		text := strings.TrimSpace(m.input.Value())
		if text != "" {
			// Inject user feedback via pending messages for the steering middleware.
			m.pendingMu.Lock()
			m.pendingMsgs = append(m.pendingMsgs, text)
			m.pendingMu.Unlock()
			m.messages = append(m.messages, &chat.Message{Kind: chat.KindUser, Content: text})
		}
		m.input.Reset()
		m.input.SetHeight(1)
		m.paceCheckpointActive = false
		// Resume the agent goroutine.
		if m.paceCheckpointResp != nil {
			select {
			case m.paceCheckpointResp <- struct{}{}:
			default:
			}
			m.paceCheckpointResp = nil
		}
		return m, m.input.Focus()

	case "escape":
		m.paceCheckpointActive = false
		if m.paceCheckpointResp != nil {
			select {
			case m.paceCheckpointResp <- struct{}{}:
			default:
			}
			m.paceCheckpointResp = nil
		}
		if m.cancel != nil {
			m.cancelActiveRun(true)
		}
		return m, m.input.Focus()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) renderCheckpoint(count int) string {
	var b strings.Builder
	b.WriteString(m.sty.TagWarning.Render(" Checkpoint "))
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %d tool calls completed. Review the progress above.\n", count)
	b.WriteString("\n")
	b.WriteString(m.sty.Subtle.Render("  Press Enter to continue, type feedback to steer, or Esc to cancel"))
	b.WriteString("\n")
	prompt := m.sty.Input.Prompt.Render(" " + styles.PromptIcon + " ")
	b.WriteString(prompt)
	b.WriteString(m.input.View())
	return b.String()
}

// paceReviewDiff generates a git diff summary for review mode.
// Returns nil if no changes or not in review mode.
func (m *Model) paceReviewDiff() *chat.Message {
	if m.pace.mode != PaceReview {
		return nil
	}
	dir := m.cfg.WorkingDir
	if dir == "" {
		dir = "."
	}

	// Get diff stat.
	cmd := exec.Command("git", "diff", "--stat", "--no-color")
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return nil
	}
	stat := strings.TrimSpace(out.String())
	if stat == "" {
		// Check staged changes too.
		cmd2 := exec.Command("git", "diff", "--cached", "--stat", "--no-color")
		cmd2.Dir = dir
		var out2 bytes.Buffer
		cmd2.Stdout = &out2
		cmd2.Stderr = &out2
		if err := cmd2.Run(); err == nil {
			stat = strings.TrimSpace(out2.String())
		}
	}
	if stat == "" {
		return nil
	}

	var b strings.Builder
	b.WriteString("**Review: changes made this run**\n\n")
	b.WriteString("```\n")
	b.WriteString(stat)
	b.WriteString("\n```\n")
	b.WriteString("\nUse `/diff` for full diff, `/undo` to revert.")
	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

// paceShouldForceApproval returns true if pingpong mode requires approval
// for this tool (bypassing the "always allow" session cache).
func (m *Model) paceShouldForceApproval(toolName string) bool {
	if m.pace.mode != PacePingPong {
		return false
	}
	return mutatingTools[toolName]
}

// paceClarifyPrompt wraps the user prompt with a clarification instruction
// when clarify-first mode is enabled.
func (m *Model) paceClarifyPrompt(prompt string) string {
	if !m.pace.clarifyFirst {
		return prompt
	}
	return prompt + "\n\n[Pace mode: Before implementing, ask me 2-3 clarifying questions about this request to make sure you understand what I need. Wait for my answers before proceeding with any code changes.]"
}
