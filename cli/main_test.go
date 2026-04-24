package main

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPeriodStart(t *testing.T) {
	now := time.Date(2026, 4, 24, 15, 30, 0, 0, time.Local)

	if got := periodStart(now, periodDay); got.Day() != 24 || got.Hour() != 0 {
		t.Fatalf("day start = %v", got)
	}
	if got := periodStart(now, periodWeek); got.Day() != 18 || got.Hour() != 0 {
		t.Fatalf("week start = %v", got)
	}
	if got := periodStart(now, periodMonth); got.Day() != 1 || got.Hour() != 0 {
		t.Fatalf("month start = %v", got)
	}
}

func TestAggregateSamples(t *testing.T) {
	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	yesterday := time.Date(2026, 4, 23, 12, 0, 0, 0, time.Local).UnixMilli()

	rows := aggregateSamples([]sample{
		{recordedAtMs: yesterday, sessionID: "ses_1", provider: "anthropic", model: "claude", inputTokens: 10, outputTokens: 5, reasoningTokens: 1, cacheReadTokens: 2, cacheWriteTokens: 3, totalTokens: 21},
		{recordedAtMs: yesterday, sessionID: "ses_1", provider: "anthropic", model: "claude", throughputTokens: 50, durationMs: 1000, tokensPerSecond: 50},
		{recordedAtMs: day, sessionID: "ses_2", provider: "openai", model: "gpt", inputTokens: 100, outputTokens: 10, reasoningTokens: 5, cacheReadTokens: 20, cacheWriteTokens: 1, totalTokens: 136},
		{recordedAtMs: day, sessionID: "ses_2", provider: "openai", model: "gpt", inputTokens: 200, outputTokens: 20, reasoningTokens: 6, cacheReadTokens: 30, cacheWriteTokens: 2, totalTokens: 258},
		{recordedAtMs: day, sessionID: "ses_2", provider: "openai", model: "gpt", throughputTokens: 100, durationMs: 1000, tokensPerSecond: 100},
		{recordedAtMs: day, sessionID: "ses_2", provider: "openai", model: "gpt", throughputTokens: 100, durationMs: 10000, tokensPerSecond: 10},
	}, groupByNone)

	if len(rows) != 2 {
		t.Fatalf("got %d rows", len(rows))
	}
	row := rows[0]
	if row.day != "2026-04-24" || row.hour != "" || row.provider != "openai" || row.model != "gpt" {
		t.Fatalf("unexpected first row: %+v", row)
	}
	if row.inputTokens != "300" || row.outputTokens != "30" || row.reasoningTokens != "11" || row.cacheReadTokens != "50" || row.cacheWriteTokens != "3" || row.totalTokens != "394" {
		t.Fatalf("unexpected tokens: %+v", row)
	}
	if row.tpsAvg != "18.18" || row.tpsMean != "55.00" || row.tpsMedian != "55.00" {
		t.Fatalf("unexpected tps: %+v", row)
	}
}

func TestAggregateSamplesHourly(t *testing.T) {
	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()

	rows := aggregateSamples([]sample{
		{recordedAtMs: day, sessionID: "ses_1", provider: "openai", model: "gpt", inputTokens: 100, outputTokens: 10, reasoningTokens: 5, cacheReadTokens: 20, cacheWriteTokens: 1, totalTokens: 136},
		{recordedAtMs: day, sessionID: "ses_1", provider: "openai", model: "gpt", inputTokens: 200, outputTokens: 20, reasoningTokens: 6, cacheReadTokens: 30, cacheWriteTokens: 2, totalTokens: 258},
		{recordedAtMs: day, sessionID: "ses_1", provider: "openai", model: "gpt", throughputTokens: 100, durationMs: 1000, tokensPerSecond: 100},
		{recordedAtMs: day, sessionID: "ses_1", provider: "openai", model: "gpt", throughputTokens: 100, durationMs: 10000, tokensPerSecond: 10},
	}, groupByHour)

	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	row := rows[0]
	if row.day != "2026-04-24" || row.hour != "12:00" || row.provider != "openai" || row.model != "gpt" {
		t.Fatalf("unexpected data row: %+v", row)
	}
	if row.inputTokens != "300" || row.outputTokens != "30" || row.reasoningTokens != "11" || row.cacheReadTokens != "50" || row.cacheWriteTokens != "3" || row.totalTokens != "394" {
		t.Fatalf("unexpected tokens: %+v", row)
	}
	if row.tpsAvg != "18.18" || row.tpsMean != "55.00" || row.tpsMedian != "55.00" {
		t.Fatalf("unexpected tps: %+v", row)
	}
}

