package ui

import (
	"fmt"
	"strings"

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
	var b strings.Builder
	b.WriteString("**Runtime profile**\n\n")
	fmt.Fprintf(&b, "- Provider/model: `%s/%s`\n", m.cfg.Provider, m.cfg.Model)
	if m.cfg.RouterModel != "" {
		fmt.Fprintf(&b, "- Router model: `%s`\n", m.cfg.RouterModel)
	}
	fmt.Fprintf(&b, "- Timeout: `%s`\n", m.cfg.Timeout)
	fmt.Fprintf(&b, "- Team mode: `%s` (effective: `%t`)\n", m.cfg.TeamMode, m.runtime.EffectiveTeamMode)
	if m.runtime.RouterModelName != "" {
		fmt.Fprintf(&b, "- Effective router model: `%s`\n", m.runtime.RouterModelName)
	}
	if m.runtime.TeamModeReason != "" {
		fmt.Fprintf(&b, "- Team mode reason: %s\n", m.runtime.TeamModeReason)
	}
	if m.cfg.ReasoningEffort != "" {
		fmt.Fprintf(&b, "- Reasoning effort: `%s`\n", m.cfg.ReasoningEffort)
	}
	if m.cfg.ThinkingBudget > 0 {
		fmt.Fprintf(&b, "- Thinking budget: `%d`\n", m.cfg.ThinkingBudget)
	}
	if m.cfg.AutoContextMaxTokens > 0 {
		fmt.Fprintf(&b, "- Auto-context: `%d` tokens, keep last `%d` turns\n", m.cfg.AutoContextMaxTokens, m.cfg.AutoContextKeepLastN)
	}
	fmt.Fprintf(&b, "- Top-level personality: `%t`\n", m.cfg.TopLevelPersonality)
	b.WriteString("\n**Tool surfaces**\n\n")
	b.WriteString("- Guaranteed repo tools: `bash`, `bash_status`, `bash_kill`, `view`, `edit`, `write`, `multi_edit`, `glob`, `grep`, `ls`, `lsp`\n")
	b.WriteString("- Guaranteed workflow tools: `planning`, `invariants`, `verification`\n")
	fmt.Fprintf(&b, "- Delegate: `%t`\n", !m.cfg.DisableDelegate)
	fmt.Fprintf(&b, "- Execute code: `%s`\n", m.runtime.CodeModeStatus)
	if m.runtime.CodeModeError != "" {
		fmt.Fprintf(&b, "- Execute code note: %s\n", m.runtime.CodeModeError)
	}
	if m.runtime.OpenImageStatus != "" {
		fmt.Fprintf(&b, "- Open image: `%s`\n", m.runtime.OpenImageStatus)
	}
	b.WriteString("- Optional environment-dependent tools should only be trusted when surfaced by the runtime/tool list.\n")
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
