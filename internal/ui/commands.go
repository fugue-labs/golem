package ui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/search"
	"github.com/fugue-labs/golem/internal/ui/chat"
	"github.com/fugue-labs/golem/internal/ui/spec"
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
	b.WriteString("- `/replay [file|list]` — replay the latest trace or inspect saved traces\n")
	b.WriteString("- `/budget` — show budget status and limits\n")
	b.WriteString("- `/resume` — restore the last saved session for this project\n")
	b.WriteString("- `/search <query>` — search across all saved sessions with readable context\n")
	b.WriteString("- `/model [name]` — show or switch the active model\n")
	b.WriteString("- `/diff` — show git diff of uncommitted changes\n")
	b.WriteString("- `/undo [path]` — revert one unstaged git-tracked file change\n")
	b.WriteString("- `/mission [new|status|tasks|plan|approve|start|pause|cancel|list]` — mission orchestration\n")
	b.WriteString("- `/rewind [N]` — rewind to turn N (or list checkpoints)\n")
	b.WriteString("- `/doctor` — diagnose setup issues\n")
	b.WriteString("- `/config` — show effective configuration\n")
	b.WriteString("- `/team` — show team member status\n")
	b.WriteString("- `/context` — show context window usage\n")
	b.WriteString("- `/skills` — list detected skills\n")
	b.WriteString("- `/skill <name>` — toggle a skill on or off\n")
	b.WriteString("- `/spec [file]` — start or show spec-driven development\n")
	b.WriteString("- `/quit` or `/exit` — quit the app\n\n")
	b.WriteString("**Keys**\n\n")
	b.WriteString("- `Enter` — send\n")
	b.WriteString("- `Shift+Enter` — insert newline\n")
	b.WriteString("- `Tab` — autocomplete slash commands\n")
	b.WriteString("- `Esc` — cancel the active run\n")
	b.WriteString("- `Ctrl+L` — clear transcript\n")
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

	// Budget summary if configured.
	if budget := m.cfg.EffectiveBudget(); budget > 0 {
		remaining := budget - total
		if remaining < 0 {
			remaining = 0
		}
		pct := total / budget * 100
		fmt.Fprintf(&b, "\n**Budget: $%.2f** — %.1f%% used, $%.4f remaining\n", budget, pct, remaining)
		if m.downgraded && m.originalModel != "" {
			fmt.Fprintf(&b, "Model downgraded: `%s` → `%s`\n", m.originalModel, m.cfg.Model)
		}
	}

	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