func TestAggregateSamplesSession(t *testing.T) {
	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()

	rows := aggregateSamples([]sample{
		{recordedAtMs: day, sessionID: "ses_2", provider: "openai", model: "gpt", inputTokens: 100, outputTokens: 10, reasoningTokens: 5, cacheReadTokens: 20, cacheWriteTokens: 1, totalTokens: 136},
		{recordedAtMs: day, sessionID: "ses_1", provider: "openai", model: "gpt", inputTokens: 100, outputTokens: 10, reasoningTokens: 5, cacheReadTokens: 20, cacheWriteTokens: 1, totalTokens: 136},
		{recordedAtMs: day, sessionID: "ses_1", provider: "openai", model: "gpt", inputTokens: 200, outputTokens: 20, reasoningTokens: 6, cacheReadTokens: 30, cacheWriteTokens: 2, totalTokens: 258},
		{recordedAtMs: day, sessionID: "ses_2", provider: "openai", model: "gpt", throughputTokens: 100, durationMs: 1000, tokensPerSecond: 100},
		{recordedAtMs: day, sessionID: "ses_1", provider: "openai", model: "gpt", throughputTokens: 100, durationMs: 10000, tokensPerSecond: 10},
		{recordedAtMs: day, sessionID: "ses_1", provider: "openai", model: "gpt", throughputTokens: 100, durationMs: 1000, tokensPerSecond: 100},
	}, groupBySession)

	if len(rows) != 2 {
		t.Fatalf("got %d rows", len(rows))
	}
	row := rows[0]
	if row.day != "2026-04-24" || row.sessionID != "ses_1" || row.provider != "openai" || row.model != "gpt" {
		t.Fatalf("unexpected session row: %+v", row)
	}
	if row.inputTokens != "300" || row.outputTokens != "30" || row.reasoningTokens != "11" || row.cacheReadTokens != "50" || row.cacheWriteTokens != "3" || row.totalTokens != "394" {
		t.Fatalf("unexpected tokens: %+v", row)
	}
	if row.tpsAvg != "18.18" || row.tpsMean != "55.00" || row.tpsMedian != "55.00" {
		t.Fatalf("unexpected tps: %+v", row)
	}
}

func TestRenderTable(t *testing.T) {
	output := renderTable([]tableRow{{
		day:              "2026-04-24",
		provider:         "openai",
		model:            "gpt",
		tpsAvg:           "18.18",
		tpsMean:          "55.00",
		tpsMedian:        "55.00",
		inputTokens:      "300",
		outputTokens:     "30",
		reasoningTokens:  "11",
		cacheReadTokens:  "50",
		cacheWriteTokens: "3",
		totalTokens:      "394",
	}}, groupByNone)

	for _, expected := range []string{"day", "provider", "model", "tps avg", "tps mean", "tps median", "input", "output", "reasoning", "cache read", "cache write", "total", "2026-04-24", "18.18", "394"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing %q in %q", expected, output)
		}
	}
	if strings.Contains(output, "hour") || strings.Contains(output, "12:00") {
		t.Fatalf("unexpected hourly output: %q", output)
	}
}

func TestRenderTableHourly(t *testing.T) {
	output := renderTable([]tableRow{{
		day:              "2026-04-24",
		hour:             "12:00",
		provider:         "openai",
		model:            "gpt",
		tpsAvg:           "18.18",
		tpsMean:          "55.00",
		tpsMedian:        "55.00",
		inputTokens:      "300",
		outputTokens:     "30",
		reasoningTokens:  "11",
		cacheReadTokens:  "50",
		cacheWriteTokens: "3",
		totalTokens:      "394",
	}}, groupByHour)

	for _, expected := range []string{"day", "hour", "provider", "model", "tps avg", "input", "2026-04-24", "12:00", "18.18", "394"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing %q in %q", expected, output)
		}
	}
}

