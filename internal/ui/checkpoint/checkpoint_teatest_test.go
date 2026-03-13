package checkpoint

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
)

// checkpointTUIModel is a minimal bubbletea model wrapping checkpoint.Store
// for integration testing of the checkpoint/rewind TUI flow.
type checkpointTUIModel struct {
	store     *Store
	turnCount int
	output    []string // rendered message lines
	input     string
	quitting  bool
}

func newCheckpointTUIModel(workDir string) checkpointTUIModel {
	return checkpointTUIModel{
		store: NewStore(workDir),
	}
}

func (m checkpointTUIModel) Init() tea.Cmd { return nil }

func (m checkpointTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			m.handleInput()
			m.input = ""
			return m, nil
		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
			return m, nil
		default:
			if len(msg.Text) == 1 {
				m.input += msg.Text
			}
			return m, nil
		}

	case createCheckpointMsg:
		m.turnCount++
		m.store.Save(Checkpoint{
			Turn:      m.turnCount,
			Prompt:    msg.prompt,
			CreatedAt: time.Now(),
		})
		m.output = append(m.output, fmt.Sprintf("checkpoint created: turn %d", m.turnCount))
		return m, nil
	}

	return m, nil
}

func (m *checkpointTUIModel) handleInput() {
	text := strings.TrimSpace(m.input)
	if text == "" {
		return
	}

	switch {
	case text == "/rewind":
		// List checkpoints.
		if m.store.Len() == 0 {
			m.output = append(m.output, "No checkpoints yet.")
			return
		}
		m.output = append(m.output, "Checkpoints:")
		for _, s := range m.store.List() {
			m.output = append(m.output, "  "+s)
		}
		m.output = append(m.output, "Use /rewind N to restore turn N.")

	case strings.HasPrefix(text, "/rewind "):
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/rewind "))
		turn, err := strconv.Atoi(arg)
		if err != nil {
			m.output = append(m.output, fmt.Sprintf("Invalid turn: %q", arg))
			return
		}
		cp, err := m.store.RewindTo(turn)
		if err != nil {
			m.output = append(m.output, fmt.Sprintf("Rewind failed: %v", err))
			return
		}
		m.turnCount = cp.Turn
		m.output = append(m.output, fmt.Sprintf("Rewound to turn %d. Prompt was: %q", cp.Turn, cp.Prompt))

	default:
		m.output = append(m.output, fmt.Sprintf("unknown command: %s", text))
	}
}

func (m checkpointTUIModel) View() tea.View {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Checkpoints: %d | Turn: %d\n", m.store.Len(), m.turnCount))
	b.WriteString(strings.Repeat("-", 40) + "\n")
	for _, line := range m.output {
		b.WriteString(line + "\n")
	}
	b.WriteString(strings.Repeat("-", 40) + "\n")
	b.WriteString("> " + m.input)
	return tea.NewView(b.String())
}

// createCheckpointMsg is a tea.Msg that triggers checkpoint creation.
type createCheckpointMsg struct {
	prompt string
}

// initGitRepo creates a minimal git repo with an initial commit in the given directory.
// This is required for tests that call RewindTo, since it runs git checkout.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	gitRun("init")
	if err := os.WriteFile(dir+"/.gitkeep", []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	gitRun("add", ".")
	gitRun("commit", "-m", "initial")
}

// TestTeatestCheckpointCreation tests creating checkpoints via the TUI model.
func TestTeatestCheckpointCreation(t *testing.T) {
	m := newCheckpointTUIModel(t.TempDir())
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Create two checkpoints by sending messages.
	tm.Send(createCheckpointMsg{prompt: "fix the bug"})
	tm.Send(createCheckpointMsg{prompt: "add tests"})

	// Wait for the view to reflect both checkpoints.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Checkpoints: 2") && strings.Contains(s, "Turn: 2")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestCheckpointList tests the /rewind command to list checkpoints.
func TestTeatestCheckpointList(t *testing.T) {
	m := newCheckpointTUIModel(t.TempDir())
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Create checkpoints.
	tm.Send(createCheckpointMsg{prompt: "first prompt"})
	tm.Send(createCheckpointMsg{prompt: "second prompt"})

	// Wait for checkpoints to be created.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Checkpoints: 2")
	}, teatest.WithDuration(5*time.Second))

	// Type /rewind to list.
	tm.Type("/rewind")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Wait for checkpoint listing in output.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Checkpoints:") &&
			strings.Contains(s, "turn 1") &&
			strings.Contains(s, "turn 2") &&
			strings.Contains(s, "Use /rewind N")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestCheckpointListEmpty tests /rewind with no checkpoints.
