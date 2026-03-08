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
	b.WriteString(fmt.Sprintf("**Current plan** — %d/%d completed\n\n", completed, total))
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
		b.WriteString(fmt.Sprintf("- %s `%s` — %s", icon, task.ID, task.Description))
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
	b.WriteString(fmt.Sprintf("- Hard: %d pass, %d fail, %d unresolved (%d total)\n", hardPass, hardFail, hardUnresolved, hardTotal))
	b.WriteString(fmt.Sprintf("- Soft: %d pass, %d fail (%d total)\n\n", softPass, softFail, softTotal))
	for _, item := range m.invariantState.Items {
		kind := item.Kind
		if kind == "" {
			kind = "hard"
		}
		status := item.Status
		if status == "" {
			status = "unknown"
		}
		b.WriteString(fmt.Sprintf("- `%s` [%s/%s] %s", item.ID, kind, status, item.Description))
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
	b.WriteString(fmt.Sprintf("- Provider/model: `%s/%s`\n", m.cfg.Provider, m.cfg.Model))
	if m.cfg.RouterModel != "" {
		b.WriteString(fmt.Sprintf("- Router model: `%s`\n", m.cfg.RouterModel))
	}
	b.WriteString(fmt.Sprintf("- Timeout: `%s`\n", m.cfg.Timeout))
	b.WriteString(fmt.Sprintf("- Team mode: `%s` (effective: `%t`)\n", m.cfg.TeamMode, m.runtime.EffectiveTeamMode))
	if m.runtime.TeamModeReason != "" {
		b.WriteString(fmt.Sprintf("- Team mode reason: %s\n", m.runtime.TeamModeReason))
	}
	b.WriteString(fmt.Sprintf("- Delegate: `%t`\n", !m.cfg.DisableDelegate))
	b.WriteString(fmt.Sprintf("- Code mode: `%s`\n", m.runtime.CodeModeStatus))
	if m.runtime.CodeModeError != "" {
		b.WriteString(fmt.Sprintf("- Code mode note: %s\n", m.runtime.CodeModeError))
	}
	if m.cfg.ReasoningEffort != "" {
		b.WriteString(fmt.Sprintf("- Reasoning effort: `%s`\n", m.cfg.ReasoningEffort))
	}
	if m.cfg.ThinkingBudget > 0 {
		b.WriteString(fmt.Sprintf("- Thinking budget: `%d`\n", m.cfg.ThinkingBudget))
	}
	if m.cfg.AutoContextMaxTokens > 0 {
		b.WriteString(fmt.Sprintf("- Auto-context: `%d` tokens, keep last `%d` turns\n", m.cfg.AutoContextMaxTokens, m.cfg.AutoContextKeepLastN))
	}
	b.WriteString(fmt.Sprintf("- Top-level personality: `%t`\n", m.cfg.TopLevelPersonality))
	return &chat.Message{Kind: chat.KindAssistant, Content: b.String()}
}