func TestRenderTableSession(t *testing.T) {
	output := renderTable([]tableRow{{
		day:              "2026-04-24",
		sessionID:        "session_1234567890",
		provider:         "openai",
		model:            "openai/gpt-5.5",
		tpsAvg:           "18.18",
		tpsMean:          "55.00",
		tpsMedian:        "55.00",
		inputTokens:      "300",
		outputTokens:     "30",
		reasoningTokens:  "11",
		cacheReadTokens:  "50",
		cacheWriteTokens: "3",
		totalTokens:      "394",
	}}, groupBySession)

	for _, expected := range []string{"day", "session id", "provider", "model", "tps avg", "2026-04-24", "34567890", "gpt-5.5", "18.18", "394"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing %q in %q", expected, output)
		}
	}
	for _, unexpected := range []string{"session_1234567890", "openai/gpt-5.5"} {
		if strings.Contains(output, unexpected) {
			t.Fatalf("unexpected %q in %q", unexpected, output)
		}
	}
}

func TestParseTableOptionsDefaultsInteractiveToWeek(t *testing.T) {
	options, err := parseTableOptions([]string{"--db-path", "tokens.sqlite"}, io.Discard, false, periodWeek)
	if err != nil {
		t.Fatal(err)
	}
	if options.period != periodWeek {
		t.Fatalf("period = %q, want %q", options.period, periodWeek)
	}
}

func TestDisplaySessionID(t *testing.T) {
	if got := displaySessionID("session_1234567890"); got != "34567890" {
		t.Fatalf("displaySessionID = %q", got)
	}
	if got := displaySessionID("ses_1"); got != "ses_1" {
		t.Fatalf("displaySessionID short = %q", got)
	}
}

func TestDisplayModel(t *testing.T) {
	if got := displayModel("openai/gpt-5.5"); got != "gpt-5.5" {
		t.Fatalf("displayModel = %q", got)
	}
	if got := displayModel("gpt"); got != "gpt" {
		t.Fatalf("displayModel plain = %q", got)
	}
}