func (m *Model) renderBudgetMessage() *chat.Message {
	var b strings.Builder
	b.WriteString("**Budget status**\n\n")

	cost := m.costTracker.TotalCost()
	sessionBudget := m.cfg.SessionBudget
	projectBudget := m.cfg.ProjectBudget
	effectiveBudget := m.cfg.EffectiveBudget()

	fmt.Fprintf(&b, "- Current cost: $%.4f\n", cost)

	if sessionBudget > 0 {
		pct := cost / sessionBudget * 100
		fmt.Fprintf(&b, "- Session budget: $%.2f (%.1f%% used)\n", sessionBudget, pct)
	} else {
		b.WriteString("- Session budget: unlimited\n")
	}

	if projectBudget > 0 {
		pct := cost / projectBudget * 100
		fmt.Fprintf(&b, "- Project budget: $%.2f (%.1f%% used)\n", projectBudget, pct)
	} else {
		b.WriteString("- Project budget: unlimited\n")
	}

	if effectiveBudget > 0 {
		remaining := effectiveBudget - cost
		if remaining < 0 {
			remaining = 0
		}
		fmt.Fprintf(&b, "- Remaining: $%.4f\n", remaining)
		fmt.Fprintf(&b, "- Warning threshold: %.0f%%\n", m.cfg.BudgetWarnPct*100)
	}

	fmt.Fprintf(&b, "- Current model: `%s`\n", m.cfg.Model)
	if m.downgraded && m.originalModel != "" {
		fmt.Fprintf(&b, "- Original model: `%s` (downgraded for budget)\n", m.originalModel)
	}

	if fallback := config.CheaperModel(m.cfg.Provider, m.cfg.Model); fallback != "" {
		fmt.Fprintf(&b, "- Next fallback: `%s`\n", fallback)
	} else if !m.downgraded {
		b.WriteString("- Next fallback: none (already cheapest)\n")
	}

	if m.cfg.FallbackModel != "" {
		fmt.Fprintf(&b, "- Configured fallback: `%s`\n", m.cfg.FallbackModel)
	}

	b.WriteString("\n**Configuration**\n\n")
	b.WriteString("Set in `~/.golem/config.json`:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString("  \"session_budget\": 1.00,\n")
	b.WriteString("  \"project_budget\": 10.00,\n")
	b.WriteString("  \"budget_warn_pct\": 0.8,\n")
	b.WriteString("  \"fallback_model\": \"claude-haiku-4-5\"\n")
	b.WriteString("}\n")
	b.WriteString("```\n\n")
	b.WriteString("Or via env vars: `GOLEM_SESSION_BUDGET`, `GOLEM_PROJECT_BUDGET`, `GOLEM_BUDGET_WARN_PCT`, `GOLEM_FALLBACK_MODEL`\n")

	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

func (m *Model) handleModelCommand(text string) *chat.Message {
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/model"))
	if arg == "" {
		var b strings.Builder
		fmt.Fprintf(&b, "Current model: `%s` (provider: `%s`)\n\n", m.cfg.Model, m.cfg.Provider)

		// Show model routing status.
		rc := m.runtime.RoutingConfig
		if agent.IsRoutingEnabled(m.cfg, rc) {
			fastModel := agent.ResolveFastModel(m.cfg, rc)
			fmt.Fprintf(&b, "Model routing: on\n")
			fmt.Fprintf(&b, "  Fast model:   `%s`\n", fastModel)
			fmt.Fprintf(&b, "  Strong model: `%s`\n", m.cfg.Model)
			if m.runtime.RoutedModel != "" {
				fmt.Fprintf(&b, "  Last routed:  `%s` (%s)\n", m.runtime.RoutedModel, m.runtime.RoutingReason)
			}
			b.WriteString("\n")
		}

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

	// Parse optional file path: /undo [path]
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/undo"))

	if arg != "" {
		// Undo a specific file.
		cmd := exec.Command("git", "diff", "--name-only", "--", arg)
		cmd.Dir = dir
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("git failed: %v", err)}
		}
		if strings.TrimSpace(out.String()) == "" {
			return &chat.Message{Kind: chat.KindAssistant, Content: fmt.Sprintf("No unstaged changes in `%s`.", arg)}
		}
		cmd2 := exec.Command("git", "checkout", "--", arg)
		cmd2.Dir = dir
		var out2 bytes.Buffer
		cmd2.Stdout = &out2
		cmd2.Stderr = &out2
		if err := cmd2.Run(); err != nil {
			return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("git checkout failed: %v\n%s", err, out2.String())}
		}
		return &chat.Message{Kind: chat.KindAssistant, Content: fmt.Sprintf("Reverted `%s` to last committed state.", arg)}
	}

	// Undo all unstaged changes.
	cmd := exec.Command("git", "diff", "--name-only")
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("git failed: %v", err)}
	}
	files := strings.TrimSpace(out.String())
	if files == "" {
		return &chat.Message{Kind: chat.KindAssistant, Content: "No uncommitted changes to undo."}
	}

	cmd2 := exec.Command("git", "checkout", "--", ".")
	cmd2.Dir = dir
	var out2 bytes.Buffer
	cmd2.Stdout = &out2
	cmd2.Stderr = &out2
	if err := cmd2.Run(); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("git checkout failed: %v\n%s", err, out2.String())}
	}

	count := len(strings.Split(files, "\n"))
	return &chat.Message{
		Kind:    chat.KindAssistant,
		Content: fmt.Sprintf("Reverted %d file(s) to last committed state:\n```\n%s\n```", count, files),
	}
}

