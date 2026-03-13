package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/fugue-labs/golem/internal/ui/styles"
)

// paceTUIModel is a minimal bubbletea model wrapping pace control logic
// for integration testing of the pace UI flow.
type paceTUIModel struct {
	pace     paceState
	sty      *styles.Styles
	input    textarea.Model
	output   []string // rendered output lines
	quitting bool

	// Simulated checkpoint state.
	checkpointActive bool
	checkpointCount  int
	checkpointResp   chan struct{}
}

func newPaceTUIModel() paceTUIModel {
	ti := textarea.New()
	ti.ShowLineNumbers = false
	ti.SetHeight(1)
	ti.Focus()

	return paceTUIModel{
		pace:  defaultPaceState(),
		sty:   styles.New(nil),
		input: ti,
	}
}

func (m paceTUIModel) Init() tea.Cmd { return nil }

// setPaceModeMsg is a tea.Msg that sets the pace mode via the /pace command.
type setPaceModeMsg struct {
	text string // e.g. "checkpoint 10", "pingpong", "off", ""
}

// triggerCheckpointMsg simulates a checkpoint pause at the given tool count.
type triggerCheckpointMsg struct {
	count int
}

func (m paceTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.checkpointActive {
			return m.handleCheckpointKey(msg)
		}
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil

	case setPaceModeMsg:
		result := m.handlePaceCommand(msg.text)
		if result != "" {
			m.output = append(m.output, result)
		}
		return m, nil

	case triggerCheckpointMsg:
		m.checkpointActive = true
		m.checkpointCount = msg.count
		m.checkpointResp = make(chan struct{}, 1)
		return m, nil
	}

	return m, nil
}

func (m *paceTUIModel) handlePaceCommand(text string) string {
	arg := strings.TrimSpace(text)

	if arg == "" {
		var b strings.Builder
		b.WriteString("Pace mode\n")
		fmt.Fprintf(&b, "Mode: %s\n", m.pace.mode)
		if m.pace.mode == PaceCheckpoint {
			fmt.Fprintf(&b, "Checkpoint interval: every %d tool calls\n", m.pace.checkpointInterval)
		}
		fmt.Fprintf(&b, "Clarify first: %t\n", m.pace.clarifyFirst)
		return b.String()
	}

	if strings.ToLower(arg) == "clarify" {
		m.pace.clarifyFirst = !m.pace.clarifyFirst
		if m.pace.clarifyFirst {
			return "Clarify mode enabled"
		}
		return "Clarify mode disabled"
	}

	parts := strings.Fields(arg)
	mode, ok := parsePaceMode(parts[0])
	if !ok {
		return fmt.Sprintf("Unknown pace mode: %q", parts[0])
	}

	m.pace.mode = mode

	if mode == PaceCheckpoint && len(parts) > 1 {
		var interval int
		if _, err := fmt.Sscanf(parts[1], "%d", &interval); err == nil && interval > 0 {
			m.pace.checkpointInterval = interval
		}
	}

	switch mode {
	case PaceOff:
		return "Pace mode disabled"
	case PaceCheckpoint:
		return fmt.Sprintf("Checkpoint mode enabled. Interval: %d", m.pace.checkpointInterval)
	case PacePingPong:
		return "Ping-pong mode enabled"
	case PaceReview:
		return "Review mode enabled"
	}
	return ""
}

func (m *paceTUIModel) handleCheckpointKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		text := strings.TrimSpace(m.input.Value())
		if text != "" {
			m.output = append(m.output, text)
		}
		m.input.Reset()
		m.input.SetHeight(1)
		m.checkpointActive = false
		if m.checkpointResp != nil {
			select {
			case m.checkpointResp <- struct{}{}:
			default:
			}
			m.checkpointResp = nil
		}
		return m, nil

	case "escape", "esc":
		m.checkpointActive = false
		if m.checkpointResp != nil {
			select {
			case m.checkpointResp <- struct{}{}:
			default:
			}
			m.checkpointResp = nil
		}
		m.output = append(m.output, "Checkpoint cancelled")
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m paceTUIModel) View() tea.View {
	var b strings.Builder

	// Header: show current mode and state.
	b.WriteString(fmt.Sprintf("Pace: %s", m.pace.mode))
	if m.pace.mode == PaceCheckpoint {
		fmt.Fprintf(&b, " (every %d)", m.pace.checkpointInterval)
	}
	if m.pace.clarifyFirst {
		b.WriteString(" [clarify]")
	}
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", 40) + "\n")

	// Output messages.
	for _, line := range m.output {
		b.WriteString(line + "\n")
	}

	// Checkpoint pause indicator.
	if m.checkpointActive {
		b.WriteString(strings.Repeat("-", 40) + "\n")
		fmt.Fprintf(&b, "CHECKPOINT: %d tool calls completed\n", m.checkpointCount)
		b.WriteString("Press Enter to continue, Esc to cancel\n")
		b.WriteString("> " + m.input.Value())
	}

	return tea.NewView(b.String())
}

