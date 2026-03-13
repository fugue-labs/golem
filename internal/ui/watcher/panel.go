package watcher

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// EventMsg carries a batch of file change events into the bubbletea update loop.
type EventMsg struct {
	Events []Event
}

// ToggleMsg toggles the watcher panel between enabled and disabled.
type ToggleMsg struct{}

// Panel is a bubbletea model that displays recent file-change events from the
// watcher. It supports enable/disable toggling and maintains a rolling window
// of the most recent events.
type Panel struct {
	enabled bool
	events  []Event
	maxLog  int // maximum events to retain
	width   int
}

// NewPanel creates a watcher panel that displays up to maxLog recent events.
func NewPanel(maxLog int) *Panel {
	if maxLog <= 0 {
		maxLog = 20
	}
	return &Panel{
		enabled: true,
		maxLog:  maxLog,
	}
}

// Enabled reports whether the watcher panel is enabled.
func (p *Panel) Enabled() bool { return p.enabled }

// Events returns the current list of recorded events.
func (p *Panel) Events() []Event { return p.events }

// SetWidth sets the available render width.
func (p *Panel) SetWidth(w int) { p.width = w }

// Init satisfies the tea.Model interface.
func (p *Panel) Init() tea.Cmd { return nil }

// Update processes bubbletea messages.
func (p *Panel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case EventMsg:
		if !p.enabled {
			return p, nil
		}
		p.events = append(p.events, msg.Events...)
		if len(p.events) > p.maxLog {
			p.events = p.events[len(p.events)-p.maxLog:]
		}
	case ToggleMsg:
		p.enabled = !p.enabled
	case tea.WindowSizeMsg:
		p.width = msg.Width
	}
	return p, nil
}

// View renders the watcher panel.
func (p *Panel) View() tea.View {
	if !p.enabled {
		return tea.NewView("File watcher: disabled")
	}
	if len(p.events) == 0 {
		return tea.NewView("File watcher: watching (no changes)")
	}

	var b strings.Builder
	b.WriteString("File watcher: watching\n")

	for _, ev := range p.events {
		b.WriteString(fmt.Sprintf("  %s %s\n", opIcon(ev.Op), ev.Path))
	}

	summary := changeSummary(p.events)
	b.WriteString(summary)

	return tea.NewView(b.String())
}

// changeSummary builds a one-line summary of the events.
func changeSummary(events []Event) string {
	if len(events) == 0 {
		return ""
	}
	if len(events) == 1 {
		return fmt.Sprintf("External change detected: %s", events[0].Path)
	}
	paths := make([]string, len(events))
	for i, ev := range events {
		paths[i] = ev.Path
	}
	if len(paths) <= 5 {
		return fmt.Sprintf("External changes detected: %s", strings.Join(paths, ", "))
	}
	return fmt.Sprintf("External changes detected: %s and %d more",
		strings.Join(paths[:3], ", "), len(paths)-3)
}

func opIcon(op string) string {
	switch op {
	case "create":
		return "+"
	case "write":
		return "~"
	case "remove":
		return "-"
	case "rename":
		return ">"
	default:
		return "?"
	}
}
