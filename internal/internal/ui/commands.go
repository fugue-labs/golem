package ui

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/ui/chat"
	"github.com/fugue-labs/gollem/core"
)

func (m *Model) renderHelpMessage() *chat.Message {
	var b strings.Builder
	b.WriteString("**Commands**\n\n")
	b.WriteString("- `/help` — show available commands\n")
	b.WriteString("- `/clear` — clear the current transcript\n")
	b.WriteString("- `/plan` — summarize the current tracked plan\n")
	b.WriteString("- `/invariants` — summarize the tracked invariant checklist\n")
	b.WriteString("- `/runtime` — show the effective runtime profile\n")
	b.WriteString("- `/verify` — show the latest verification summary\n")
	b.WriteString("- `/compact` — compress conversation context\n")
	b.WriteString("- `/cost` — show session cost breakdown\n")
	b.WriteString("- `/resume` — restore the last saved session\n")
	b.WriteString("- `/model [name]` — show or switch the active model\n")
	b.WriteString("- `/diff` — show git diff of uncommitted changes\n")
	b.WriteString("- `/undo [path]` — revert one unstaged git-tracked file change\n")
	b.WriteString("- `/doctor` — diagnose setup issues\n")
	b.WriteString("- `/config` — show effective configuration\n")
	b.WriteString("- `/skills` — list detected skills\n")
	b.WriteString("- `/skill <name>` — toggle a skill on or off\n")
	b.WriteString("- `/quit` or `/exit` — quit the app\n\n")
	b.WriteString("**Keys**\n\n")
	b.WriteString("- `Enter` — send\n")
	b.WriteString("- `Shift+Enter` — insert newline\n")
	b.WriteString("- `Esc` — cancel the active run\n")
	b.WriteString("- `↑/↓` — recall input history\n")
	b.WriteString("- `PgUp/PgDn` — scroll the transcript\n")
	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

func (m *Model) renderPlanSummaryMessage() *chat.Message {
	completed, total := m.planState.Progress()
	if total == 0 {
		return &chat.Message{
			Kind:    chat.KindAssistant,
			Content: "No active plan yet. The planning tool will populate this once the agent creates one.",
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "**Current plan** — %d/%d completed\n\n", completed, total)
	for _, task := range m.planState.Tasks {
		icon := "○"
		switch task.Status {
		case "completed":
			icon = "✓"
		case "in_progress":
			icon = "◐"
		case "blocked":
			icon = "✗"
		}
		fmt.Fprintf(&b, "- %s `%s` — %s", icon, task.ID, task.Description)
		if task.Notes != "" {
			b.WriteString(" — ")
			b.WriteString(task.Notes)
		}
		b.WriteString("\n")
	}

	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

func (m *Model) renderInvariantSummaryMessage() *chat.Message {
	hardTotal, hardPass, hardFail, hardUnresolved, softTotal, softPass, softFail := m.invariantState.Counts()
	if hardTotal == 0 && softTotal == 0 && !m.invariantState.Extracted {
		return &chat.Message{
			Kind:    chat.KindAssistant,
			Content: "No invariant checklist yet. The invariants tool will populate this once the agent extracts constraints.",
		}
	}

	var b strings.Builder
	b.WriteString("**Invariant checklist**\n\n")
	fmt.Fprintf(&b, "- Hard: %d pass, %d fail, %d unresolved (%d total)\n", hardPass, hardFail, hardUnresolved, hardTotal)
	fmt.Fprintf(&b, "- Soft: %d pass, %d fail (%d total)\n\n", softPass, softFail, softTotal)
	for _, item := range m.invariantState.Items {
		kind := item.Kind
		if kind == "" {
			kind = "hard"
		}
		status := item.Status
		if status == "" {
			status = "unknown"
		}
		fmt.Fprintf(&b, "- `%s` [%s/%s] %s", item.ID, kind, status, item.Description)
		if item.Evidence != "" {
			b.WriteString(" — ")
			b.WriteString(item.Evidence)
		}
		b.WriteString("\n")
	}

	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

func (m *Model) renderRuntimeSummaryMessage() *chat.Message {
	report := agent.BuildRuntimeReport(m.cfg, m.runtime, m.cfg.Validate(), nil)
	return &chat.Message{Kind: chat.KindAssistant, Content: agent.RenderRuntimeSummary(report)}
}

func (m *Model) renderCostSummaryMessage() *chat.Message {
	total := m.costTracker.TotalCost()
	if total == 0 && m.sessionUsage.Requests == 0 {
		return &chat.Message{
			Kind:    chat.KindAssistant,
			Content: "No usage recorded yet.",
		}
	}

	var b strings.Builder
	b.WriteString("**Session cost summary**\n\n")

	// Token usage.
	fmt.Fprintf(&b, "- Requests: %d\n", m.sessionUsage.Requests)
	fmt.Fprintf(&b, "- Input tokens: %d\n", m.sessionUsage.InputTokens)
	fmt.Fprintf(&b, "- Output tokens: %d\n", m.sessionUsage.OutputTokens)
	if m.sessionUsage.CacheReadTokens > 0 {
		fmt.Fprintf(&b, "- Cache read tokens: %d\n", m.sessionUsage.CacheReadTokens)
	}
	if m.sessionUsage.CacheWriteTokens > 0 {
		fmt.Fprintf(&b, "- Cache write tokens: %d\n", m.sessionUsage.CacheWriteTokens)
	}
	fmt.Fprintf(&b, "- Tool calls: %d\n\n", m.sessionUsage.ToolCalls)

	// Cost breakdown.
	breakdown := m.costTracker.CostBreakdown()
	if len(breakdown) > 0 {
		b.WriteString("**Cost breakdown**\n\n")
		models := make([]string, 0, len(breakdown))
		for model := range breakdown {
			models = append(models, model)
		}
		sort.Strings(models)
		for _, model := range models {
			fmt.Fprintf(&b, "- `%s`: $%.4f\n", model, breakdown[model])
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "**Total: $%.4f**\n", total)
	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

func (m *Model) handleModelCommand(text string) *chat.Message {
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/model"))
	if arg == "" {
		var b strings.Builder
		fmt.Fprintf(&b, "Current model: `%s` (provider: `%s`)\n\n", m.cfg.Model, m.cfg.Provider)
		if models := knownModels(m.cfg.Provider); len(models) > 0 {
			b.WriteString("Available models:\n")
			for _, name := range models {
				marker := " "
				if name == m.cfg.Model {
					marker = ">"
				}
				fmt.Fprintf(&b, "%s `%s`\n", marker, name)
			}
		}
		return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
	}
	if m.busy {
		return &chat.Message{Kind: chat.KindAssistant, Content: "Cannot switch models while agent is running."}
	}
	old := m.cfg.Model
	m.cfg.Model = arg
	m.costTracker = core.NewCostTracker(modelPricing())
	return &chat.Message{
		Kind:    chat.KindAssistant,
		Content: fmt.Sprintf("Switched model from `%s` to `%s`. Next run will use the new model.", old, arg),
	}
}

func (m *Model) renderDiffMessage() *chat.Message {
	dir := m.cfg.WorkingDir
	if dir == "" {
		dir = "."
	}
	cmd := exec.Command("git", "diff", "--stat", "--no-color")
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("git diff failed: %v\n%s", err, out.String())}
	}
	diff := strings.TrimSpace(out.String())
	if diff == "" {
		cmd2 := exec.Command("git", "diff", "--cached", "--stat", "--no-color")
		cmd2.Dir = dir
		var out2 bytes.Buffer
		cmd2.Stdout = &out2
		cmd2.Stderr = &out2
		if err := cmd2.Run(); err == nil {
			diff = strings.TrimSpace(out2.String())
		}
	}
	if diff == "" {
		return &chat.Message{Kind: chat.KindAssistant, Content: "No uncommitted changes."}
	}

	cmd3 := exec.Command("git", "diff", "--no-color")
	cmd3.Dir = dir
	var out3 bytes.Buffer
	cmd3.Stdout = &out3
	cmd3.Stderr = &out3
	_ = cmd3.Run()
	fullDiff := out3.String()
	if len(fullDiff) > 3000 {
		fullDiff = fullDiff[:3000] + "\n... (truncated)"
	}

	var b strings.Builder
	b.WriteString("**Git diff**\n\n")
	b.WriteString("```\n")
	b.WriteString(diff)
	b.WriteString("\n```\n")
	if fullDiff != "" {
		b.WriteString("\n```diff\n")
		b.WriteString(fullDiff)
		b.WriteString("\n```\n")
	}
	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

func (m *Model) handleUndo(text string) *chat.Message {
	dir := m.cfg.WorkingDir
	if dir == "" {
		dir = "."
	}
	if m.busy {
		return &chat.Message{Kind: chat.KindAssistant, Content: "Cannot undo while agent is running."}
	}

	pathArg := strings.TrimSpace(strings.TrimPrefix(text, "/undo"))
	if pathArg == "" {
		return &chat.Message{Kind: chat.KindAssistant, Content: "Usage: `/undo <path>` reverts unstaged changes for one tracked file."}
	}
	if filepath.IsAbs(pathArg) {
		return &chat.Message{Kind: chat.KindError, Content: "`/undo` requires a repo-relative path."}
	}
	cleanPath := filepath.Clean(pathArg)
	if cleanPath == "." || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return &chat.Message{Kind: chat.KindError, Content: "`/undo` only accepts paths inside the repository."}
	}

	tracked := exec.Command("git", "ls-files", "--error-unmatch", "--", cleanPath)
	tracked.Dir = dir
	var trackedOut bytes.Buffer
	tracked.Stdout = &trackedOut
	tracked.Stderr = &trackedOut
	if err := tracked.Run(); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("%q is not a tracked file. Only tracked files can be undone.", cleanPath)}
	}

	cmd := exec.Command("git", "diff", "--name-only", "--", cleanPath)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("git diff failed: %v\n%s", err, out.String())}
	}
	if strings.TrimSpace(out.String()) == "" {
		return &chat.Message{Kind: chat.KindAssistant, Content: fmt.Sprintf("No unstaged changes to undo for `%s`.", cleanPath)}
	}

	restore := exec.Command("git", "restore", "--worktree", "--", cleanPath)
	restore.Dir = dir
	var restoreOut bytes.Buffer
	restore.Stdout = &restoreOut
	restore.Stderr = &restoreOut
	if err := restore.Run(); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("git restore failed: %v\n%s", err, restoreOut.String())}
	}

	return &chat.Message{
		Kind:    chat.KindAssistant,
		Content: fmt.Sprintf("Reverted unstaged changes in `%s`.", cleanPath),
	}
}