func (m *Model) handleRewind(text string) *chat.Message {
	if m.busy {
		return &chat.Message{Kind: chat.KindAssistant, Content: "Cannot rewind while agent is running."}
	}

	arg := strings.TrimSpace(strings.TrimPrefix(text, "/rewind"))

	// No argument: list available checkpoints.
	if arg == "" {
		if m.checkpoints.Len() == 0 {
			return &chat.Message{
				Kind:    chat.KindAssistant,
				Content: "No checkpoints yet. Checkpoints are created after each agent turn.",
			}
		}
		var b strings.Builder
		b.WriteString("**Checkpoints**\n\n")
		for _, summary := range m.checkpoints.List() {
			fmt.Fprintf(&b, "- %s\n", summary)
		}
		b.WriteString("\nUse `/rewind N` to restore turn N.")
		return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
	}

	// Parse turn number.
	turn, err := strconv.Atoi(arg)
	if err != nil {
		return &chat.Message{
			Kind:    chat.KindError,
			Content: fmt.Sprintf("Invalid turn number: %q. Use `/rewind` to list checkpoints.", arg),
		}
	}

	cp, err := m.checkpoints.RewindTo(turn)
	if err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Rewind failed: %v", err)}
	}

	// Restore all session state from the checkpoint.
	m.history = cp.History
	m.toolState = cp.ToolState
	m.planState = cp.PlanState
	m.invariantState = cp.InvariantState
	m.verificationState = cp.VerificationState
	m.sessionUsage = cp.SessionUsage
	m.lastCost = cp.LastCost
	m.turnCount = cp.Turn
	m.lastPrompt = cp.Prompt

	// Restore messages to the checkpoint state, then append a rewind confirmation.
	m.messages = cp.Messages

	return &chat.Message{
		Kind:    chat.KindAssistant,
		Content: fmt.Sprintf("Rewound to turn %d. Conversation, tool state, plan, invariants, verification, and files restored.", turn),
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

func (m *Model) handleSearchCommand(text string) *chat.Message {
	query := strings.TrimSpace(strings.TrimPrefix(text, "/search"))
	if query == "" {
		return &chat.Message{
			Kind:    chat.KindAssistant,
			Content: "Usage: `/search <query>` — search across all saved sessions with readable context.\n\nExamples:\n- `/search flaky test fix`\n- `/search database migration`\n- `/search authentication bug`\n\nResults include the saved prompt, model/time summary, project, and nearby transcript or workflow state when available.",
		}
	}

	results, err := search.SearchSessions(query, "", 10)
	if err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Search failed: %v", err)}
	}

	if len(results) == 0 {
		return &chat.Message{
			Kind:    chat.KindAssistant,
			Content: fmt.Sprintf("No sessions found matching %q. Saved sessions become searchable after successful runs and keep transcript/workflow context when available.", query),
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**Search results for** %q — %d match(es)\n\n", query, len(results))
	for i, r := range results {
		title := strings.TrimSpace(r.Prompt)
		if title == "" {
			if r.Model != "" {
				title = fmt.Sprintf("Saved session with %s", r.Model)
			} else {
				title = fmt.Sprintf("Session from %s", r.Timestamp.Format("Jan 2, 2006 15:04"))
			}
		}
		if len([]rune(title)) > 120 {
			title = string([]rune(title)[:119]) + "…"
		}
		fmt.Fprintf(&sb, "**%d.** %s\n", i+1, title)

		metaParts := make([]string, 0, 4)
		if r.Summary != "" {
			metaParts = append(metaParts, r.Summary)
		} else if !r.Timestamp.IsZero() {
			metaParts = append(metaParts, r.Timestamp.Format("Jan 2, 2006 15:04"))
		}
		if r.ProjectDir != "" {
			metaParts = append(metaParts, fmt.Sprintf("`%s`", r.ProjectDir))
		}
		metaParts = append(metaParts, fmt.Sprintf("score %.1f", r.Score))
		fmt.Fprintf(&sb, "   %s\n", strings.Join(metaParts, " · "))

		if r.Snippet != "" {
			snippet := r.Snippet
			if len([]rune(snippet)) > 320 {
				snippet = string([]rune(snippet)[:319]) + "…"
			}
			for _, line := range strings.Split(snippet, "\n") {
				fmt.Fprintf(&sb, "   > %s\n", line)
			}
		}
		sb.WriteString("\n")
	}
	return &chat.Message{Kind: chat.KindAssistant, Content: sb.String()}
}

func (m *Model) resumeSession() *chat.Message {
	if m.busy {
		return &chat.Message{Kind: chat.KindAssistant, Content: "Cannot resume while agent is running."}
	}
	if m.sessionHasVisibleState() {
		return &chat.Message{Kind: chat.KindAssistant, Content: "Session already has visible state. Use `/clear` first to resume a previous session."}
	}
	session, err := agent.LoadLatestSession(m.cfg.WorkingDir)
	if err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to load session: %v", err)}
	}
	if session == nil {
		return &chat.Message{Kind: chat.KindAssistant, Content: "No previous session found for this project."}
	}
	msgs, err := session.RestoreMessages()
	if err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to restore messages: %v", err)}
	}
	if err := m.restoreSessionState(session, msgs); err != nil {
		return &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to restore session state: %v", err)}
	}
	summary := session.Summary()
	content := fmt.Sprintf("Resumed session from %s (%d messages, %d requests).", session.Timestamp.Format("Jan 2 15:04"), len(msgs), session.Usage.Requests)
	if summary.PromptPreview != "" {
		content += fmt.Sprintf(" Prompt: %q.", summary.PromptPreview)
	}
	content += fmt.Sprintf(" Restored %s.", summary.RestorableStateDescription())
	return &chat.Message{Kind: chat.KindAssistant, Content: content}
}

