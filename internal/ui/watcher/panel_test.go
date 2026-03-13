package watcher

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// newTestPanel creates a Panel and returns it, mirroring the teatest.NewModel() pattern.
func newTestPanel(maxLog int) *Panel {
	p := NewPanel(maxLog)
	p.Init()
	return p
}

func viewContent(p *Panel) string {
	return p.View().Content
}

// --- (1) Watcher panel rendering ---

func TestPanelRenderEmpty(t *testing.T) {
	p := newTestPanel(10)
	got := viewContent(p)
	if !strings.Contains(got, "watching") {
		t.Fatalf("expected 'watching' in empty panel view, got: %q", got)
	}
	if !strings.Contains(got, "no changes") {
		t.Fatalf("expected 'no changes' in empty panel view, got: %q", got)
	}
}

func TestPanelRenderDisabled(t *testing.T) {
	p := newTestPanel(10)
	p.Update(ToggleMsg{})
	got := viewContent(p)
	if !strings.Contains(got, "disabled") {
		t.Fatalf("expected 'disabled' in view, got: %q", got)
	}
}

func TestPanelRenderWithEvents(t *testing.T) {
	p := newTestPanel(10)
	p.Update(EventMsg{Events: []Event{
		{Path: "main.go", Op: "write"},
		{Path: "go.mod", Op: "create"},
	}})
	got := viewContent(p)
	if !strings.Contains(got, "main.go") {
		t.Fatalf("expected 'main.go' in view, got: %q", got)
	}
	if !strings.Contains(got, "go.mod") {
		t.Fatalf("expected 'go.mod' in view, got: %q", got)
	}
	if !strings.Contains(got, "~ main.go") {
		t.Fatalf("expected '~ main.go' (write icon) in view, got: %q", got)
	}
	if !strings.Contains(got, "+ go.mod") {
		t.Fatalf("expected '+ go.mod' (create icon) in view, got: %q", got)
	}
}

func TestPanelRenderOpIcons(t *testing.T) {
	tests := []struct {
		op   string
		icon string
	}{
		{"create", "+"},
		{"write", "~"},
		{"remove", "-"},
		{"rename", ">"},
		{"modify", "?"},
	}
	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			p := newTestPanel(10)
			p.Update(EventMsg{Events: []Event{{Path: "test.txt", Op: tt.op}}})
			got := viewContent(p)
			want := tt.icon + " test.txt"
			if !strings.Contains(got, want) {
				t.Fatalf("expected %q in view for op %q, got: %q", want, tt.op, got)
			}
		})
	}
}

// --- (2) File change event display ---

func TestPanelSingleEventDisplay(t *testing.T) {
	p := newTestPanel(10)
	p.Update(EventMsg{Events: []Event{{Path: "README.md", Op: "write"}}})
	got := viewContent(p)
	if !strings.Contains(got, "External change detected: README.md") {
		t.Fatalf("expected single-file summary, got: %q", got)
	}
}

func TestPanelMultipleEventDisplay(t *testing.T) {
	p := newTestPanel(10)
	p.Update(EventMsg{Events: []Event{
		{Path: "a.go", Op: "write"},
		{Path: "b.go", Op: "create"},
		{Path: "c.go", Op: "remove"},
	}})
	got := viewContent(p)
	if !strings.Contains(got, "External changes detected: a.go, b.go, c.go") {
		t.Fatalf("expected multi-file summary, got: %q", got)
	}
}

func TestPanelManyEventsDisplay(t *testing.T) {
	p := newTestPanel(20)
	events := make([]Event, 8)
	for i := range events {
		events[i] = Event{Path: strings.Repeat("x", 1) + string(rune('a'+i)) + ".go", Op: "write"}
	}
	p.Update(EventMsg{Events: events})
	got := viewContent(p)
	if !strings.Contains(got, "and 5 more") {
		t.Fatalf("expected truncated summary with 'and 5 more', got: %q", got)
	}
}

