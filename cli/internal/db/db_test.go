package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tokeninspector.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			recorded_at TEXT NOT NULL,
			%s INTEGER NOT NULL,
			%s TEXT NOT NULL,
			%s TEXT NOT NULL,
			%s TEXT NOT NULL DEFAULT 'unknown',
			%s TEXT NOT NULL DEFAULT 'unknown',
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL
		)
	`,
		TableTokenEvents,
		ColRecordedAtMs, ColSessionID, ColMessageID, ColProvider, ColModel,
		ColInputTokens, ColOutputTokens, ColReasoningTokens,
		ColCacheReadTokens, ColCacheWriteTokens, ColTotalTokens,
	)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			recorded_at TEXT NOT NULL,
			%s INTEGER NOT NULL,
			%s TEXT NOT NULL,
			%s TEXT NOT NULL UNIQUE,
			%s TEXT NOT NULL DEFAULT 'unknown',
			%s TEXT NOT NULL DEFAULT 'unknown',
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			%s REAL NOT NULL
		)
	`,
		TableTpsSamples,
		ColRecordedAtMs, ColSessionID, ColMessageID, ColProvider, ColModel,
		ColOutputTokens, ColReasoningTokens, ColTotalTokens,
		ColDurationMs, "ttft_ms", ColTokensPerSecond,
	)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			recorded_at TEXT NOT NULL,
			%s INTEGER NOT NULL,
			%s TEXT NOT NULL,
			%s TEXT NOT NULL,
			%s TEXT NOT NULL DEFAULT 'unknown',
			%s TEXT NOT NULL DEFAULT 'unknown',
			%s INTEGER NOT NULL,
			%s TEXT NOT NULL DEFAULT 'unknown'
		)
	`,
		TableLLMRequests,
		ColRecordedAtMs, ColSessionID, ColMessageID, ColProvider, ColModel,
		ColAttemptIndex, ColThinkingLevel,
	)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			recorded_at TEXT NOT NULL,
			%s INTEGER NOT NULL,
			%s TEXT NOT NULL,
			%s TEXT NOT NULL,
			%s TEXT NOT NULL DEFAULT 'unknown',
			%s TEXT NOT NULL DEFAULT 'unknown',
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			UNIQUE(%s, %s)
		)
	`,
		TablePiTokenEvents,
		ColRecordedAtMs, ColSessionID, ColMessageID, ColProvider, ColModel,
		ColInputTokens, ColOutputTokens, ColReasoningTokens,
		ColCacheReadTokens, ColCacheWriteTokens, ColTotalTokens,
		ColSessionID, ColMessageID,
	)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			recorded_at TEXT NOT NULL,
			%s INTEGER NOT NULL,
			%s TEXT NOT NULL,
			%s TEXT NOT NULL UNIQUE,
			%s TEXT NOT NULL DEFAULT 'unknown',
			%s TEXT NOT NULL DEFAULT 'unknown',
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			%s INTEGER NOT NULL,
			%s REAL NOT NULL
		)
	`,
		TablePiTpsSamples,
		ColRecordedAtMs, ColSessionID, ColMessageID, ColProvider, ColModel,
		ColOutputTokens, ColReasoningTokens, ColTotalTokens,
		ColDurationMs, "ttft_ms", ColTokensPerSecond,
	)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			recorded_at TEXT NOT NULL,
			%s INTEGER NOT NULL,
			%s TEXT NOT NULL,
			%s TEXT NOT NULL,
			%s TEXT NOT NULL DEFAULT 'unknown',
			%s TEXT NOT NULL DEFAULT 'unknown',
			%s INTEGER NOT NULL,
			%s TEXT NOT NULL DEFAULT 'unknown'
		)
	`,
		TablePiLLMRequests,
		ColRecordedAtMs, ColSessionID, ColMessageID, ColProvider, ColModel,
		ColAttemptIndex, ColThinkingLevel,
	)); err != nil {
		t.Fatal(err)
	}
	for _, tableName := range []string{TableToolCalls, TablePiToolCalls} {
		if _, err := db.Exec(fmt.Sprintf(`
			CREATE TABLE %s (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				recorded_at TEXT NOT NULL,
				%s INTEGER NOT NULL,
				%s TEXT NOT NULL,
				%s TEXT NOT NULL,
				%s TEXT NOT NULL,
				%s TEXT NOT NULL DEFAULT 'unknown',
				%s TEXT NOT NULL DEFAULT 'unknown',
				%s TEXT NOT NULL DEFAULT 'unknown',
				%s TEXT NOT NULL,
				UNIQUE(%s, %s, %s)
			)
		`,
			tableName,
			ColRecordedAtMs, ColSessionID, ColMessageID, ColToolCallID, ColToolName,
			ColProvider, ColModel, ColStatus, ColSessionID, ColToolCallID, ColStatus,
		)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", SupportedSchemaVersion)); err != nil {
		t.Fatal(err)
	}
	return db
}