func (m *Model) sessionHasVisibleState() bool {
	if len(m.messages) > 0 || len(m.history) > 0 {
		return true
	}
	if strings.TrimSpace(m.lastPrompt) != "" {
		return true
	}
	if m.sessionUsage.Requests > 0 || m.sessionUsage.InputTokens > 0 || m.sessionUsage.OutputTokens > 0 || m.sessionUsage.CacheReadTokens > 0 || m.sessionUsage.CacheWriteTokens > 0 || m.sessionUsage.ToolCalls > 0 {
		return true
	}
	if m.planState.HasTasks() || m.invariantState.HasItems() || m.verificationState.HasEntries() || m.specState.IsActive() {
		return true
	}
	return false
}

func summarizePromptPreview(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "(no saved prompt)"
	}
	runes := []rune(prompt)
	if len(runes) <= 80 {
		return prompt
	}
	return string(runes[:79]) + "…"
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
	if cfg.SessionBudget > 0 {
		fmt.Fprintf(&b, "- Session budget: `$%.2f`\n", cfg.SessionBudget)
	}
	if cfg.ProjectBudget > 0 {
		fmt.Fprintf(&b, "- Project budget: `$%.2f`\n", cfg.ProjectBudget)
	}
	if cfg.FallbackModel != "" {
		fmt.Fprintf(&b, "- Fallback model: `%s`\n", cfg.FallbackModel)
	}
	fmt.Fprintf(&b, "- Top-level personality: `%t`\n", cfg.TopLevelPersonality)
	fmt.Fprintf(&b, "- Disable delegate: `%t`\n", cfg.DisableDelegate)
	fmt.Fprintf(&b, "- Disable code mode: `%t`\n", cfg.DisableCodeMode)

	b.WriteString("\n**Environment**\n\n")
	envVars := []string{"GOLEM_PROVIDER", "GOLEM_MODEL", "GOLEM_API_KEY", "GOLEM_TIMEOUT", "GOLEM_TEAM_MODE", "GOLEM_PERMISSION_MODE", "GOLEM_MCP_SERVERS", "GOLEM_SESSION_BUDGET", "GOLEM_PROJECT_BUDGET", "GOLEM_FALLBACK_MODEL"}
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

