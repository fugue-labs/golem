package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/ui/chat"
	uiinvariants "github.com/fugue-labs/golem/internal/ui/invariants"
	"github.com/fugue-labs/golem/internal/ui/plan"
	"github.com/fugue-labs/golem/internal/ui/styles"
	uiverification "github.com/fugue-labs/golem/internal/ui/verification"
)

// newTestModel creates a minimal Model suitable for command testing.
func newTestModel() *Model {
	return New(&config.Config{
		Provider:   config.ProviderOpenAI,
		Model:      "gpt-5.4",
		WorkingDir: ".",
		Timeout:    time.Minute,
	})
}

// simulateCommand sets the input text and presses Enter, returning the updated Model.
func simulateCommand(t *testing.T, m *Model, cmd string) *Model {
	t.Helper()
	m.input.SetValue(cmd)
	updated, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	return updated.(*Model)
}

// lastMessage returns the last chat message appended to the model's messages.
func lastMessage(t *testing.T, m *Model) string {
	t.Helper()
	if len(m.messages) == 0 {
		t.Fatal("expected at least one message")
	}
	return m.messages[len(m.messages)-1].Content
}

// ---------- Command tests ----------

func TestCommandHelp(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/help")
	content := lastMessage(t, m)
	if content == "" {
		t.Fatal("expected non-empty help output")
	}
	for _, want := range []string{"/help", "/clear", "/plan", "/search <query>", "golem dashboard", "Enter", "Esc", "Tab"} {
		if !strings.Contains(content, want) {
			t.Fatalf("help output missing %q", want)
		}
	}
}

func TestContextualHelpIdle(t *testing.T) {
	m := newTestModel()
	m.sty = styles.New(nil)
	m.width = 100
	got := stripANSI(m.renderContextualHelpLine(m.width))
	for _, want := range []string{"Help ·", "Try /help", "/search <query>", "/doctor", "Enter send", "Tab complete", "Esc cancel"} {
		if !strings.Contains(got, want) {
			t.Fatalf("contextual help missing %q in %q", want, got)
		}
	}
}

func TestCommandClear(t *testing.T) {
	m := newTestModel()
	m.messages = append(m.messages, &chat.Message{Kind: chat.KindAssistant, Content: "old"})
	m = simulateCommand(t, m, "/clear")
	if len(m.messages) != 0 {
		t.Fatalf("expected messages to be cleared, got %d", len(m.messages))
	}
	if m.input.Value() != "" {
		t.Fatal("expected input to be reset")
	}
}

func TestCommandPlanEmpty(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/plan")
	content := lastMessage(t, m)
	if !strings.Contains(content, "No active plan") {
		t.Fatalf("expected empty plan message, got %q", content)
	}
}

func TestCommandPlanPopulated(t *testing.T) {
	m := newTestModel()
	m.planState = plan.State{Tasks: []plan.Task{
		{ID: "T1", Description: "build feature", Status: "completed"},
		{ID: "T2", Description: "add tests", Status: "in_progress"},
	}}
	m = simulateCommand(t, m, "/plan")
	content := lastMessage(t, m)
	if !strings.Contains(content, "Current plan") {
		t.Fatalf("expected plan summary, got %q", content)
	}
	if !strings.Contains(content, "T1") || !strings.Contains(content, "T2") {
		t.Fatalf("expected task IDs in plan output, got %q", content)
	}
}

func TestCommandInvariantsEmpty(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/invariants")
	content := lastMessage(t, m)
	if !strings.Contains(content, "No invariant checklist") {
		t.Fatalf("expected empty invariants message, got %q", content)
	}
}

func TestCommandInvariantsPopulated(t *testing.T) {
	m := newTestModel()
	m.invariantState = uiinvariants.State{
		Extracted: true,
		Items: []uiinvariants.Item{
			{ID: "I1", Description: "tests pass", Kind: "hard", Status: "pass"},
		},
	}
	m = simulateCommand(t, m, "/invariants")
	content := lastMessage(t, m)
	if !strings.Contains(content, "Invariant checklist") {
		t.Fatalf("expected invariant checklist, got %q", content)
	}
	if !strings.Contains(content, "I1") {
		t.Fatalf("expected invariant ID in output, got %q", content)
	}
}

func TestCommandVerifyEmpty(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/verify")
	content := lastMessage(t, m)
	if !strings.Contains(content, "No verification") {
		t.Fatalf("expected empty verification message, got %q", content)
	}
}

func TestCommandVerifyPopulated(t *testing.T) {
	m := newTestModel()
	m.verificationState = uiverification.State{Entries: []uiverification.Entry{
		{ID: "V1", Command: "go test ./...", Status: "pass", Freshness: "fresh"},
	}}
	m = simulateCommand(t, m, "/verify")
	content := lastMessage(t, m)
	if !strings.Contains(content, "Verification summary") {
		t.Fatalf("expected verification summary, got %q", content)
	}
}

func TestCommandCost(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/cost")
	content := lastMessage(t, m)
	if content == "" {
		t.Fatal("expected non-empty cost output")
	}
	// With no usage, should indicate no usage recorded
	if !strings.Contains(content, "No usage recorded") {
		t.Fatalf("expected no-usage message, got %q", content)
	}
}

func TestCommandCostWithUsage(t *testing.T) {
	m := newTestModel()
	m.sessionUsage.Requests = 3
	m.sessionUsage.InputTokens = 1000
	m.sessionUsage.OutputTokens = 500
	m = simulateCommand(t, m, "/cost")
	content := lastMessage(t, m)
	if !strings.Contains(content, "Session cost summary") {
		t.Fatalf("expected cost summary, got %q", content)
	}
	if !strings.Contains(content, "Requests: 3") {
		t.Fatalf("expected request count in cost output, got %q", content)
	}
}