func TestPanelBatchedEventsAccumulate(t *testing.T) {
	p := newTestPanel(10)

	// First batch.
	p.Update(EventMsg{Events: []Event{{Path: "a.go", Op: "write"}}})
	if len(p.Events()) != 1 {
		t.Fatalf("expected 1 event after first batch, got %d", len(p.Events()))
	}

	// Second batch.
	p.Update(EventMsg{Events: []Event{
		{Path: "b.go", Op: "create"},
		{Path: "c.go", Op: "remove"},
	}})
	if len(p.Events()) != 3 {
		t.Fatalf("expected 3 events after second batch, got %d", len(p.Events()))
	}

	got := viewContent(p)
	for _, path := range []string{"a.go", "b.go", "c.go"} {
		if !strings.Contains(got, path) {
			t.Fatalf("expected %q in accumulated view, got: %q", path, got)
		}
	}
}

func TestPanelRollingWindowEvictsOld(t *testing.T) {
	p := newTestPanel(3) // keep only 3 events

	// Add 5 events.
	for i := 0; i < 5; i++ {
		p.Update(EventMsg{Events: []Event{
			{Path: strings.Repeat("f", 1) + string(rune('0'+i)) + ".go", Op: "write"},
		}})
	}

	if len(p.Events()) != 3 {
		t.Fatalf("expected maxLog=3 to cap events, got %d", len(p.Events()))
	}

	got := viewContent(p)
	// Oldest events (f0.go, f1.go) should be evicted.
	if strings.Contains(got, "f0.go") {
		t.Fatalf("expected f0.go to be evicted, got: %q", got)
	}
	if strings.Contains(got, "f1.go") {
		t.Fatalf("expected f1.go to be evicted, got: %q", got)
	}
	// Newest events should remain.
	if !strings.Contains(got, "f4.go") {
		t.Fatalf("expected f4.go to be present, got: %q", got)
	}
}

// --- (3) Reactive context update display ---

func TestPanelUpdatesViewOnNewEvents(t *testing.T) {
	p := newTestPanel(10)

	// Initial view should show "no changes".
	v1 := viewContent(p)
	if !strings.Contains(v1, "no changes") {
		t.Fatalf("expected 'no changes' initially, got: %q", v1)
	}

	// After event, view should update reactively.
	p.Update(EventMsg{Events: []Event{{Path: "config.yaml", Op: "write"}}})
	v2 := viewContent(p)
	if strings.Contains(v2, "no changes") {
		t.Fatalf("view should no longer show 'no changes' after event, got: %q", v2)
	}
	if !strings.Contains(v2, "config.yaml") {
		t.Fatalf("expected 'config.yaml' in updated view, got: %q", v2)
	}
}

func TestPanelContextAfterToggleCycle(t *testing.T) {
	p := newTestPanel(10)

	// Add events while enabled.
	p.Update(EventMsg{Events: []Event{{Path: "a.go", Op: "write"}}})
	if len(p.Events()) != 1 {
		t.Fatalf("expected 1 event, got %d", len(p.Events()))
	}

	// Disable — events should be ignored.
	p.Update(ToggleMsg{})
	p.Update(EventMsg{Events: []Event{{Path: "b.go", Op: "create"}}})
	if len(p.Events()) != 1 {
		t.Fatalf("expected events unchanged while disabled, got %d", len(p.Events()))
	}

	// Re-enable — new events should arrive, old ones preserved.
	p.Update(ToggleMsg{})
	p.Update(EventMsg{Events: []Event{{Path: "c.go", Op: "remove"}}})
	if len(p.Events()) != 2 {
		t.Fatalf("expected 2 events after re-enable, got %d", len(p.Events()))
	}

	got := viewContent(p)
	if !strings.Contains(got, "a.go") {
		t.Fatalf("expected 'a.go' preserved after toggle cycle, got: %q", got)
	}
	if !strings.Contains(got, "c.go") {
		t.Fatalf("expected 'c.go' after re-enable, got: %q", got)
	}
	if strings.Contains(got, "b.go") {
		t.Fatalf("'b.go' should not appear (sent while disabled), got: %q", got)
	}
}