func createTokenEventsTable(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		CREATE TABLE oc_token_events (
			recorded_at_ms INTEGER NOT NULL,
			session_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			input_tokens INTEGER NOT NULL,
			output_tokens INTEGER NOT NULL,
			reasoning_tokens INTEGER NOT NULL,
			cache_read_tokens INTEGER NOT NULL,
			cache_write_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func insertTokenEvent(t *testing.T, db *sql.DB, recordedAtMs int64, sessionID string, inputTokens int64, outputTokens int64, reasoningTokens int64, cacheReadTokens int64, cacheWriteTokens int64, totalTokens int64) {
	t.Helper()
	_, err := db.Exec(
		"INSERT INTO oc_token_events (recorded_at_ms, session_id, provider, model, input_tokens, output_tokens, reasoning_tokens, cache_read_tokens, cache_write_tokens, total_tokens) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		recordedAtMs,
		sessionID,
		"openai",
		"gpt",
		inputTokens,
		outputTokens,
		reasoningTokens,
		cacheReadTokens,
		cacheWriteTokens,
		totalTokens,
	)
	if err != nil {
		t.Fatal(err)
	}
}

func createTpsSamplesTable(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		CREATE TABLE oc_tps_samples (
			recorded_at_ms INTEGER NOT NULL,
			session_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			total_tokens INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL,
			tokens_per_second REAL NOT NULL
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func insertTpsSample(t *testing.T, db *sql.DB, recordedAtMs int64, sessionID string, totalTokens int64, durationMs int64, tokensPerSecond float64) {
	t.Helper()
	_, err := db.Exec(
		"INSERT INTO oc_tps_samples (recorded_at_ms, session_id, provider, model, total_tokens, duration_ms, tokens_per_second) VALUES (?, ?, ?, ?, ?, ?, ?)",
		recordedAtMs,
		sessionID,
		"openai",
		"gpt",
		totalTokens,
		durationMs,
		tokensPerSecond,
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunTableQueriesDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "oc-tps.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	createTokenEventsTable(t, db)
	createTpsSamplesTable(t, db)

	recordedAtMs := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertTokenEvent(t, db, recordedAtMs, "ses_1", 100, 10, 5, 20, 1, 136)
	insertTokenEvent(t, db, recordedAtMs, "ses_1", 200, 20, 6, 30, 2, 258)
	insertTpsSample(t, db, recordedAtMs, "ses_1", 100, 1000, 100)
	insertTpsSample(t, db, recordedAtMs, "ses_1", 100, 10000, 10)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err = runTable(
		context.Background(),
		[]string{"--db-path", dbPath, "--day"},
		&output,
		io.Discard,
		time.Date(2026, 4, 24, 15, 0, 0, 0, time.Local),
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{"2026-04-24", "openai", "gpt", "18.18", "55.00", "300", "30", "11", "50", "3", "394"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing %q in %q", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "12:00") {
		t.Fatalf("unexpected hourly output: %q", output.String())
	}
}

func TestRunTableQueriesDBHourly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "oc-tps.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	createTokenEventsTable(t, db)
	createTpsSamplesTable(t, db)

	recordedAtMs := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertTokenEvent(t, db, recordedAtMs, "ses_1", 100, 10, 5, 20, 1, 136)
	insertTpsSample(t, db, recordedAtMs, "ses_1", 100, 1000, 100)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err = runTable(
		context.Background(),
		[]string{"--db-path", dbPath, "--day", "--group-by=hour"},
		&output,
		io.Discard,
		time.Date(2026, 4, 24, 13, 0, 0, 0, time.Local),
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{"hour", "2026-04-24", "12:00", "openai", "gpt", "100.00", "136"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing %q in %q", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "13:00") {
		t.Fatalf("unexpected empty hourly row: %q", output.String())
	}
}

func TestRunTableQueriesDBSession(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "oc-tps.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	createTokenEventsTable(t, db)
	createTpsSamplesTable(t, db)

	recordedAtMs := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertTokenEvent(t, db, recordedAtMs, "ses_1", 100, 10, 5, 20, 1, 136)
	insertTokenEvent(t, db, recordedAtMs, "ses_2", 200, 20, 6, 30, 2, 258)
	insertTpsSample(t, db, recordedAtMs, "ses_1", 100, 1000, 100)
	insertTpsSample(t, db, recordedAtMs, "ses_2", 200, 2000, 100)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err = runTable(
		context.Background(),
		[]string{"--db-path", dbPath, "--day", "--group-by=session"},
		&output,
		io.Discard,
		time.Date(2026, 4, 24, 13, 0, 0, 0, time.Local),
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{"session id", "2026-04-24", "ses_1", "ses_2", "openai", "gpt", "100.00", "136", "258"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing %q in %q", expected, output.String())
		}
	}
}

func TestFilterSamples(t *testing.T) {
	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	otherDay := time.Date(2026, 4, 23, 12, 0, 0, 0, time.Local).UnixMilli()
	queryFilters := filters{}
	if err := queryFilters.sessionIDs.Set("ses_1,ses_2"); err != nil {
		t.Fatal(err)
	}
	if err := queryFilters.providers.Set("openai"); err != nil {
		t.Fatal(err)
	}
	if err := queryFilters.models.Set("gpt"); err != nil {
		t.Fatal(err)
	}
	if err := queryFilters.days.Set("2026-04-24"); err != nil {
		t.Fatal(err)
	}

	filtered, err := filterSamples([]sample{
		{recordedAtMs: day, sessionID: "ses_1", provider: "openai", model: "gpt"},
		{recordedAtMs: day, sessionID: "ses_3", provider: "openai", model: "gpt"},
		{recordedAtMs: day, sessionID: "ses_1", provider: "github-copilot", model: "gpt"},
		{recordedAtMs: day, sessionID: "ses_1", provider: "openai", model: "other"},
		{recordedAtMs: otherDay, sessionID: "ses_1", provider: "openai", model: "gpt"},
	}, queryFilters)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 {
		t.Fatalf("got %d rows", len(filtered))
	}
	if filtered[0].sessionID != "ses_1" || filtered[0].provider != "openai" || filtered[0].model != "gpt" {
		t.Fatalf("unexpected row: %+v", filtered[0])
	}
}

func TestFilterSamplesInvalidDay(t *testing.T) {
	queryFilters := filters{}
	if err := queryFilters.days.Set("04-24-2026"); err != nil {
		t.Fatal(err)
	}
	_, err := filterSamples(nil, queryFilters)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid --filter-day") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunTableMissingDB(t *testing.T) {
	var stderr bytes.Buffer
	err := runTable(
		context.Background(),
		[]string{"--db-path", filepath.Join(t.TempDir(), "missing.sqlite"), "--day"},
		io.Discard,
		&stderr,
		time.Date(2026, 4, 24, 15, 0, 0, 0, time.Local),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "db not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunTableInvalidGroupBy(t *testing.T) {
	var stderr bytes.Buffer
	err := runTable(
		context.Background(),
		[]string{"--db-path", filepath.Join(t.TempDir(), "missing.sqlite"), "--day", "--group-by=provider"},
		io.Discard,
		&stderr,
		time.Date(2026, 4, 24, 15, 0, 0, 0, time.Local),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid --group-by") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunTableRepeatedGroupBy(t *testing.T) {
	var stderr bytes.Buffer
	err := runTable(
		context.Background(),
		[]string{"--db-path", filepath.Join(t.TempDir(), "missing.sqlite"), "--day", "--group-by=hour", "--group-by=session"},
		io.Discard,
		&stderr,
		time.Date(2026, 4, 24, 15, 0, 0, 0, time.Local),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "only one --group-by") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormatTokens(t *testing.T) {
	cases := []struct {
		value int64
		want  string
	}{
		{0, ""},
		{1, "1"},
		{999, "999"},
		{1000, "1K"},
		{1500, "1K"},
		{687979, "687K"},
		{999999, "999K"},
		{1_000_000, "1M"},
		{6835769, "6M"},
		{1_999_999, "1M"},
	}
	for _, tc := range cases {
		if got := formatTokens(tc.value); got != tc.want {
			t.Errorf("formatTokens(%d) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestAggregateSamplesCompactUnits(t *testing.T) {
	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	rows := aggregateSamples([]sample{
		{recordedAtMs: day, sessionID: "ses_1", provider: "openai", model: "gpt", inputTokens: 213457, outputTokens: 8860, reasoningTokens: 0, cacheReadTokens: 465662, cacheWriteTokens: 0, totalTokens: 687979},
		{recordedAtMs: day, sessionID: "ses_2", provider: "openai", model: "gpt", inputTokens: 608954, outputTokens: 29214, reasoningTokens: 26977, cacheReadTokens: 6170624, cacheWriteTokens: 0, totalTokens: 6835769},
	}, groupBySession)

	if len(rows) != 2 {
		t.Fatalf("got %d rows", len(rows))
	}

	first := rows[0]
	if first.sessionID != "ses_1" {
		t.Fatalf("unexpected row order: %+v", first)
	}
	if first.inputTokens != "213K" || first.outputTokens != "8K" || first.reasoningTokens != "" || first.cacheReadTokens != "465K" || first.cacheWriteTokens != "" || first.totalTokens != "687K" {
		t.Fatalf("unexpected ses_1 tokens: %+v", first)
	}

	second := rows[1]
	if second.sessionID != "ses_2" {
		t.Fatalf("unexpected row order: %+v", second)
	}
	if second.inputTokens != "608K" || second.outputTokens != "29K" || second.reasoningTokens != "26K" || second.cacheReadTokens != "6M" || second.cacheWriteTokens != "" || second.totalTokens != "6M" {
		t.Fatalf("unexpected ses_2 tokens: %+v", second)
	}
}