var testMsgCounter int

func nextMsgID() string {
	testMsgCounter++
	return fmt.Sprintf("msg_%d", testMsgCounter)
}

func insertTokenEvent(t *testing.T, db *sql.DB, recordedAtMs int64, sessionID, provider, model string, input, output, reasoning, cacheRead, cacheWrite, total int64) {
	t.Helper()
	recordedAt := time.UnixMilli(recordedAtMs).UTC().Format(time.RFC3339)
	_, err := db.Exec(fmt.Sprintf(
		"INSERT INTO %s (recorded_at, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		TableTokenEvents,
		ColRecordedAtMs, ColSessionID, ColMessageID, ColProvider, ColModel,
		ColInputTokens, ColOutputTokens, ColReasoningTokens,
		ColCacheReadTokens, ColCacheWriteTokens, ColTotalTokens,
	), recordedAt, recordedAtMs, sessionID, nextMsgID(), provider, model, input, output, reasoning, cacheRead, cacheWrite, total)
	if err != nil {
		t.Fatal(err)
	}
}

func insertTpsSample(t *testing.T, db *sql.DB, recordedAtMs int64, sessionID, provider, model string, totalTokens, durationMs int64, tokensPerSecond float64) {
	t.Helper()
	recordedAt := time.UnixMilli(recordedAtMs).UTC().Format(time.RFC3339)
	_, err := db.Exec(fmt.Sprintf(
		"INSERT INTO %s (recorded_at, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		TableTpsSamples,
		ColRecordedAtMs, ColSessionID, ColMessageID, ColProvider, ColModel,
		ColOutputTokens, ColReasoningTokens, ColTotalTokens,
		ColDurationMs, "ttft_ms", ColTokensPerSecond,
	), recordedAt, recordedAtMs, sessionID, nextMsgID(), provider, model, 0, 0, totalTokens, durationMs, 0, tokensPerSecond)
	if err != nil {
		t.Fatal(err)
	}
}

func insertRequest(t *testing.T, db *sql.DB, recordedAtMs int64, sessionID, provider, model string, attemptIndex int64, thinkingLevel string) {
	t.Helper()
	recordedAt := time.UnixMilli(recordedAtMs).UTC().Format(time.RFC3339)
	_, err := db.Exec(fmt.Sprintf(
		"INSERT INTO %s (recorded_at, %s, %s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		TableLLMRequests,
		ColRecordedAtMs, ColSessionID, ColMessageID, ColProvider, ColModel,
		ColAttemptIndex, ColThinkingLevel,
	), recordedAt, recordedAtMs, sessionID, nextMsgID(), provider, model, attemptIndex, thinkingLevel)
	if err != nil {
		t.Fatal(err)
	}
}

func insertPiTokenEvent(t *testing.T, db *sql.DB, recordedAtMs int64, sessionID, provider, model string, input, output, reasoning, cacheRead, cacheWrite, total int64) {
	t.Helper()
	recordedAt := time.UnixMilli(recordedAtMs).UTC().Format(time.RFC3339)
	_, err := db.Exec(fmt.Sprintf(
		"INSERT INTO %s (recorded_at, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		TablePiTokenEvents,
		ColRecordedAtMs, ColSessionID, ColMessageID, ColProvider, ColModel,
		ColInputTokens, ColOutputTokens, ColReasoningTokens,
		ColCacheReadTokens, ColCacheWriteTokens, ColTotalTokens,
	), recordedAt, recordedAtMs, sessionID, nextMsgID(), provider, model, input, output, reasoning, cacheRead, cacheWrite, total)
	if err != nil {
		t.Fatal(err)
	}
}