func TestPanelWindowSizeMsg(t *testing.T) {
	p := newTestPanel(10)
	p.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if p.width != 120 {
		t.Fatalf("expected width=120 after resize, got %d", p.width)
	}
}

// --- (4) Watcher enable/disable toggle ---

func TestPanelToggleDisables(t *testing.T) {
	p := newTestPanel(10)
	if !p.Enabled() {
		t.Fatal("expected panel to start enabled")
	}

	p.Update(ToggleMsg{})
	if p.Enabled() {
		t.Fatal("expected panel to be disabled after toggle")
	}

	got := viewContent(p)
	if !strings.Contains(got, "disabled") {
		t.Fatalf("expected 'disabled' in view after toggle, got: %q", got)
	}
}

func TestPanelToggleReEnables(t *testing.T) {
	p := newTestPanel(10)
	p.Update(ToggleMsg{}) // disable
	p.Update(ToggleMsg{}) // re-enable

	if !p.Enabled() {
		t.Fatal("expected panel to be re-enabled after double toggle")
	}
	got := viewContent(p)
	if strings.Contains(got, "disabled") {
		t.Fatalf("expected 'disabled' to not appear after re-enable, got: %q", got)
	}
}

func TestPanelDisabledIgnoresEvents(t *testing.T) {
	p := newTestPanel(10)
	p.Update(ToggleMsg{}) // disable

	p.Update(EventMsg{Events: []Event{{Path: "ignored.go", Op: "write"}}})
	if len(p.Events()) != 0 {
		t.Fatalf("expected no events while disabled, got %d", len(p.Events()))
	}

	got := viewContent(p)
	if strings.Contains(got, "ignored.go") {
		t.Fatalf("expected disabled panel to not show events, got: %q", got)
	}
}

func TestPanelEnabledAcceptsEvents(t *testing.T) {
	p := newTestPanel(10)
	// Enabled by default.
	p.Update(EventMsg{Events: []Event{{Path: "accepted.go", Op: "write"}}})
	if len(p.Events()) != 1 {
		t.Fatalf("expected 1 event while enabled, got %d", len(p.Events()))
	}
}

func TestPanelTogglePreservesExistingEvents(t *testing.T) {
	p := newTestPanel(10)
	p.Update(EventMsg{Events: []Event{{Path: "keep.go", Op: "write"}}})

	p.Update(ToggleMsg{}) // disable
	if len(p.Events()) != 1 {
		t.Fatalf("toggle should not clear events, got %d", len(p.Events()))
	}

	p.Update(ToggleMsg{}) // re-enable
	got := viewContent(p)
	if !strings.Contains(got, "keep.go") {
		t.Fatalf("expected 'keep.go' preserved through toggle cycle, got: %q", got)
	}
}

// --- changeSummary unit tests ---

func TestChangeSummaryEmpty(t *testing.T) {
	if s := changeSummary(nil); s != "" {
		t.Fatalf("expected empty string for nil events, got: %q", s)
	}
}

func TestChangeSummarySingle(t *testing.T) {
	s := changeSummary([]Event{{Path: "x.go", Op: "write"}})
	if s != "External change detected: x.go" {
		t.Fatalf("unexpected: %q", s)
	}
}

func TestChangeSummaryFew(t *testing.T) {
	s := changeSummary([]Event{
		{Path: "a.go", Op: "write"},
		{Path: "b.go", Op: "create"},
	})
	if s != "External changes detected: a.go, b.go" {
		t.Fatalf("unexpected: %q", s)
	}
}

func TestChangeSummaryMany(t *testing.T) {
	events := make([]Event, 7)
	for i := range events {
		events[i] = Event{Path: string(rune('a'+i)) + ".go", Op: "write"}
	}
	s := changeSummary(events)
	if !strings.Contains(s, "and 4 more") {
		t.Fatalf("expected 'and 4 more', got: %q", s)
	}
}
