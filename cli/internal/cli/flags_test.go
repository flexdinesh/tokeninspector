package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSelectedPeriodToday(t *testing.T) {
	got, err := selectedPeriod(true, false, false, false, false, periodWeek)
	if err != nil {
		t.Fatal(err)
	}
	if got != periodToday {
		t.Fatalf("got %q, want %q", got, periodToday)
	}
}

func TestSelectedPeriodWeek(t *testing.T) {
	got, err := selectedPeriod(false, true, false, false, false, periodWeek)
	if err != nil {
		t.Fatal(err)
	}
	if got != periodWeek {
		t.Fatalf("got %q, want %q", got, periodWeek)
	}
}

func TestSelectedPeriodMonth(t *testing.T) {
	got, err := selectedPeriod(false, false, true, false, false, periodWeek)
	if err != nil {
		t.Fatal(err)
	}
	if got != periodMonth {
		t.Fatalf("got %q, want %q", got, periodMonth)
	}
}

func TestSelectedPeriodAllTime(t *testing.T) {
	got, err := selectedPeriod(false, false, false, true, false, periodWeek)
	if err != nil {
		t.Fatal(err)
	}
	if got != periodAllTime {
		t.Fatalf("got %q, want %q", got, periodAllTime)
	}
}

func TestSelectedPeriodDefault(t *testing.T) {
	got, err := selectedPeriod(false, false, false, false, false, periodWeek)
	if err != nil {
		t.Fatal(err)
	}
	if got != periodWeek {
		t.Fatalf("got %q, want %q", got, periodWeek)
	}
}

func TestSelectedPeriodMultiple(t *testing.T) {
	_, err := selectedPeriod(true, true, false, false, false, periodWeek)
	if err == nil {
		t.Fatal("expected error for multiple periods")
	}
	if !strings.Contains(err.Error(), "choose exactly one") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSelectedPeriodRequired(t *testing.T) {
	_, err := selectedPeriod(false, false, false, false, true, periodWeek)
	if err == nil {
		t.Fatal("expected error when period required but none given")
	}
	if !strings.Contains(err.Error(), "choose exactly one") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPeriodStartToday(t *testing.T) {
	now := time.Date(2026, 4, 24, 15, 30, 0, 0, time.Local)
	got := periodStart(now, periodToday)
	want := time.Date(2026, 4, 24, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPeriodStartWeek(t *testing.T) {
	base := time.Date(2026, 4, 20, 15, 0, 0, 0, time.Local)

	for i := 0; i < 7; i++ {
		date := base.AddDate(0, 0, i)
		got := periodStart(date, periodWeek)

		offset := int(date.Weekday() - time.Monday)
		if offset < 0 {
			offset += 7
		}
		monday := date.AddDate(0, 0, -offset)
		want := time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, monday.Location())

		if !got.Equal(want) {
			t.Errorf("%s: got %v, want %v", date.Weekday(), got, want)
		}
	}
}

func TestPeriodStartMonth(t *testing.T) {
	now := time.Date(2026, 4, 24, 15, 30, 0, 0, time.Local)
	got := periodStart(now, periodMonth)
	want := time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPeriodStartAllTime(t *testing.T) {
	now := time.Date(2026, 4, 24, 15, 30, 0, 0, time.Local)
	got := periodStart(now, periodAllTime)
	if !got.IsZero() {
		t.Fatalf("expected zero time, got %v", got)
	}
}

func TestParseTableOptionsMissingDBPath(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseTableOptions([]string{"--today"}, &stderr, false, periodWeek)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing --db-path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTableOptionsToday(t *testing.T) {
	var stderr bytes.Buffer
	opts, err := parseTableOptions([]string{"--db-path", "/tmp/test.sqlite", "--today"}, &stderr, false, periodWeek)
	if err != nil {
		t.Fatal(err)
	}
	if opts.period != periodToday {
		t.Fatalf("got %q, want %q", opts.period, periodToday)
	}
}

func TestParseTableOptionsAllTime(t *testing.T) {
	var stderr bytes.Buffer
	opts, err := parseTableOptions([]string{"--db-path", "/tmp/test.sqlite", "--all-time"}, &stderr, false, periodWeek)
	if err != nil {
		t.Fatal(err)
	}
	if opts.period != periodAllTime {
		t.Fatalf("got %q, want %q", opts.period, periodAllTime)
	}
}

func TestParseTableOptionsMultiplePeriods(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseTableOptions([]string{"--db-path", "/tmp/test.sqlite", "--today", "--week"}, &stderr, false, periodWeek)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "choose exactly one") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTableOptionsInvalidDayFrom(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseTableOptions([]string{"--db-path", "/tmp/test.sqlite", "--today", "--filter-day-from", "not-a-date"}, &stderr, false, periodWeek)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid --filter-day-from") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTableOptionsInvalidDayTo(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseTableOptions([]string{"--db-path", "/tmp/test.sqlite", "--today", "--filter-day-to", "bad"}, &stderr, false, periodWeek)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid --filter-day-to") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTableOptionsDayFromAfterDayTo(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseTableOptions([]string{
		"--db-path", "/tmp/test.sqlite",
		"--today",
		"--filter-day-from", "2026-04-25",
		"--filter-day-to", "2026-04-20",
	}, &stderr, false, periodWeek)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "must not be after") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTableOptionsValidDayRange(t *testing.T) {
	var stderr bytes.Buffer
	opts, err := parseTableOptions([]string{
		"--db-path", "/tmp/test.sqlite",
		"--today",
		"--filter-day-from", "2026-04-20",
		"--filter-day-to", "2026-04-25",
	}, &stderr, false, periodWeek)
	if err != nil {
		t.Fatal(err)
	}
	if opts.filters.dayFrom != "2026-04-20" {
		t.Fatalf("got dayFrom %q, want 2026-04-20", opts.filters.dayFrom)
	}
	if opts.filters.dayTo != "2026-04-25" {
		t.Fatalf("got dayTo %q, want 2026-04-25", opts.filters.dayTo)
	}
}

func TestParseTableOptionsUnexpectedArgument(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseTableOptions([]string{"--db-path", "/tmp/test.sqlite", "--today", "unexpected"}, &stderr, false, periodWeek)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unexpected argument") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTableOptionsDefaultWeek(t *testing.T) {
	var stderr bytes.Buffer
	opts, err := parseTableOptions([]string{"--db-path", "/tmp/test.sqlite"}, &stderr, false, periodWeek)
	if err != nil {
		t.Fatal(err)
	}
	if opts.period != periodWeek {
		t.Fatalf("got %q, want %q", opts.period, periodWeek)
	}
}