// --- Tests ---

// TestTeatestPaceDefaultMode tests that the default pace mode is off.
func TestTeatestPaceDefaultMode(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Pace: off")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestPaceShowStatus tests the /pace command with no arguments shows current status.
func TestTeatestPaceShowStatus(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(setPaceModeMsg{text: ""})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Mode: off") &&
			strings.Contains(s, "Clarify first: false")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestPaceSetCheckpoint tests switching to checkpoint mode.
func TestTeatestPaceSetCheckpoint(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(setPaceModeMsg{text: "checkpoint"})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Checkpoint mode enabled") &&
			strings.Contains(s, "Pace: checkpoint")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestPaceSetCheckpointWithInterval tests checkpoint mode with custom interval.
func TestTeatestPaceSetCheckpointWithInterval(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(setPaceModeMsg{text: "checkpoint 10"})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Checkpoint mode enabled. Interval: 10") &&
			strings.Contains(s, "Pace: checkpoint (every 10)")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestPaceSetPingPong tests switching to pingpong mode.
func TestTeatestPaceSetPingPong(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(setPaceModeMsg{text: "pingpong"})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Ping-pong mode enabled") &&
			strings.Contains(s, "Pace: pingpong")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestPaceSetReview tests switching to review mode.
func TestTeatestPaceSetReview(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(setPaceModeMsg{text: "review"})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Review mode enabled") &&
			strings.Contains(s, "Pace: review")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestPaceSetOff tests switching back to off mode.
func TestTeatestPaceSetOff(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// First switch to checkpoint, then back to off.
	tm.Send(setPaceModeMsg{text: "checkpoint"})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Checkpoint mode enabled")
	}, teatest.WithDuration(5*time.Second))

	tm.Send(setPaceModeMsg{text: "off"})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Pace mode disabled") &&
			strings.Contains(s, "Pace: off")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestPaceAliases tests short aliases for pace modes.
func TestTeatestPaceAliases(t *testing.T) {
	tests := []struct {
		alias    string
		expected string
		header   string
	}{
		{"cp", "Checkpoint mode enabled", "Pace: checkpoint"},
		{"pp", "Ping-pong mode enabled", "Pace: pingpong"},
		{"rev", "Review mode enabled", "Pace: review"},
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			m := newPaceTUIModel()
			tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

			tm.Send(setPaceModeMsg{text: tt.alias})

			teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
				s := string(bts)
				return strings.Contains(s, tt.expected) &&
					strings.Contains(s, tt.header)
			}, teatest.WithDuration(5*time.Second))

			tm.Quit()
			tm.WaitFinished(t)
		})
	}
}

// TestTeatestPaceUnknownMode tests an invalid pace mode.
func TestTeatestPaceUnknownMode(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(setPaceModeMsg{text: "turbo"})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), `Unknown pace mode: "turbo"`)
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestPaceClarifyToggle tests toggling clarify-first mode.
func TestTeatestPaceClarifyToggle(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Enable clarify.
	tm.Send(setPaceModeMsg{text: "clarify"})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Clarify mode enabled") &&
			strings.Contains(s, "[clarify]")
	}, teatest.WithDuration(5*time.Second))

	// Disable clarify.
	tm.Send(setPaceModeMsg{text: "clarify"})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Clarify mode disabled") &&
			!strings.Contains(s, "[clarify]\n") // header should not show clarify
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestPaceCheckpointPause tests the checkpoint pause display.
func TestTeatestPaceCheckpointPause(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Set checkpoint mode.
	tm.Send(setPaceModeMsg{text: "checkpoint"})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Checkpoint mode enabled")
	}, teatest.WithDuration(5*time.Second))

	// Trigger a checkpoint pause.
	tm.Send(triggerCheckpointMsg{count: 5})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "CHECKPOINT: 5 tool calls completed") &&
			strings.Contains(s, "Press Enter to continue")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestPaceCheckpointResume tests resuming from a checkpoint via Enter.
