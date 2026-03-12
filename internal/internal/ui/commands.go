package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/ui/chat"
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
	b.WriteString("- `/undo` — revert the last git-tracked file change\n")
	b.WriteString("- `/doctor` — diagnose setup issues\n")
	b.WriteString("- `/skills` — list detected skills\n")
	b.WriteString("- `/skill <name>` — toggle a skill on or off\n")
	b.WriteString("- `/quit` or `/exit` — quit the app\n\n")
	b.WriteString("**Keys**\n\n")
	b.WriteString("- `Enter` — send\n")
	b.WriteString("- `Shift+Enter` — insert newline\n")
	b.WriteString("- `Esc` — cancel the active run\n")
	b.WriteString("- `↑/↓` and `PgUp/PgDn` — scroll the transcript\n")
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