func (m *Model) renderTeamMessage() *chat.Message {
	session := m.runtime.Session
	if session == nil || session.Team == nil {
		return &chat.Message{
			Kind:    chat.KindAssistant,
			Content: "No team active. Set `GOLEM_TEAM_MODE=auto` to enable team mode.",
		}
	}
	members := session.Team.Members()
	if len(members) <= 1 {
		return &chat.Message{
			Kind:    chat.KindAssistant,
			Content: "Team mode enabled but no teammates spawned yet.",
		}
	}

	running, idle, stopped := 0, 0, 0
	for _, mi := range members {
		switch mi.State.String() {
		case "running":
			running++
		case "idle":
			idle++
		case "stopped":
			stopped++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "**Team** — %d running, %d idle, %d stopped\n\n", running, idle, stopped)
	for _, mi := range members {
		icon := "○"
		switch mi.State.String() {
		case "running":
			icon = "◐"
		case "idle":
			icon = "✓"
		case "stopped":
			icon = "×"
		case "starting":
			icon = "●"
		}
		fmt.Fprintf(&b, "- %s `%s` — %s\n", icon, mi.Name, mi.State.String())
	}
	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}

func (m *Model) renderContextMessage() *chat.Message {
	var b strings.Builder
	b.WriteString("**Context window**\n\n")

	ctxWindow := modelContextWindow(m.cfg.Model)
	tokenCount := m.estimatedTokens
	if tokenCount == 0 && m.usage.InputTokens > 0 {
		tokenCount = m.usage.InputTokens
	}

	fmt.Fprintf(&b, "- Model: `%s`\n", m.cfg.Model)
	if ctxWindow > 0 {
		fmt.Fprintf(&b, "- Window: %dk tokens\n", ctxWindow/1000)
	}
	if tokenCount > 0 {
		fmt.Fprintf(&b, "- Estimated usage: ~%dk tokens", tokenCount/1000)
		if ctxWindow > 0 {
			pct := tokenCount * 100 / ctxWindow
			fmt.Fprintf(&b, " (%d%%)", pct)
		}
		b.WriteString("\n")
	} else {
		b.WriteString("- Estimated usage: no data yet\n")
	}

	if m.cfg.AutoContextMaxTokens > 0 {
		fmt.Fprintf(&b, "- Auto-compact: triggers at %dk tokens, keeps last %d turns\n",
			m.cfg.AutoContextMaxTokens/1000, m.cfg.AutoContextKeepLastN)
	} else {
		b.WriteString("- Auto-compact: disabled\n")
	}

	fmt.Fprintf(&b, "- Messages: %d in transcript\n", len(m.messages))
	fmt.Fprintf(&b, "- Requests: %d\n", m.sessionUsage.Requests)

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

func (m *Model) handleSpecCommand(text string) (tea.Model, tea.Cmd) {
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/spec"))

	// No argument: show current spec status.
	if arg == "" {
		m.messages = append(m.messages, m.renderSpecSummaryMessage())
		m.scroll = 0
		return m, m.input.Focus()
	}

	if m.busy {
		m.messages = append(m.messages, &chat.Message{
			Kind:    chat.KindAssistant,
			Content: "Cannot start a spec workflow while agent is running.",
		})
		m.scroll = 0
		return m, m.input.Focus()
	}

	// Resolve file path.
	specPath := arg
	if !filepath.IsAbs(specPath) {
		dir := m.cfg.WorkingDir
		if dir == "" {
			dir = "."
		}
		specPath = filepath.Join(dir, specPath)
	}

	content, err := os.ReadFile(specPath)
	if err != nil {
		m.messages = append(m.messages, &chat.Message{
			Kind:    chat.KindError,
			Content: fmt.Sprintf("Failed to read spec file: %v", err),
		})
		m.scroll = 0
		return m, m.input.Focus()
	}

	// Initialize spec state.
	m.specState = spec.New(specPath)

	// Build a prompt that drives the spec workflow.
	prompt := buildSpecPrompt(specPath, string(content))

	// Submit the spec as a user message to kick off the agent workflow.
	userMsg := &chat.Message{Kind: chat.KindUser, Content: fmt.Sprintf("/spec %s", arg)}
	m.messages = append(m.messages, userMsg)
	m.inputHistory = append(m.inputHistory, text)
	m.busy = true
	m.startTime = time.Now()
	m.lastPrompt = prompt
	m.currentRunMessages = []*chat.Message{userMsg}
	m.runID++
	m.hookRID.Store(int64(m.runID))
	m.runCtx, m.cancel = context.WithCancel(context.Background())
	m.scroll = 0
	return m, m.prepareRun(prompt)
}

func buildSpecPrompt(filePath, content string) string {
	var b strings.Builder
	b.WriteString("# Spec-Driven Development Mode\n\n")
	b.WriteString("The user has loaded a spec file for implementation. Follow this structured workflow:\n\n")
	b.WriteString("## Spec file\n")
	fmt.Fprintf(&b, "Path: `%s`\n\n", filePath)
	b.WriteString("```\n")
	b.WriteString(content)
	b.WriteString("\n```\n\n")
	b.WriteString("## Workflow (3 gates)\n\n")
	b.WriteString("### Gate 1: Spec Approval\n")
	b.WriteString("1. Read and analyze the spec thoroughly\n")
	b.WriteString("2. Identify any ambiguities, missing requirements, or contradictions\n")
	b.WriteString("3. Extract hard and soft invariants from the spec using the invariants tool\n")
	b.WriteString("4. Present a summary to the user with any questions or concerns\n")
	b.WriteString("5. Ask the user to approve the spec before proceeding\n\n")
	b.WriteString("### Gate 2: Task Decomposition\n")
	b.WriteString("1. Decompose the spec into concrete implementation tasks using the plan tool\n")
	b.WriteString("2. Each task should be atomic and independently verifiable\n")
	b.WriteString("3. Order tasks by dependency (implement foundations first)\n")
	b.WriteString("4. Present the task plan to the user for review\n")
	b.WriteString("5. Ask the user to approve the task plan before implementing\n\n")
	b.WriteString("### Gate 3: Implementation & Final Review\n")
	b.WriteString("1. Implement each task in order, updating the plan as you go\n")
	b.WriteString("2. After all tasks are complete, verify invariants\n")
	b.WriteString("3. Show a diff summary of all changes\n")
	b.WriteString("4. Update the spec file to reflect what was actually built\n")
	b.WriteString("5. Present the final review for user approval\n\n")
	b.WriteString("## Important\n")
	b.WriteString("- Wait for explicit user approval at each gate before proceeding\n")
	b.WriteString("- Use the plan tool for task tracking and the invariants tool for requirements\n")
	b.WriteString("- Keep the spec file as the living source of truth\n")
	b.WriteString("- After implementation, update the spec to reflect what was actually built\n")
	return b.String()
}

func (m *Model) renderSpecSummaryMessage() *chat.Message {
	if !m.specState.IsActive() {
		return &chat.Message{
			Kind:    chat.KindAssistant,
			Content: "No active spec. Use `/spec <file>` to start spec-driven development.",
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "**Spec-driven development** — %s\n\n", m.specState.PhaseLabel())
	fmt.Fprintf(&b, "- File: `%s`\n", m.specState.FilePath)
	fmt.Fprintf(&b, "- Phase: %s\n\n", m.specState.PhaseLabel())

	b.WriteString("**Gates**\n\n")
	for _, g := range m.specState.Gates {
		icon := "○"
		if g.Status == "passed" {
			icon = "✓"
		}
		fmt.Fprintf(&b, "- %s %s — %s\n", icon, g.Name, g.Status)
	}

	if completed, total := m.specState.Progress(); total > 0 {
		fmt.Fprintf(&b, "\n**Tasks**: %d/%d completed\n", completed, total)
	}

	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}
