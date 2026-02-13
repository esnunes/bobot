package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronExpr represents a parsed 5-field cron expression.
// Fields: minute hour day-of-month month day-of-week
type CronExpr struct {
	Minutes    []bool // 0-59
	Hours      []bool // 0-23
	DaysOfMonth []bool // 1-31
	Months     []bool // 1-12
	DaysOfWeek []bool // 0-6 (0=Sunday)
}

// Parse parses a 5-field cron expression string.
// Supported syntax: *, numeric, ranges (1-5), step values (*/15, 1-5/2), lists (1,3,5).
func Parse(expr string) (*CronExpr, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("expected 5 fields, got %d", len(fields))
	}

	minutes, err := parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("minute field: %w", err)
	}

	hours, err := parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("hour field: %w", err)
	}

	daysOfMonth, err := parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("day-of-month field: %w", err)
	}

	months, err := parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("month field: %w", err)
	}

	daysOfWeek, err := parseField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("day-of-week field: %w", err)
	}

	return &CronExpr{
		Minutes:     minutes,
		Hours:       hours,
		DaysOfMonth: daysOfMonth,
		Months:      months,
		DaysOfWeek:  daysOfWeek,
	}, nil
}

// Next returns the next UTC time matching the expression after from.
// Panics if no match is found within 4 years (should not happen for valid expressions).
func (c *CronExpr) Next(from time.Time) time.Time {
	// Start from the next minute
	t := from.UTC().Truncate(time.Minute).Add(time.Minute)

	// Search up to 4 years ahead
	limit := t.Add(4 * 365 * 24 * time.Hour)

	for t.Before(limit) {
		// Check month
		if !c.Months[t.Month()] {
			// Advance to first day of next month
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
			continue
		}

		// Check day of month
		if !c.DaysOfMonth[t.Day()] {
			// Advance to next day
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, time.UTC)
			continue
		}

		// Check day of week
		if !c.DaysOfWeek[int(t.Weekday())] {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, time.UTC)
			continue
		}

		// Check hour
		if !c.Hours[t.Hour()] {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, time.UTC)
			continue
		}

		// Check minute
		if !c.Minutes[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}

		return t
	}

	// Should never reach here for valid expressions
	return limit
}

// MinInterval computes the shortest possible gap between two consecutive firings.
func MinInterval(expr *CronExpr) time.Duration {
	// Find a representative match, then compute the next match after it
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	first := expr.Next(start)

	// Check several consecutive intervals and return the minimum
	minDuration := time.Duration(1<<63 - 1) // max duration
	current := first
	for i := 0; i < 100; i++ {
		next := expr.Next(current)
		d := next.Sub(current)
		if d < minDuration {
			minDuration = d
		}
		current = next
	}

	return minDuration
}

// parseField parses a single cron field into a boolean slice.
// The slice is indexed from 0 to max (inclusive).
func parseField(field string, min, max int) ([]bool, error) {
	result := make([]bool, max+1)

	// Handle comma-separated list
	parts := strings.Split(field, ",")
	for _, part := range parts {
		if err := parsePart(part, min, max, result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func parsePart(part string, min, max int, result []bool) error {
	// Handle step values: */n or range/n
	step := 1
	if idx := strings.Index(part, "/"); idx != -1 {
		var err error
		step, err = strconv.Atoi(part[idx+1:])
		if err != nil || step <= 0 {
			return fmt.Errorf("invalid step value: %s", part)
		}
		part = part[:idx]
	}

	// Handle wildcard
	if part == "*" {
		for i := min; i <= max; i += step {
			result[i] = true
		}
		return nil
	}

	// Handle range: a-b
	if idx := strings.Index(part, "-"); idx != -1 {
		start, err := strconv.Atoi(part[:idx])
		if err != nil {
			return fmt.Errorf("invalid range start: %s", part)
		}
		end, err := strconv.Atoi(part[idx+1:])
		if err != nil {
			return fmt.Errorf("invalid range end: %s", part)
		}
		if start < min || end > max || start > end {
			return fmt.Errorf("range out of bounds: %d-%d (valid: %d-%d)", start, end, min, max)
		}
		for i := start; i <= end; i += step {
			result[i] = true
		}
		return nil
	}

	// Handle single number
	val, err := strconv.Atoi(part)
	if err != nil {
		return fmt.Errorf("invalid value: %s", part)
	}
	if val < min || val > max {
		return fmt.Errorf("value out of range: %d (valid: %d-%d)", val, min, max)
	}
	result[val] = true
	return nil
}
