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

func TestHelpMessageGolden(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4"})
	got := m.renderHelpMessage().Content
	for _, want := range []string{
		"**Commands**",
		"/help",
		"/search <query>",
		"/mission",
		"**Discoverability**",
		"golem dashboard",
		"Enter",
		"Esc",
		"Tab",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help message missing %q\n%s", want, got)
		}
	}
}

func TestShellLayoutViewGolden(t *testing.T) {
	m := New(&config.Config{Provider: config.ProviderOpenAI, Model: "gpt-5.4", PermissionMode: "suggest"})
	m.sty = styles.New(nil)
	m.width = 120
	m.height = 24
	m.input.SetValue("/help")
	m.pendingMsgs = []string{"follow-up"}
	m.invariantState = uiinvariants.State{Extracted: true}

	got := stripANSI(m.View().Content)
	for _, want := range []string{"GOLEM", "Transcript", "Input", "Status", "Workflow", "Context ·", "Activity ·", "/help", " Context ", "Help ·", "/search <query>", "Tab complete"} {
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
