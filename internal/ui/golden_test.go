package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/ui/chat"
	uiinvariants "github.com/fugue-labs/golem/internal/ui/invariants"
	"github.com/fugue-labs/golem/internal/ui/styles"
)

func TestRuntimeSummaryGolden(t *testing.T) {
	m := New(&config.Config{
		Provider:             config.ProviderOpenAI,
		ProviderSource:       config.SourceGolemEnv,
		Model:                "gpt-5.4",
		RouterModel:          "router-mini",
		APIKey:               "test-key",
		Timeout:              time.Minute,
		TeamMode:             "auto",
		ReasoningEffort:      "low",
		AutoContextMaxTokens: 100,
		AutoContextKeepLastN: 2,
		TopLevelPersonality:  true,
	})
	m.runtime = agent.RuntimeState{
		RouterModelName:   "router-resolved",
		TeamModeReason:    "auto router pending",
		CodeModeStatus:    "on",
		OpenImageStatus:   "off",
		WebSearchStatus:   "off",
		FetchURLStatus:    "on",
		AskUserStatus:     "pending",
		EffectiveTeamMode: false,
	}

	got := m.renderRuntimeSummaryMessage().Content
	want, err := os.ReadFile(filepath.Join("testdata", "runtime_summary.golden.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.TrimRight(got, "\n") != strings.TrimRight(string(want), "\n") {
		t.Fatalf("runtime summary mismatch\n--- got ---\n%s\n--- want ---\n%s", got, string(want))
	}
}

func expectedHelpMessage() string {
	return strings.Join([]string{
		"**Commands**",
		"",
		"- `/help` — show available commands",
		"- `/clear` — clear the current transcript",
		"- `/plan` — summarize the current tracked plan",
		"- `/invariants` — summarize the tracked invariant checklist",
		"- `/runtime` — show the effective runtime profile",
		"- `/verify` — show the latest verification summary",
		"- `/compact` — compress conversation context",
		"- `/cost` — show session cost breakdown",
		"- `/replay [file|list]` — replay a recorded session trace",
		"- `/budget` — show budget status and limits",
		"- `/resume` — restore the last saved session",
		"- `/search <query>` — search across all saved sessions",
		"- `/model [name]` — show or switch the active model",
		"- `/diff` — show git diff of uncommitted changes",
		"- `/undo [path]` — revert one unstaged git-tracked file change",
		"- `/mission [new|status|tasks|plan|approve|start|pause|cancel|list]` — mission orchestration",
		"- `/rewind [N]` — rewind to turn N (or list checkpoints)",
		"- `/doctor` — diagnose setup issues",
		"- `/config` — show effective configuration",
		"- `/team` — show team member status",
		"- `/context` — show context window usage",
		"- `/skills` — list detected skills",
		"- `/skill <name>` — toggle a skill on or off",
		"- `/spec [file]` — start or show spec-driven development",
		"- `/quit` or `/exit` — quit the app",
		"",
		"**Discoverability**",
		"",
		"- Input help stays visible while you work so the shell keeps teaching next actions",
		"- From this prompt, start with `/help`, recover context with `/search <query>`, or check setup with `/doctor`",
		"- External terminal command: `golem dashboard` opens Mission Control; use `Tab`, `Shift+Tab`, `1-4`, and `j/k` to navigate panes",
		"",
		"**Keys**",
		"",
		"- `Enter` — send",
		"- `Shift+Enter` — insert newline",
		"- `Tab` — autocomplete slash commands",
		"- `Esc` — cancel the active run",
		"- `Ctrl+L` — clear transcript",
		"- `↑/↓` — recall input history",
		"- `PgUp/PgDn` — scroll the transcript",
	}, "\n")
}

func TestHelpMessageGolden(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	got := m.renderHelpMessage().Content
	want := expectedHelpMessage()
	if strings.TrimSpace(got) != want {
		t.Fatalf("help message mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestShellLayoutViewGolden(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", PermissionMode: "suggest"})
	m.sty = styles.New(nil)
	m.width = 140
	m.height = 24
	m.input.SetValue("/help")
	m.pendingMsgs = []string{"follow-up"}
	m.invariantState = uiinvariants.State{Extracted: true}

	got := stripANSI(m.View().Content)
	for _, want := range []string{"GOLEM", "Transcript", "Input", "Status", "Workflow", "Context ·", "Activity ·", "/help", " Context ", "Help ·", "/search <query>", "Tab complete", "In another terminal run golem dashboard"} {
		if !strings.Contains(got, want) {
			t.Fatalf("shell layout missing %q\n%s", want, got)
		}
	}
}

func TestShellLayoutShortWindowGolden(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	m.sty = styles.New(nil)
	m.width = 72
	m.height = 6
	m.messages = []*chat.Message{{Kind: chat.KindAssistant, Content: "short window response"}}

	got := stripANSI(m.View().Content)
	if gotHeight := lipgloss.Height(got); gotHeight > m.height {
		t.Fatalf("short shell layout height=%d exceeds terminal height=%d\n%s", gotHeight, m.height, got)
	}
	for _, want := range []string{"GOLEM", "short window response", "❯", "Ready"} {
		if !strings.Contains(got, want) {
			t.Fatalf("short shell layout missing %q\n%s", want, got)
		}
	}
}

func TestWelcomeDiscoverabilityGolden(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", WorkingDir: "/tmp/project"})
	m.sty = styles.New(nil)

	got := stripANSI(m.renderWelcome(21, 100))
	for _, want := range []string{"Start here", "/help", "/search <query>", "/doctor", "golem dashboard", "Input help stays visible", "Shell regions", "Keys"} {
		if !strings.Contains(got, want) {
			t.Fatalf("welcome discoverability view missing %q\n%s", want, got)
		}
	}
}
