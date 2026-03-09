package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/config"
)

func TestRunStatusJSONOutputsRuntimeReport(t *testing.T) {
	origLoad := loadConfigFunc
	origPrepare := prepareRuntimeFunc
	t.Cleanup(func() {
		loadConfigFunc = origLoad
		prepareRuntimeFunc = origPrepare
	})

	loadConfigFunc = func() (*config.Config, error) {
		return &config.Config{
			Provider:             config.ProviderOpenAI,
			ProviderSource:       config.SourceGolemEnv,
			Model:                "gpt-5.4",
			APIKey:               "test-key",
			Timeout:              time.Minute,
			TeamMode:             "auto",
			AutoContextMaxTokens: 100,
			AutoContextKeepLastN: 2,
		}, nil
	}
	prepareRuntimeFunc = func(context.Context, *config.Config, string) (agent.RuntimeState, error) {
		return agent.RuntimeState{
			EffectiveTeamMode: false,
			TeamModeReason:    "auto router pending",
			RouterModelName:   "gpt-5.4",
			CodeModeStatus:    "on",
			OpenImageStatus:   "on",
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"status", "--json"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run() code=%d err=%q", code, errOut.String())
	}

	var report agent.RuntimeReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, out.String())
	}
	if report.Provider != string(config.ProviderOpenAI) {
		t.Fatalf("provider=%q", report.Provider)
	}
	if report.ToolSurfaces.ExecuteCode != "on" {
		t.Fatalf("execute_code=%q", report.ToolSurfaces.ExecuteCode)
	}
	if report.AutoContextKeepLastN != 2 {
		t.Fatalf("auto_context_keep_last_n=%d", report.AutoContextKeepLastN)
	}
}

func TestRunRuntimeShowsValidationWarnings(t *testing.T) {
	origLoad := loadConfigFunc
	origPrepare := prepareRuntimeFunc
	t.Cleanup(func() {
		loadConfigFunc = origLoad
		prepareRuntimeFunc = origPrepare
	})

	loadConfigFunc = func() (*config.Config, error) {
		return &config.Config{
			Provider:             config.ProviderOpenAI,
			Model:                "gpt-5.4",
			APIKey:               "test-key",
			Timeout:              time.Minute,
			TeamMode:             "on",
			DisableDelegate:      true,
			AutoContextMaxTokens: 100,
			AutoContextKeepLastN: 2,
		}, nil
	}
	prepareRuntimeFunc = func(context.Context, *config.Config, string) (agent.RuntimeState, error) {
		return agent.RuntimeState{
			EffectiveTeamMode: false,
			TeamModeReason:    "delegate disabled",
			RouterModelName:   "gpt-5.4",
			CodeModeStatus:    "off",
			OpenImageStatus:   "on",
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"runtime"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run() code=%d err=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "**Validation warnings**") {
		t.Fatalf("missing validation warnings\n%s", out.String())
	}
	if !strings.Contains(out.String(), "delegate is disabled") {
		t.Fatalf("missing delegate warning\n%s", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
}
