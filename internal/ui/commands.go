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
