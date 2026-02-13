package schedule

import (
	"testing"
	"time"
)

func TestParse_ValidExpressions(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"every minute", "* * * * *"},
		{"specific minute", "30 * * * *"},
		{"every 15 minutes", "*/15 * * * *"},
		{"weekdays at 9am", "0 9 * * 1-5"},
		{"first of month", "0 0 1 * *"},
		{"specific days", "0 9 * * 1,3,5"},
		{"range with step", "0-30/10 * * * *"},
		{"complex", "15 14 1 * *"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.expr)
			if err != nil {
				t.Errorf("Parse(%q) returned error: %v", tt.expr, err)
			}
		})
	}
}

func TestParse_InvalidExpressions(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"too few fields", "* * *"},
		{"too many fields", "* * * * * *"},
		{"invalid minute", "60 * * * *"},
		{"invalid hour", "* 24 * * *"},
		{"invalid day", "* * 32 * *"},
		{"invalid month", "* * * 13 *"},
		{"invalid dow", "* * * * 7"},
		{"invalid step", "*/0 * * * *"},
		{"invalid value", "abc * * * *"},
		{"bad range", "5-3 * * * *"},
		{"range out of bounds", "* * * 0-12 *"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.expr)
			if err == nil {
				t.Errorf("Parse(%q) expected error", tt.expr)
			}
		})
	}
}

func TestNext(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		from     time.Time
		expected time.Time
	}{
		{
			"every minute",
			"* * * * *",
			time.Date(2026, 2, 12, 10, 30, 0, 0, time.UTC),
			time.Date(2026, 2, 12, 10, 31, 0, 0, time.UTC),
		},
		{
			"specific minute",
			"30 * * * *",
			time.Date(2026, 2, 12, 10, 0, 0, 0, time.UTC),
			time.Date(2026, 2, 12, 10, 30, 0, 0, time.UTC),
		},
		{
			"specific minute past",
			"30 * * * *",
			time.Date(2026, 2, 12, 10, 30, 0, 0, time.UTC),
			time.Date(2026, 2, 12, 11, 30, 0, 0, time.UTC),
		},
		{
			"every 15 minutes",
			"*/15 * * * *",
			time.Date(2026, 2, 12, 10, 0, 0, 0, time.UTC),
			time.Date(2026, 2, 12, 10, 15, 0, 0, time.UTC),
		},
		{
			"weekdays at 9am UTC - from Monday",
			"0 9 * * 1-5",
			time.Date(2026, 2, 9, 0, 0, 0, 0, time.UTC), // Monday
			time.Date(2026, 2, 9, 9, 0, 0, 0, time.UTC),
		},
		{
			"weekdays at 9am UTC - from Saturday",
			"0 9 * * 1-5",
			time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC), // Saturday
			time.Date(2026, 2, 16, 9, 0, 0, 0, time.UTC),  // Monday
		},
		{
			"first of month at midnight",
			"0 0 1 * *",
			time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"specific days of week",
			"0 9 * * 1,3,5",
			time.Date(2026, 2, 10, 10, 0, 0, 0, time.UTC), // Tuesday
			time.Date(2026, 2, 11, 9, 0, 0, 0, time.UTC),  // Wednesday
		},
		{
			"year rollover",
			"0 0 1 1 *",
			time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
			time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"range with step",
			"0-30/10 * * * *",
			time.Date(2026, 2, 12, 10, 5, 0, 0, time.UTC),
			time.Date(2026, 2, 12, 10, 10, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := Parse(tt.expr)
			if err != nil {
				t.Fatal(err)
			}
			got := expr.Next(tt.from)
			if !got.Equal(tt.expected) {
				t.Errorf("Next(%v) = %v, want %v", tt.from, got, tt.expected)
			}
		})
	}
}

func TestNext_SkipsCurrentMinute(t *testing.T) {
	// When from is exactly on a matching minute, Next should return the *next* match
	expr, _ := Parse("0 9 * * *")
	from := time.Date(2026, 2, 12, 9, 0, 0, 0, time.UTC)
	got := expr.Next(from)
	expected := time.Date(2026, 2, 13, 9, 0, 0, 0, time.UTC)
	if !got.Equal(expected) {
		t.Errorf("Next(%v) = %v, want %v", from, got, expected)
	}
}