func knownModels(provider config.Provider) []string {
	switch provider {
	case config.ProviderAnthropic:
		return []string{"claude-sonnet-4-20250514", "claude-opus-4-20250514", "claude-haiku-3.5"}
	case config.ProviderOpenAI:
		return []string{"gpt-5.4", "gpt-4o", "gpt-4o-mini", "o3", "o4-mini"}
	case config.ProviderOpenAICompatible:
		return []string{"grok-3", "grok-4-0709", "grok-code-fast-1"}
	case config.ProviderVertexAI:
		return []string{"gemini-2.5-pro", "gemini-2.5-flash"}
	case config.ProviderVertexAnthropic:
		return []string{"claude-sonnet-4-5", "claude-opus-4"}
	default:
		return nil
	}
}

func (m *Model) renderDoctorMessage() *chat.Message {
	var b strings.Builder
	b.WriteString("**Golem Doctor**\n\n")

	// Provider & auth check.
	validation := m.cfg.Validate()
	if validation.HasErrors() {
		for _, e := range validation.Errors {
			fmt.Fprintf(&b, "- **ERROR**: %s\n", e)
		}
	} else {
		fmt.Fprintf(&b, "- Provider: `%s` — OK\n", m.cfg.Provider)
		fmt.Fprintf(&b, "- Model: `%s`\n", m.cfg.Model)
		authStatus := "configured"
		if m.cfg.ChatGPTCreds != nil {
			authStatus = "ChatGPT subscription"
		} else if m.cfg.APIKey != "" {
			authStatus = "API key"
		}
		fmt.Fprintf(&b, "- Auth: %s\n", authStatus)
	}
	for _, w := range validation.Warnings {
		fmt.Fprintf(&b, "- **WARN**: %s\n", w)
	}

	// Git check.
	if m.runtime.Git != nil && m.runtime.Git.IsRepo {
		fmt.Fprintf(&b, "- Git: `%s` — OK\n", m.runtime.Git.BranchDisplay())
	} else {
		b.WriteString("- Git: not a git repository\n")
	}

	// Working dir.
	fmt.Fprintf(&b, "- Working dir: `%s`\n", m.cfg.WorkingDir)

	// Instructions check.
	if len(m.runtime.Instructions) > 0 {
		fmt.Fprintf(&b, "- Instructions: %d file(s) loaded\n", len(m.runtime.Instructions))
	} else {
		b.WriteString("- Instructions: none (create GOLEM.md for project-specific guidance)\n")
	}

	// MCP check.
	if len(m.runtime.MCPServers) > 0 {
		fmt.Fprintf(&b, "- MCP servers: %s\n", strings.Join(m.runtime.MCPServers, ", "))
	} else {
		b.WriteString("- MCP: no servers configured\n")
	}

	// Permission mode.
	fmt.Fprintf(&b, "- Permission mode: `%s`\n", m.cfg.PermissionMode)

	// Tool checks.
	toolChecks := []struct {
		name string
		cmd  string
	}{
		{"git", "git --version"},
		{"node", "node --version"},
		{"python3", "python3 --version"},
	}
	b.WriteString("\n**Tool checks**\n\n")
	for _, tc := range toolChecks {
		parts := strings.Fields(tc.cmd)
		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Dir = m.cfg.WorkingDir
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(&b, "- `%s`: not found\n", tc.name)
		} else {
			ver := strings.TrimSpace(out.String())
			if len(ver) > 50 {
				ver = ver[:50]
			}
			fmt.Fprintf(&b, "- `%s`: %s\n", tc.name, ver)
		}
	}

	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