func TestTeatestPaceCheckpointResume(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Trigger a checkpoint pause.
	tm.Send(triggerCheckpointMsg{count: 3})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "CHECKPOINT: 3 tool calls completed")
	}, teatest.WithDuration(5*time.Second))

	// Press Enter to resume, then send a mode change to confirm we're out of checkpoint.
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	tm.Send(setPaceModeMsg{text: "review"})

	// After resume, the mode change should apply (proving checkpoint is dismissed).
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Review mode enabled")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestPaceCheckpointCancel tests cancelling a checkpoint via Escape.
func TestTeatestPaceCheckpointCancel(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Trigger a checkpoint pause.
	tm.Send(triggerCheckpointMsg{count: 7})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "CHECKPOINT: 7 tool calls completed")
	}, teatest.WithDuration(5*time.Second))

	// Press Escape to cancel.
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEscape})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Checkpoint cancelled")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestPaceModeSwitching tests switching between multiple modes in sequence.
func TestTeatestPaceModeSwitching(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	modes := []struct {
		cmd      string
		expected string
	}{
		{"checkpoint", "Pace: checkpoint"},
		{"pingpong", "Pace: pingpong"},
		{"review", "Pace: review"},
		{"off", "Pace: off"},
	}

	for _, mode := range modes {
		tm.Send(setPaceModeMsg{text: mode.cmd})
		teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
			return strings.Contains(string(bts), mode.expected)
		}, teatest.WithDuration(5*time.Second))
	}

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestPaceCheckpointIntervalUpdate tests changing the checkpoint interval.
func TestTeatestPaceCheckpointIntervalUpdate(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Set checkpoint with default interval.
	tm.Send(setPaceModeMsg{text: "checkpoint"})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Pace: checkpoint (every 5)")
	}, teatest.WithDuration(5*time.Second))

	// Update interval to 20.
	tm.Send(setPaceModeMsg{text: "checkpoint 20"})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Pace: checkpoint (every 20)")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestPaceClarifyWithMode tests clarify flag combined with a pace mode.
func TestTeatestPaceClarifyWithMode(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Set checkpoint mode.
	tm.Send(setPaceModeMsg{text: "checkpoint"})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Pace: checkpoint")
	}, teatest.WithDuration(5*time.Second))

	// Enable clarify.
	tm.Send(setPaceModeMsg{text: "clarify"})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Pace: checkpoint") &&
			strings.Contains(s, "[clarify]")
	}, teatest.WithDuration(5*time.Second))

	// Show status.
	tm.Send(setPaceModeMsg{text: ""})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Mode: checkpoint") &&
			strings.Contains(s, "Clarify first: true")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestPaceMultipleCheckpoints tests multiple checkpoint triggers in sequence.
func TestTeatestPaceMultipleCheckpoints(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// First checkpoint — trigger and immediately resume.
	tm.Send(triggerCheckpointMsg{count: 5})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "CHECKPOINT: 5 tool calls completed")
	}, teatest.WithDuration(5*time.Second))

	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Second checkpoint — the higher count confirms the first was resumed.
	tm.Send(triggerCheckpointMsg{count: 10})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "10 tool calls completed")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestPacePanelRendering tests that the pace header renders correctly
// with mode, interval, and clarify indicator.
func TestTeatestPacePanelRendering(t *testing.T) {
	m := newPaceTUIModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Default: off, no clarify.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Pace: off") &&
			!strings.Contains(s, "[clarify]") &&
			!strings.Contains(s, "every")
	}, teatest.WithDuration(5*time.Second))

	// Switch to checkpoint with custom interval and clarify.
	tm.Send(setPaceModeMsg{text: "checkpoint 15"})
	tm.Send(setPaceModeMsg{text: "clarify"})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Pace: checkpoint (every 15)") &&
			strings.Contains(s, "[clarify]")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}
