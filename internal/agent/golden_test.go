package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/golem/internal/config"
)

func TestRuntimePromptGolden(t *testing.T) {
	cfg := &config.Config{
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
	}
	runtime := RuntimeState{
		RouterModelName:   "router-resolved",
		TeamModeReason:    "auto router pending",
		CodeModeStatus:    "on",
		OpenImageStatus:   "off",
		EffectiveTeamMode: false,
	}

	got := buildRuntimePrompt(cfg, runtime, nil)
	want, err := os.ReadFile(filepath.Join("testdata", "runtime_prompt.golden.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.TrimRight(got, "\n") != strings.TrimRight(string(want), "\n") {
		t.Fatalf("runtime prompt mismatch\n--- got ---\n%s\n--- want ---\n%s", got, string(want))
	}
}