func (m *Model) renderConfigMessage() *chat.Message {
	var b strings.Builder
	b.WriteString("**Effective configuration**\n\n")
	cfg := m.cfg
	fmt.Fprintf(&b, "- Provider: `%s`\n", cfg.Provider)
	fmt.Fprintf(&b, "- Model: `%s`\n", cfg.Model)
	if cfg.BaseURL != "" {
		fmt.Fprintf(&b, "- Base URL: `%s`\n", cfg.BaseURL)
	}
	fmt.Fprintf(&b, "- Timeout: `%s`\n", cfg.Timeout)
	fmt.Fprintf(&b, "- Working dir: `%s`\n", cfg.WorkingDir)
	fmt.Fprintf(&b, "- Permission mode: `%s`\n", cfg.PermissionMode)
	fmt.Fprintf(&b, "- Team mode: `%s`\n", cfg.TeamMode)
	if cfg.RouterModel != "" {
		fmt.Fprintf(&b, "- Router model: `%s`\n", cfg.RouterModel)
	}
	if cfg.ReasoningEffort != "" {
		fmt.Fprintf(&b, "- Reasoning effort: `%s`\n", cfg.ReasoningEffort)
	}
	if cfg.ThinkingBudget > 0 {
		fmt.Fprintf(&b, "- Thinking budget: `%d`\n", cfg.ThinkingBudget)
	}
	if cfg.AutoContextMaxTokens > 0 {
		fmt.Fprintf(&b, "- Auto-context: `%d` tokens, keep last `%d` turns\n", cfg.AutoContextMaxTokens, cfg.AutoContextKeepLastN)
	}
	fmt.Fprintf(&b, "- Top-level personality: `%t`\n", cfg.TopLevelPersonality)
	fmt.Fprintf(&b, "- Disable delegate: `%t`\n", cfg.DisableDelegate)
	fmt.Fprintf(&b, "- Disable code mode: `%t`\n", cfg.DisableCodeMode)

	b.WriteString("\n**Environment**\n\n")
	envVars := []string{"GOLEM_PROVIDER", "GOLEM_MODEL", "GOLEM_API_KEY", "GOLEM_TIMEOUT", "GOLEM_TEAM_MODE", "GOLEM_PERMISSION_MODE", "GOLEM_MCP_SERVERS"}
	for _, env := range envVars {
		val := os.Getenv(env)
		if val == "" {
			continue
		}
		display := val
		if strings.Contains(strings.ToLower(env), "key") || strings.Contains(strings.ToLower(env), "secret") {
			if len(display) > 8 {
				display = display[:4] + "..." + display[len(display)-4:]
			}
		}
		fmt.Fprintf(&b, "- `%s`: `%s`\n", env, display)
	}

	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

func (m *Model) renderVerificationSummaryMessage() *chat.Message {
	if !m.verificationState.HasEntries() {
		return &chat.Message{
			Kind:    chat.KindAssistant,
			Content: "No verification results tracked yet. The verification tool will populate this once the agent records test/build outcomes.",
		}
	}

	total, pass, fail, stale, inProgress := m.verificationState.Counts()
	var b strings.Builder
	b.WriteString("**Verification summary**\n\n")
	fmt.Fprintf(&b, "- Total: %d, Pass: %d, Fail: %d, In progress: %d, Stale: %d\n\n", total, pass, fail, inProgress, stale)
	for _, entry := range m.verificationState.Entries {
		icon := "○"
		switch entry.Status {
		case "pass":
			icon = "✓"
		case "fail":
			icon = "✗"
		case "in_progress":
			icon = "◐"
		}
		freshLabel := ""
		if entry.Freshness == "stale" {
			freshLabel = " [stale"
			if entry.StaleBy != "" {
				freshLabel += ": " + entry.StaleBy
			}
			freshLabel += "]"
		}
		fmt.Fprintf(&b, "- %s `%s` — `%s`%s", icon, entry.ID, entry.Command, freshLabel)
		if entry.Summary != "" {
			b.WriteString(" — ")
			b.WriteString(entry.Summary)
		}
		b.WriteString("\n")
	}
	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}