func insertPiTpsSample(t *testing.T, db *sql.DB, recordedAtMs int64, sessionID, provider, model string, totalTokens, durationMs int64, tokensPerSecond float64) {
	t.Helper()
	recordedAt := time.UnixMilli(recordedAtMs).UTC().Format(time.RFC3339)
	_, err := db.Exec(fmt.Sprintf(
		"INSERT INTO %s (recorded_at, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		TablePiTpsSamples,
		ColRecordedAtMs, ColSessionID, ColMessageID, ColProvider, ColModel,
		ColOutputTokens, ColReasoningTokens, ColTotalTokens,
		ColDurationMs, "ttft_ms", ColTokensPerSecond,
	), recordedAt, recordedAtMs, sessionID, nextMsgID(), provider, model, 0, 0, totalTokens, durationMs, 0, tokensPerSecond)
	if err != nil {
		t.Fatal(err)
	}
}

func insertPiRequest(t *testing.T, db *sql.DB, recordedAtMs int64, sessionID, provider, model string, attemptIndex int64, thinkingLevel string) {
	t.Helper()
	recordedAt := time.UnixMilli(recordedAtMs).UTC().Format(time.RFC3339)
	_, err := db.Exec(fmt.Sprintf(
		"INSERT INTO %s (recorded_at, %s, %s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		TablePiLLMRequests,
		ColRecordedAtMs, ColSessionID, ColMessageID, ColProvider, ColModel,
		ColAttemptIndex, ColThinkingLevel,
	), recordedAt, recordedAtMs, sessionID, nextMsgID(), provider, model, attemptIndex, thinkingLevel)
	if err != nil {
		t.Fatal(err)
	}
}