func TestCommandBudget(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/budget")
	content := lastMessage(t, m)
	if content == "" {
		t.Fatal("expected non-empty budget output")
	}
	if !strings.Contains(content, "Budget status") {
		t.Fatalf("expected budget status, got %q", content)
	}
}

func TestCommandContext(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/context")
	content := lastMessage(t, m)
	if content == "" {
		t.Fatal("expected non-empty context output")
	}
	if !strings.Contains(content, "Context window") {
		t.Fatalf("expected context window info, got %q", content)
	}
	if !strings.Contains(content, "gpt-5.4") {
		t.Fatalf("expected model name in context output, got %q", content)
	}
}

func TestCommandRuntime(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/runtime")
	content := lastMessage(t, m)
	if content == "" {
		t.Fatal("expected non-empty runtime output")
	}
}

func TestCommandConfig(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/config")
	content := lastMessage(t, m)
	if content == "" {
		t.Fatal("expected non-empty config output")
	}
	if !strings.Contains(content, "Effective configuration") {
		t.Fatalf("expected config display, got %q", content)
	}
	if !strings.Contains(content, "gpt-5.4") {
		t.Fatalf("expected model name in config, got %q", content)
	}
}

func TestCommandDoctor(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/doctor")
	content := lastMessage(t, m)
	if content == "" {
		t.Fatal("expected non-empty doctor output")
	}
	if !strings.Contains(content, "Golem Doctor") {
		t.Fatalf("expected doctor header, got %q", content)
	}
}

func TestCommandModel(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/model")
	content := lastMessage(t, m)
	if content == "" {
		t.Fatal("expected non-empty model output")
	}
	if !strings.Contains(content, "gpt-5.4") {
		t.Fatalf("expected current model name, got %q", content)
	}
}

func TestCommandModelSwitch(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/model gpt-4o")
	content := lastMessage(t, m)
	if !strings.Contains(content, "Switched model") {
		t.Fatalf("expected switch confirmation, got %q", content)
	}
	if m.cfg.Model != "gpt-4o" {
		t.Fatalf("model not switched, got %q", m.cfg.Model)
	}
}

func TestCommandResume(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/resume")
	content := lastMessage(t, m)
	// With no session data, should report no previous session
	if !strings.Contains(content, "No previous session") && !strings.Contains(content, "Failed to load") {
		t.Fatalf("expected no-session message, got %q", content)
	}
}

func TestCommandRewind(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/rewind")
	content := lastMessage(t, m)
	if !strings.Contains(content, "No checkpoints") {
		t.Fatalf("expected no-checkpoints message, got %q", content)
	}
}

func TestCommandReplayList(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/replay list")
	content := lastMessage(t, m)
	// Should get either a list or a "no traces" message
	if content == "" {
		t.Fatal("expected non-empty replay output")
	}
}

func TestCommandDiff(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/diff")
	content := lastMessage(t, m)
	// Should get either git diff output or "No uncommitted changes" or an error
	if content == "" {
		t.Fatal("expected non-empty diff output")
	}
}

func TestCommandSearch(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/search")
	content := lastMessage(t, m)
	for _, want := range []string{"Usage:", "/search <query>", "search across all saved sessions", "Examples"} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected search usage message to contain %q, got %q", want, content)
		}
	}
}

func TestCommandTeam(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/team")
	content := lastMessage(t, m)
	if !strings.Contains(content, "No team active") {
		t.Fatalf("expected no-team message, got %q", content)
	}
}

func TestCommandSkills(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/skills")
	content := lastMessage(t, m)
	// Should show either skill list or "No skills found"
	if content == "" {
		t.Fatal("expected non-empty skills output")
	}
}

func TestCommandSpec(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/spec")
	content := lastMessage(t, m)
	if !strings.Contains(content, "No active spec") {
		t.Fatalf("expected no-spec message, got %q", content)
	}
}

func TestCommandMission(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/mission")
	content := lastMessage(t, m)
	// Should show mission help or status
	if content == "" {
		t.Fatal("expected non-empty mission output")
	}
}

func TestCommandUnknown(t *testing.T) {
	m := simulateCommand(t, newTestModel(), "/notacommand")
	content := lastMessage(t, m)
	if !strings.Contains(content, "Unknown command") {
		t.Fatalf("expected unknown command error, got %q", content)
	}
}

func TestCommandInputResetAfterCommand(t *testing.T) {
	commands := []string{
		"/help", "/plan", "/invariants", "/verify", "/cost",
		"/budget", "/context", "/runtime", "/config", "/doctor",
		"/model", "/diff", "/search", "/team", "/skills",
		"/rewind", "/spec", "/mission",
	}
	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			m := simulateCommand(t, newTestModel(), cmd)
			if m.input.Value() != "" {
				t.Fatalf("input not reset after %s", cmd)
			}
		})
	}
}

func TestCommandQuit(t *testing.T) {
	m := newTestModel()
	m.input.SetValue("/quit")
	_, cmd := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	// /quit should return tea.Quit
	if cmd == nil {
		t.Fatal("expected quit command")
	}
}

func TestCommandExit(t *testing.T) {
	m := newTestModel()
	m.input.SetValue("/exit")
	_, cmd := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected quit command")
	}
}
