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
	dbPath := filepath.Join(t.TempDir(), "oc-tps.sqlite")
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

func TestOpenSchemaVersionMismatch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "oc-tps.sqlite")
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

func TestAggregateSession(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertTokenEvent(t, db, day, "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertTokenEvent(t, db, day, "ses_2", "openai", "gpt", 200, 20, 6, 30, 2, 258)
	insertTpsSample(t, db, day, "ses_1", "openai", "gpt", 100, 1000, 100)
	insertTpsSample(t, db, day, "ses_2", "openai", "gpt", 200, 2000, 100)
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

	first := rows[0]
	if first.SessionID != "ses_1" {
		t.Fatalf("unexpected session: %s", first.SessionID)
	}
	if first.InputTokens != 100 || first.TotalTokens != 136 {
		t.Fatalf("unexpected tokens: %+v", first)
	}
	if first.ThinkingLevels != "low,high" {
		t.Fatalf("unexpected thinking levels: %s", first.ThinkingLevels)
	}
}

func TestAggregateMissingTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "oc-tps.sqlite")
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