func TestTeatestCheckpointListEmpty(t *testing.T) {
	m := newCheckpointTUIModel(t.TempDir())
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Type /rewind with no checkpoints.
	tm.Type("/rewind")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "No checkpoints yet.")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestCheckpointRewind tests rewinding to a specific checkpoint.
func TestTeatestCheckpointRewind(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	m := newCheckpointTUIModel(dir)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Create 3 checkpoints.
	tm.Send(createCheckpointMsg{prompt: "step one"})
	tm.Send(createCheckpointMsg{prompt: "step two"})
	tm.Send(createCheckpointMsg{prompt: "step three"})

	// Wait for all three.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Checkpoints: 3")
	}, teatest.WithDuration(5*time.Second))

	// Rewind to turn 2.
	tm.Type("/rewind 2")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Verify rewind confirmation and state.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, `Rewound to turn 2`) &&
			strings.Contains(s, `"step two"`) &&
			strings.Contains(s, "Checkpoints: 2") &&
			strings.Contains(s, "Turn: 2")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestCheckpointRewindInvalid tests rewinding to a non-existent turn.
func TestTeatestCheckpointRewindInvalid(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	m := newCheckpointTUIModel(dir)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Create 1 checkpoint.
	tm.Send(createCheckpointMsg{prompt: "only one"})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Checkpoints: 1")
	}, teatest.WithDuration(5*time.Second))

	// Try rewinding to non-existent turn 5.
	tm.Type("/rewind 5")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Rewind failed")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestCheckpointRewindBadArg tests /rewind with non-numeric argument.
func TestTeatestCheckpointRewindBadArg(t *testing.T) {
	m := newCheckpointTUIModel(t.TempDir())
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Type("/rewind abc")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), `Invalid turn: "abc"`)
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestCheckpointPanelRendering tests that the View shows checkpoint count and turn.
func TestTeatestCheckpointPanelRendering(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	m := newCheckpointTUIModel(dir)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Initially: 0 checkpoints, turn 0.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Checkpoints: 0") && strings.Contains(s, "Turn: 0")
	}, teatest.WithDuration(5*time.Second))

	// Add a checkpoint.
	tm.Send(createCheckpointMsg{prompt: "hello world"})

	// Panel should update.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Checkpoints: 1") && strings.Contains(s, "Turn: 1")
	}, teatest.WithDuration(5*time.Second))

	// Add another.
	tm.Send(createCheckpointMsg{prompt: "second turn"})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Checkpoints: 2") && strings.Contains(s, "Turn: 2")
	}, teatest.WithDuration(5*time.Second))

	// Rewind to turn 1 — panel should reflect reduced count.
	tm.Type("/rewind 1")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Checkpoints: 1") && strings.Contains(s, "Turn: 1")
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}

// TestTeatestCheckpointRewindThenCreate tests creating new checkpoints after a rewind.
func TestTeatestCheckpointRewindThenCreate(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	m := newCheckpointTUIModel(dir)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Create 3 checkpoints.
	tm.Send(createCheckpointMsg{prompt: "alpha"})
	tm.Send(createCheckpointMsg{prompt: "beta"})
	tm.Send(createCheckpointMsg{prompt: "gamma"})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Checkpoints: 3")
	}, teatest.WithDuration(5*time.Second))

	// Rewind to turn 1.
	tm.Type("/rewind 1")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Checkpoints: 1") &&
			strings.Contains(string(bts), "Turn: 1")
	}, teatest.WithDuration(5*time.Second))

	// Create a new checkpoint after rewind — should be turn 2 again.
	tm.Send(createCheckpointMsg{prompt: "delta"})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Checkpoints: 2") &&
			strings.Contains(string(bts), "Turn: 2")
	}, teatest.WithDuration(5*time.Second))

	// List to verify new history.
	tm.Type("/rewind")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "turn 1") &&
			strings.Contains(s, "turn 2") &&
			strings.Contains(s, `"alpha"`) &&
			strings.Contains(s, `"delta"`) &&
			!strings.Contains(s, `"beta"`) &&
			!strings.Contains(s, `"gamma"`)
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
	tm.WaitFinished(t)
}
