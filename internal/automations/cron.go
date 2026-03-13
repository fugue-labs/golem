package automations

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronTrigger evaluates cron schedule expressions and fires events on match.
type CronTrigger struct {
	Name     string
	Schedule CronSchedule
	Workflow Workflow
}

// CronSchedule represents a parsed 5-field cron expression:
// minute hour day-of-month month day-of-week
type CronSchedule struct {
	Minute     fieldMatcher
	Hour       fieldMatcher
	DayOfMonth fieldMatcher
	Month      fieldMatcher
	DayOfWeek  fieldMatcher
	Raw        string
}

// ParseCronSchedule parses a standard 5-field cron expression.
// Supports: *, N, N-M, */N, N-M/N, comma-separated lists.
func ParseCronSchedule(expr string) (CronSchedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return CronSchedule{}, fmt.Errorf("cron expression must have 5 fields, got %d: %q", len(fields), expr)
	}

	minute, err := parseField(fields[0], 0, 59)
	if err != nil {
		return CronSchedule{}, fmt.Errorf("minute field: %w", err)
	}
	hour, err := parseField(fields[1], 0, 23)
	if err != nil {
		return CronSchedule{}, fmt.Errorf("hour field: %w", err)
	}
	dom, err := parseField(fields[2], 1, 31)
	if err != nil {
		return CronSchedule{}, fmt.Errorf("day-of-month field: %w", err)
	}
	month, err := parseField(fields[3], 1, 12)
	if err != nil {
		return CronSchedule{}, fmt.Errorf("month field: %w", err)
	}
	dow, err := parseField(fields[4], 0, 6)
	if err != nil {
		return CronSchedule{}, fmt.Errorf("day-of-week field: %w", err)
	}

	return CronSchedule{
		Minute:     minute,
		Hour:       hour,
		DayOfMonth: dom,
		Month:      month,
		DayOfWeek:  dow,
		Raw:        expr,
	}, nil
}

// Matches reports whether the given time matches this schedule.
func (s CronSchedule) Matches(t time.Time) bool {
	return s.Minute.matches(t.Minute()) &&
		s.Hour.matches(t.Hour()) &&
		s.DayOfMonth.matches(t.Day()) &&
		s.Month.matches(int(t.Month())) &&
		s.DayOfWeek.matches(int(t.Weekday()))
}

// NextAfter returns the next time after t that matches this schedule.
// It searches up to 366 days into the future.
func (s CronSchedule) NextAfter(t time.Time) (time.Time, bool) {
	// Start from the next minute.
	candidate := t.Truncate(time.Minute).Add(time.Minute)
	limit := t.Add(366 * 24 * time.Hour)

	for candidate.Before(limit) {
		if s.Matches(candidate) {
			return candidate, true
		}
		// Skip forward intelligently.
		if !s.Month.matches(int(candidate.Month())) {
			// Jump to next month.
			candidate = time.Date(candidate.Year(), candidate.Month()+1, 1, 0, 0, 0, 0, candidate.Location())
			continue
		}
		if !s.DayOfMonth.matches(candidate.Day()) || !s.DayOfWeek.matches(int(candidate.Weekday())) {
			// Jump to next day.
			candidate = time.Date(candidate.Year(), candidate.Month(), candidate.Day()+1, 0, 0, 0, 0, candidate.Location())
			continue
		}
		if !s.Hour.matches(candidate.Hour()) {
			// Jump to next hour.
			candidate = time.Date(candidate.Year(), candidate.Month(), candidate.Day(), candidate.Hour()+1, 0, 0, 0, candidate.Location())
			continue
		}
		// Try next minute.
		candidate = candidate.Add(time.Minute)
	}
	return time.Time{}, false
}

// RunCronTrigger starts a goroutine that checks the schedule every minute and
// sends matching events to the handler. Blocks until ctx is cancelled.
func RunCronTrigger(ctx context.Context, trigger CronTrigger, handler func(Event)) {
	// Align to the next minute boundary.
	now := time.Now()
	nextMinute := now.Truncate(time.Minute).Add(time.Minute)
	alignTimer := time.NewTimer(time.Until(nextMinute))
	defer alignTimer.Stop()

	select {
	case <-ctx.Done():
		return
	case <-alignTimer.C:
	}

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	checkAndFire := func(t time.Time) {
		if trigger.Schedule.Matches(t) {
			props := map[string]any{
				"schedule":  trigger.Schedule.Raw,
				"fired_at":  t.Format(time.RFC3339),
				"automation": trigger.Name,
			}
			raw, _ := json.Marshal(props)
			handler(Event{
				Type:       "cron",
				Name:       trigger.Name,
				Timestamp:  t,
				Raw:        raw,
				Properties: props,
			})
		}
	}

	// Check immediately for the aligned minute.
	checkAndFire(time.Now().Truncate(time.Minute))

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			checkAndFire(t.Truncate(time.Minute))
		}
	}
}

// --- Field matching ---

type fieldMatcher struct {
	values map[int]bool // nil means "match all" (wildcard)
}

func (f fieldMatcher) matches(v int) bool {
	if f.values == nil {
		return true // wildcard
	}
	return f.values[v]
}

func parseField(field string, min, max int) (fieldMatcher, error) {
	if field == "*" {
		return fieldMatcher{}, nil
	}

	values := make(map[int]bool)
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if err := parseFieldPart(part, min, max, values); err != nil {
			return fieldMatcher{}, err
		}
	}
	return fieldMatcher{values: values}, nil
}

func parseFieldPart(part string, min, max int, values map[int]bool) error {
	// Handle step: */N or N-M/N
	step := 1
	if idx := strings.Index(part, "/"); idx >= 0 {
		s, err := strconv.Atoi(part[idx+1:])
		if err != nil || s <= 0 {
			return fmt.Errorf("invalid step in %q", part)
		}
		step = s
		part = part[:idx]
	}

	// Handle range or wildcard.
	var lo, hi int
	if part == "*" {
		lo, hi = min, max
	} else if idx := strings.Index(part, "-"); idx >= 0 {
		var err error
		lo, err = strconv.Atoi(part[:idx])
		if err != nil {
			return fmt.Errorf("invalid range start in %q", part)
		}
		hi, err = strconv.Atoi(part[idx+1:])
		if err != nil {
			return fmt.Errorf("invalid range end in %q", part)
		}
	} else {
		v, err := strconv.Atoi(part)
		if err != nil {
			return fmt.Errorf("invalid value %q", part)
		}
		if v < min || v > max {
			return fmt.Errorf("value %d out of range [%d, %d]", v, min, max)
		}
		values[v] = true
		return nil
	}

	if lo < min || hi > max || lo > hi {
		return fmt.Errorf("range %d-%d out of bounds [%d, %d]", lo, hi, min, max)
	}
	for i := lo; i <= hi; i += step {
		values[i] = true
	}
	return nil
}
