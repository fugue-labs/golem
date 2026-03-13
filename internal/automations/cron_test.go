package automations

import (
	"testing"
	"time"
)

func TestParseCronSchedule(t *testing.T) {
	tests := []struct {
		expr    string
		wantErr bool
	}{
		{"* * * * *", false},
		{"0 9 * * *", false},
		{"*/15 * * * *", false},
		{"0 9 * * 1-5", false},
		{"0,30 * * * *", false},
		{"0 9 1,15 * *", false},
		{"0 0 * * 0", false},        // Sunday
		{"5 4 * * 6", false},        // Saturday at 4:05
		{"0 */2 * * *", false},      // Every 2 hours
		{"too few", true},           // Wrong field count
		{"a b c d e", true},         // Non-numeric
		{"60 * * * *", true},        // Minute out of range
		{"* 24 * * *", true},        // Hour out of range
		{"* * 0 * *", true},         // Day 0 out of range
		{"* * * 13 *", true},        // Month 13 out of range
		{"* * * * 7", true},         // Day-of-week 7 out of range
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			_, err := ParseCronSchedule(tt.expr)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q", tt.expr)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.expr, err)
			}
		})
	}
}

func TestCronScheduleMatches(t *testing.T) {
	tests := []struct {
		expr string
		time time.Time
		want bool
	}{
		// "every minute" matches any time
		{"* * * * *", time.Date(2026, 3, 12, 14, 30, 0, 0, time.UTC), true},
		// 9:00 daily
		{"0 9 * * *", time.Date(2026, 3, 12, 9, 0, 0, 0, time.UTC), true},
		{"0 9 * * *", time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC), false},
		// Every 15 minutes
		{"*/15 * * * *", time.Date(2026, 3, 12, 14, 0, 0, 0, time.UTC), true},
		{"*/15 * * * *", time.Date(2026, 3, 12, 14, 15, 0, 0, time.UTC), true},
		{"*/15 * * * *", time.Date(2026, 3, 12, 14, 30, 0, 0, time.UTC), true},
		{"*/15 * * * *", time.Date(2026, 3, 12, 14, 7, 0, 0, time.UTC), false},
		// Weekdays only (Mon-Fri)
		{"0 9 * * 1-5", time.Date(2026, 3, 12, 9, 0, 0, 0, time.UTC), true},  // Thursday
		{"0 9 * * 1-5", time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC), false}, // Sunday
		// Specific day of month
		{"0 0 1 * *", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), true},
		{"0 0 1 * *", time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), false},
		// Comma list
		{"0,30 * * * *", time.Date(2026, 3, 12, 14, 0, 0, 0, time.UTC), true},
		{"0,30 * * * *", time.Date(2026, 3, 12, 14, 30, 0, 0, time.UTC), true},
		{"0,30 * * * *", time.Date(2026, 3, 12, 14, 15, 0, 0, time.UTC), false},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			sched, err := ParseCronSchedule(tt.expr)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			got := sched.Matches(tt.time)
			if got != tt.want {
				t.Fatalf("Matches(%v) = %v, want %v", tt.time, got, tt.want)
			}
		})
	}
}

func TestCronScheduleNextAfter(t *testing.T) {
	tests := []struct {
		expr  string
		after time.Time
		want  time.Time
	}{
		{
			"0 9 * * *",
			time.Date(2026, 3, 12, 8, 0, 0, 0, time.UTC),
			time.Date(2026, 3, 12, 9, 0, 0, 0, time.UTC),
		},
		{
			"0 9 * * *",
			time.Date(2026, 3, 12, 9, 30, 0, 0, time.UTC),
			time.Date(2026, 3, 13, 9, 0, 0, 0, time.UTC),
		},
		{
			"30 * * * *",
			time.Date(2026, 3, 12, 14, 29, 0, 0, time.UTC),
			time.Date(2026, 3, 12, 14, 30, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			sched, err := ParseCronSchedule(tt.expr)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			got, ok := sched.NextAfter(tt.after)
			if !ok {
				t.Fatal("NextAfter returned not found")
			}
			if !got.Equal(tt.want) {
				t.Fatalf("NextAfter(%v) = %v, want %v", tt.after, got, tt.want)
			}
		})
	}
}