func insertToolCall(t *testing.T, db *sql.DB, table string, recordedAtMs int64, sessionID, provider, model, toolName, status string) {
	t.Helper()
	recordedAt := time.UnixMilli(recordedAtMs).UTC().Format(time.RFC3339)
	_, err := db.Exec(fmt.Sprintf(
		"INSERT INTO %s (recorded_at, %s, %s, %s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		table,
		ColRecordedAtMs, ColSessionID, ColMessageID, ColToolCallID, ColToolName, ColProvider, ColModel, ColStatus,
	), recordedAt, recordedAtMs, sessionID, nextMsgID(), nextMsgID(), toolName, provider, model, status)
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpenSchemaVersionMismatch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tokeninspector.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf("CREATE TABLE %s (id INTEGER PRIMARY KEY)", TableTokenEvents)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = Open(dbPath)
	if err == nil {
		t.Fatal("expected schema version error")
	}
	if !contains(err.Error(), "db missing required tables") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEventsFilter(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertTokenEvent(t, db, day, "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertTokenEvent(t, db, day, "ses_2", "github-copilot", "claude", 200, 20, 6, 30, 2, 258)

	events, err := Events(context.Background(), db, Filter{
		Start:     time.Date(2026, 4, 24, 0, 0, 0, 0, time.Local),
		Providers: []string{"openai"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Provider != "openai" {
		t.Fatalf("unexpected provider: %s", events[0].Provider)
	}
}

func TestAggregateDaily(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	yesterday := time.Date(2026, 4, 23, 12, 0, 0, 0, time.Local).UnixMilli()

	insertTokenEvent(t, db, yesterday, "ses_1", "anthropic", "claude", 10, 5, 1, 2, 3, 21)
	insertTpsSample(t, db, yesterday, "ses_1", "anthropic", "claude", 50, 1000, 50)
	insertTokenEvent(t, db, day, "ses_2", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertTokenEvent(t, db, day, "ses_2", "openai", "gpt", 200, 20, 6, 30, 2, 258)
	insertTpsSample(t, db, day, "ses_2", "openai", "gpt", 100, 1000, 100)
	insertTpsSample(t, db, day, "ses_2", "openai", "gpt", 100, 10000, 10)
	insertRequest(t, db, day, "ses_2", "openai", "gpt", 1, "low")
	insertRequest(t, db, day, "ses_2", "openai", "gpt", 2, "high")

	rows, err := Aggregate(context.Background(), db, Filter{
		Start: time.Date(2026, 4, 23, 0, 0, 0, 0, time.Local),
	}, GroupByDay)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	// Sorted by day desc
	first := rows[0]
	if first.Day != "2026-04-24" || first.Provider != "openai" || first.Model != "gpt" {
		t.Fatalf("unexpected first row: %+v", first)
	}
	if first.InputTokens != 300 || first.OutputTokens != 30 || first.ReasoningTokens != 11 || first.CacheReadTokens != 50 || first.CacheWriteTokens != 3 || first.TotalTokens != 394 {
		t.Fatalf("unexpected tokens: %+v", first)
	}
	if first.ThroughputTokens != 200 || first.DurationMs != 11000 {
		t.Fatalf("unexpected tps aggregates: %+v", first)
	}
	if first.TpsMean != 55.0 {
		t.Fatalf("unexpected tps mean: %f", first.TpsMean)
	}
	if first.TpsMedian != 55.0 {
		t.Fatalf("unexpected tps median: %f", first.TpsMedian)
	}
	if first.Requests != 1 || first.Retries != 1 {
		t.Fatalf("unexpected requests: %+v", first)
	}

	second := rows[1]
	if second.Day != "2026-04-23" || second.Provider != "anthropic" || second.Model != "claude" {
		t.Fatalf("unexpected second row: %+v", second)
	}
	if second.InputTokens != 10 || second.TotalTokens != 21 {
		t.Fatalf("unexpected second tokens: %+v", second)
	}
}

func TestAggregateHourly(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertTokenEvent(t, db, day, "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertTpsSample(t, db, day, "ses_1", "openai", "gpt", 100, 1000, 100)
	insertRequest(t, db, day, "ses_1", "openai", "gpt", 1, "low")

	rows, err := Aggregate(context.Background(), db, Filter{
		Start: time.Date(2026, 4, 24, 0, 0, 0, 0, time.Local),
	}, GroupByDayHour)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Hour != "12:00" {
		t.Fatalf("unexpected hour: %s", rows[0].Hour)
	}
	if rows[0].InputTokens != 100 {
		t.Fatalf("unexpected tokens: %+v", rows[0])
	}
}

func TestAggregateHourlyInterleaved(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	hour13 := time.Date(2026, 4, 24, 13, 0, 0, 0, time.Local).UnixMilli()
	hour12 := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()

	// OC data at 13:00
	insertTokenEvent(t, db, hour13, "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	// Pi data at 13:00
	insertPiTokenEvent(t, db, hour13, "ses_2", "anthropic", "claude", 200, 20, 10, 40, 2, 272)
	// OC data at 12:00
	insertTokenEvent(t, db, hour12, "ses_3", "openai", "gpt", 50, 5, 1, 10, 0, 66)
	// Pi data at 12:00
	insertPiTokenEvent(t, db, hour12, "ses_4", "anthropic", "claude", 80, 8, 2, 16, 1, 107)

	rows, err := Aggregate(context.Background(), db, Filter{
		Start: time.Date(2026, 4, 24, 0, 0, 0, 0, time.Local),
	}, GroupByDayHour)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4", len(rows))
	}

	// Should be interleaved by hour: 13:00 first (OC, then Pi), then 12:00 (OC, then Pi)
	if rows[0].Hour != "13:00" || rows[0].Harness != "oc" || rows[0].InputTokens != 100 {
		t.Fatalf("unexpected row 0: %+v", rows[0])
	}
	if rows[1].Hour != "13:00" || rows[1].Harness != "pi" || rows[1].InputTokens != 200 {
		t.Fatalf("unexpected row 1: %+v", rows[1])
	}
	if rows[2].Hour != "12:00" || rows[2].Harness != "oc" || rows[2].InputTokens != 50 {
		t.Fatalf("unexpected row 2: %+v", rows[2])
	}
	if rows[3].Hour != "12:00" || rows[3].Harness != "pi" || rows[3].InputTokens != 80 {
		t.Fatalf("unexpected row 3: %+v", rows[3])
	}
}

func TestAggregateSession(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	later := time.Date(2026, 4, 24, 13, 0, 0, 0, time.Local).UnixMilli()

	insertTokenEvent(t, db, day, "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertTokenEvent(t, db, later, "ses_2", "openai", "gpt", 200, 20, 6, 30, 2, 258)
	insertTpsSample(t, db, day, "ses_1", "openai", "gpt", 100, 1000, 100)
	insertTpsSample(t, db, later, "ses_2", "openai", "gpt", 200, 2000, 100)
	insertRequest(t, db, day, "ses_1", "openai", "gpt", 1, "low")
	insertRequest(t, db, day, "ses_1", "openai", "gpt", 2, "high")

	rows, err := Aggregate(context.Background(), db, Filter{
		Start: time.Date(2026, 4, 24, 0, 0, 0, 0, time.Local),
	}, GroupByDaySession)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	// ses_2 has later activity, so it sorts first with LatestAtMs desc
	first := rows[0]
	if first.SessionID != "ses_2" {
		t.Fatalf("unexpected first session: %s", first.SessionID)
	}
	if first.InputTokens != 200 || first.TotalTokens != 258 {
		t.Fatalf("unexpected first tokens: %+v", first)
	}
	if first.ThroughputTokens != 200 || first.DurationMs != 2000 {
		t.Fatalf("unexpected first tps aggregates: %+v", first)
	}
	if first.TpsMean != 100.0 {
		t.Fatalf("unexpected first tps mean: %f", first.TpsMean)
	}
	if first.TpsMedian != 100.0 {
		t.Fatalf("unexpected first tps median: %f", first.TpsMedian)
	}
	if first.ThinkingLevels != "" {
		t.Fatalf("unexpected first thinking levels: %s", first.ThinkingLevels)
	}
	if first.Requests != 0 || first.Retries != 0 {
		t.Fatalf("unexpected first requests: %+v", first)
	}
	if first.LatestAtMs != later {
		t.Fatalf("unexpected first latest at ms: %d, want %d", first.LatestAtMs, later)
	}

	second := rows[1]
	if second.SessionID != "ses_1" {
		t.Fatalf("unexpected second session: %s", second.SessionID)
	}
	if second.InputTokens != 100 || second.TotalTokens != 136 {
		t.Fatalf("unexpected second tokens: %+v", second)
	}
	if second.ThroughputTokens != 100 || second.DurationMs != 1000 {
		t.Fatalf("unexpected second tps aggregates: %+v", second)
	}
	if second.TpsMean != 100.0 {
		t.Fatalf("unexpected second tps mean: %f", second.TpsMean)
	}
	if second.TpsMedian != 100.0 {
		t.Fatalf("unexpected second tps median: %f", second.TpsMedian)
	}
	if second.ThinkingLevels != "low,high" {
		t.Fatalf("unexpected second thinking levels: %s", second.ThinkingLevels)
	}
	if second.Requests != 1 || second.Retries != 1 {
		t.Fatalf("unexpected second requests: %+v", second)
	}
	if second.LatestAtMs != day {
		t.Fatalf("unexpected second latest at ms: %d, want %d", second.LatestAtMs, day)
	}
}

func TestAggregateDailySortsLatestToOldest(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	older := time.Date(2026, 4, 24, 10, 0, 0, 0, time.Local).UnixMilli()
	newer := time.Date(2026, 4, 24, 13, 0, 0, 0, time.Local).UnixMilli()

	insertTokenEvent(t, db, older, "ses_1", "anthropic", "claude", 100, 10, 5, 20, 1, 136)
	insertTokenEvent(t, db, newer, "ses_2", "openai", "gpt", 200, 20, 6, 30, 2, 258)

	rows, err := Aggregate(context.Background(), db, Filter{
		Start: time.Date(2026, 4, 24, 0, 0, 0, 0, time.Local),
	}, GroupByDay)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Provider != "openai" || rows[0].LatestAtMs != newer {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
	if rows[1].Provider != "anthropic" || rows[1].LatestAtMs != older {
		t.Fatalf("unexpected second row: %+v", rows[1])
	}
}

func TestAggregateHourlySortsLatestToOldest(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	older := time.Date(2026, 4, 24, 13, 5, 0, 0, time.Local).UnixMilli()
	newer := time.Date(2026, 4, 24, 13, 55, 0, 0, time.Local).UnixMilli()

	insertTpsSample(t, db, older, "ses_1", "anthropic", "claude", 100, 1000, 100)
	insertTpsSample(t, db, newer, "ses_2", "openai", "gpt", 200, 2000, 100)

	rows, err := Aggregate(context.Background(), db, Filter{
		Start: time.Date(2026, 4, 24, 0, 0, 0, 0, time.Local),
	}, GroupByDayHour)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Provider != "openai" || rows[0].Hour != "13:00" || rows[0].LatestAtMs != newer {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
	if rows[1].Provider != "anthropic" || rows[1].Hour != "13:00" || rows[1].LatestAtMs != older {
		t.Fatalf("unexpected second row: %+v", rows[1])
	}
}

func TestAggregateSessionSortsLatestToOldestAcrossHarnesses(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	older := time.Date(2026, 4, 24, 10, 0, 0, 0, time.Local).UnixMilli()
	newer := time.Date(2026, 4, 24, 13, 0, 0, 0, time.Local).UnixMilli()

	insertRequest(t, db, older, "ses_1", "openai", "gpt", 1, "low")
	insertPiRequest(t, db, newer, "ses_2", "anthropic", "claude", 1, "high")

	rows, err := Aggregate(context.Background(), db, Filter{
		Start: time.Date(2026, 4, 24, 0, 0, 0, 0, time.Local),
	}, GroupByDaySession)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Harness != "pi" || rows[0].SessionID != "ses_2" || rows[0].LatestAtMs != newer {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
	if rows[1].Harness != "oc" || rows[1].SessionID != "ses_1" || rows[1].LatestAtMs != older {
		t.Fatalf("unexpected second row: %+v", rows[1])
	}
}

func TestAggregateCrossHarness(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()

	// OpenCode data
	insertTokenEvent(t, db, day, "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertTpsSample(t, db, day, "ses_1", "openai", "gpt", 100, 1000, 100)
	insertRequest(t, db, day, "ses_1", "openai", "gpt", 1, "low")

	// Pi data
	insertPiTokenEvent(t, db, day, "ses_1", "openai", "gpt", 200, 20, 10, 40, 2, 272)
	insertPiTpsSample(t, db, day, "ses_1", "openai", "gpt", 200, 2000, 100)
	insertPiRequest(t, db, day, "ses_1", "openai", "gpt", 1, "high")

	rows, err := Aggregate(context.Background(), db, Filter{
		Start: time.Date(2026, 4, 24, 0, 0, 0, 0, time.Local),
	}, GroupByDay)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	// Sorted by day desc, then harness asc
	first := rows[0]
	if first.Harness != "oc" {
		t.Fatalf("unexpected harness: %s", first.Harness)
	}
	if first.InputTokens != 100 || first.TotalTokens != 136 {
		t.Fatalf("unexpected oc tokens: %+v", first)
	}
	if first.ThroughputTokens != 100 || first.DurationMs != 1000 {
		t.Fatalf("unexpected oc tps: %+v", first)
	}
	if first.Requests != 1 || first.Retries != 0 {
		t.Fatalf("unexpected oc requests: %+v", first)
	}

	second := rows[1]
	if second.Harness != "pi" {
		t.Fatalf("unexpected harness: %s", second.Harness)
	}
	if second.InputTokens != 200 || second.TotalTokens != 272 {
		t.Fatalf("unexpected pi tokens: %+v", second)
	}
	if second.ThroughputTokens != 200 || second.DurationMs != 2000 {
		t.Fatalf("unexpected pi tps: %+v", second)
	}
	if second.Requests != 1 || second.Retries != 0 {
		t.Fatalf("unexpected pi requests: %+v", second)
	}
}

func TestAggregateToolCalls(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertToolCall(t, db, TableToolCalls, day, "ses_1", "openai", "gpt", "bash", "started")
	insertToolCall(t, db, TableToolCalls, day, "ses_1", "openai", "gpt", "bash", "error")
	insertToolCall(t, db, TablePiToolCalls, day, "ses_2", "anthropic", "claude", "read", "started")
	insertToolCall(t, db, TablePiToolCalls, day, "ses_2", "anthropic", "claude", "read", "completed")

	rows, err := Aggregate(context.Background(), db, Filter{
		Start: time.Date(2026, 4, 24, 0, 0, 0, 0, time.Local),
	}, GroupByDay)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Harness != "oc" || rows[0].ToolCalls != 1 || rows[0].ToolErrors != 1 {
		t.Fatalf("unexpected oc tool counts: %+v", rows[0])
	}
	if rows[1].Harness != "pi" || rows[1].ToolCalls != 1 || rows[1].ToolErrors != 0 {
		t.Fatalf("unexpected pi tool counts: %+v", rows[1])
	}
}

func TestAggregateToolBreakdown(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertToolCall(t, db, TableToolCalls, day, "ses_1", "openai", "gpt", "bash", "started")
	insertToolCall(t, db, TableToolCalls, day, "ses_1", "openai", "gpt", "bash", "completed")
	insertToolCall(t, db, TableToolCalls, day, "ses_1", "openai", "gpt", "read", "started")
	insertToolCall(t, db, TableToolCalls, day, "ses_1", "openai", "gpt", "read", "error")

	rows, err := AggregateToolBreakdown(context.Background(), db, Filter{
		Start: time.Date(2026, 4, 24, 0, 0, 0, 0, time.Local),
	}, GroupByDaySession)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	byTool := map[string]Row{}
	for _, row := range rows {
		byTool[row.ToolName] = row
	}
	if byTool["bash"].ToolCalls != 1 || byTool["bash"].ToolErrors != 0 {
		t.Fatalf("unexpected bash counts: %+v", byTool["bash"])
	}
	if byTool["read"].ToolCalls != 1 || byTool["read"].ToolErrors != 1 {
		t.Fatalf("unexpected read counts: %+v", byTool["read"])
	}
}

func TestAggregateMissingTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tokeninspector.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			id INTEGER PRIMARY KEY,
			%s INTEGER,
			%s TEXT,
			%s TEXT,
			%s TEXT,
			%s INTEGER,
			%s INTEGER,
			%s INTEGER,
			%s INTEGER,
			%s INTEGER,
			%s INTEGER,
			%s INTEGER
		)
	`,
		TableTokenEvents,
		ColRecordedAtMs, ColSessionID, ColMessageID, ColProvider, ColModel,
		ColInputTokens, ColOutputTokens, ColReasoningTokens,
		ColCacheReadTokens, ColCacheWriteTokens, ColTotalTokens,
	)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", SupportedSchemaVersion)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db2, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	rows, err := Aggregate(context.Background(), db2, Filter{
		Start: time.Date(2026, 4, 24, 0, 0, 0, 0, time.Local),
	}, GroupByDay)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestFilterDayRange(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	day20 := time.Date(2026, 4, 20, 12, 0, 0, 0, time.Local).UnixMilli()
	day21 := time.Date(2026, 4, 21, 12, 0, 0, 0, time.Local).UnixMilli()
	day22 := time.Date(2026, 4, 22, 12, 0, 0, 0, time.Local).UnixMilli()

	insertTokenEvent(t, db, day20, "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertTokenEvent(t, db, day21, "ses_2", "anthropic", "claude", 200, 20, 6, 30, 2, 258)
	insertTokenEvent(t, db, day22, "ses_3", "openai", "gpt", 300, 30, 7, 40, 3, 380)

	events, err := Events(context.Background(), db, Filter{
		DayFrom: "2026-04-21",
		DayTo:   "2026-04-21",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Provider != "anthropic" {
		t.Fatalf("unexpected provider: %s", events[0].Provider)
	}
}

func TestFilterDayRangeInclusive(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	day20 := time.Date(2026, 4, 20, 12, 0, 0, 0, time.Local).UnixMilli()
	day21 := time.Date(2026, 4, 21, 12, 0, 0, 0, time.Local).UnixMilli()
	day22 := time.Date(2026, 4, 22, 12, 0, 0, 0, time.Local).UnixMilli()

	insertTokenEvent(t, db, day20, "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertTokenEvent(t, db, day21, "ses_2", "anthropic", "claude", 200, 20, 6, 30, 2, 258)
	insertTokenEvent(t, db, day22, "ses_3", "openai", "gpt", 300, 30, 7, 40, 3, 380)

	events, err := Events(context.Background(), db, Filter{
		DayFrom: "2026-04-20",
		DayTo:   "2026-04-22",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
}

func TestAggregateAllTime(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	dayOld := time.Date(2026, 4, 10, 12, 0, 0, 0, time.Local).UnixMilli()
	dayNew := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()

	insertTokenEvent(t, db, dayOld, "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertTokenEvent(t, db, dayNew, "ses_2", "anthropic", "claude", 200, 20, 6, 30, 2, 258)

	rows, err := Aggregate(context.Background(), db, Filter{
		// Start is zero — all time
	}, GroupByDay)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	// Sorted by day desc
	if rows[0].Day != "2026-04-24" {
		t.Fatalf("unexpected first day: %s", rows[0].Day)
	}
	if rows[1].Day != "2026-04-10" {
		t.Fatalf("unexpected second day: %s", rows[1].Day)
	}
}
