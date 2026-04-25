package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tokeninspector-cli/internal/db"
	_ "modernc.org/sqlite"
)

func newTestDBPath(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "oc-tps.sqlite")
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := conn.Exec(fmt.Sprintf(`
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
		db.TableTokenEvents,
		db.ColRecordedAtMs, db.ColSessionID, db.ColMessageID, db.ColProvider, db.ColModel,
		db.ColInputTokens, db.ColOutputTokens, db.ColReasoningTokens,
		db.ColCacheReadTokens, db.ColCacheWriteTokens, db.ColTotalTokens,
	)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(fmt.Sprintf(`
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
		db.TableTpsSamples,
		db.ColRecordedAtMs, db.ColSessionID, db.ColMessageID, db.ColProvider, db.ColModel,
		db.ColOutputTokens, db.ColReasoningTokens, db.ColTotalTokens,
		db.ColDurationMs, "ttft_ms", db.ColTokensPerSecond,
	)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(fmt.Sprintf(`
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
		db.TableLLMRequests,
		db.ColRecordedAtMs, db.ColSessionID, db.ColMessageID, db.ColProvider, db.ColModel,
		db.ColAttemptIndex, db.ColThinkingLevel,
	)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(fmt.Sprintf("PRAGMA user_version = %d", db.SupportedSchemaVersion)); err != nil {
		t.Fatal(err)
	}
	return dbPath
}

func runTable(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
	return runWithTime(ctx, append([]string{"table"}, args...), stdout, stderr, now)
}

func insertTokenEvent(t *testing.T, dbConn *sql.DB, recordedAtMs int64, sessionID, provider, model string, input, output, reasoning, cacheRead, cacheWrite, total int64) {
	t.Helper()
	recordedAt := time.UnixMilli(recordedAtMs).UTC().Format(time.RFC3339)
	_, err := dbConn.Exec(fmt.Sprintf(
		"INSERT INTO %s (recorded_at, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		db.TableTokenEvents,
		db.ColRecordedAtMs, db.ColSessionID, db.ColMessageID, db.ColProvider, db.ColModel,
		db.ColInputTokens, db.ColOutputTokens, db.ColReasoningTokens,
		db.ColCacheReadTokens, db.ColCacheWriteTokens, db.ColTotalTokens,
	), recordedAt, recordedAtMs, sessionID, nextMsgID(), provider, model, input, output, reasoning, cacheRead, cacheWrite, total)
	if err != nil {
		t.Fatal(err)
	}
}

func insertTpsSample(t *testing.T, dbConn *sql.DB, recordedAtMs int64, sessionID, provider, model string, totalTokens, durationMs int64, tokensPerSecond float64) {
	t.Helper()
	recordedAt := time.UnixMilli(recordedAtMs).UTC().Format(time.RFC3339)
	_, err := dbConn.Exec(fmt.Sprintf(
		"INSERT INTO %s (recorded_at, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		db.TableTpsSamples,
		db.ColRecordedAtMs, db.ColSessionID, db.ColMessageID, db.ColProvider, db.ColModel,
		db.ColOutputTokens, db.ColReasoningTokens, db.ColTotalTokens,
		db.ColDurationMs, "ttft_ms", db.ColTokensPerSecond,
	), recordedAt, recordedAtMs, sessionID, nextMsgID(), provider, model, 0, 0, totalTokens, durationMs, 0, tokensPerSecond)
	if err != nil {
		t.Fatal(err)
	}
}

func insertRequest(t *testing.T, dbConn *sql.DB, recordedAtMs int64, sessionID, provider, model string, attemptIndex int64, thinkingLevel string) {
	t.Helper()
	recordedAt := time.UnixMilli(recordedAtMs).UTC().Format(time.RFC3339)
	_, err := dbConn.Exec(fmt.Sprintf(
		"INSERT INTO %s (recorded_at, %s, %s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		db.TableLLMRequests,
		db.ColRecordedAtMs, db.ColSessionID, db.ColMessageID, db.ColProvider, db.ColModel,
		db.ColAttemptIndex, db.ColThinkingLevel,
	), recordedAt, recordedAtMs, sessionID, nextMsgID(), provider, model, attemptIndex, thinkingLevel)
	if err != nil {
		t.Fatal(err)
	}
}

var msgCounter int

func nextMsgID() string {
	msgCounter++
	return fmt.Sprintf("msg_%d", msgCounter)
}

func TestRunTableQueriesDB(t *testing.T) {
	dbPath := newTestDBPath(t)
	dbConn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	recordedAtMs := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertTokenEvent(t, dbConn, recordedAtMs, "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertTokenEvent(t, dbConn, recordedAtMs, "ses_1", "openai", "gpt", 200, 20, 6, 30, 2, 258)
	insertTpsSample(t, dbConn, recordedAtMs, "ses_1", "openai", "gpt", 100, 1000, 100)
	insertTpsSample(t, dbConn, recordedAtMs, "ses_1", "openai", "gpt", 100, 10000, 10)
	insertRequest(t, dbConn, recordedAtMs, "ses_1", "openai", "gpt", 1, "low")
	insertRequest(t, dbConn, recordedAtMs, "ses_1", "openai", "gpt", 2, "high")

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

	for _, expected := range []string{"2026-04-24", "openai", "gpt", "18.18", "55.00", "300", "30", "11", "50", "3", "394", "requests", "retries"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing %q in %q", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "12:00") {
		t.Fatalf("unexpected hourly output: %q", output.String())
	}
}

func TestRunTableQueriesDBHourly(t *testing.T) {
	dbPath := newTestDBPath(t)
	dbConn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	recordedAtMs := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertTokenEvent(t, dbConn, recordedAtMs, "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertTpsSample(t, dbConn, recordedAtMs, "ses_1", "openai", "gpt", 100, 1000, 100)
	insertRequest(t, dbConn, recordedAtMs, "ses_1", "openai", "gpt", 1, "low")
	insertRequest(t, dbConn, recordedAtMs, "ses_1", "openai", "gpt", 2, "high")

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

	for _, expected := range []string{"hour", "2026-04-24", "12:00", "openai", "gpt", "100.00", "136", "requests", "retries"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing %q in %q", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "13:00") {
		t.Fatalf("unexpected empty hourly row: %q", output.String())
	}
}

func TestRunTableQueriesDBSession(t *testing.T) {
	dbPath := newTestDBPath(t)
	dbConn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	recordedAtMs := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertTokenEvent(t, dbConn, recordedAtMs, "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertTokenEvent(t, dbConn, recordedAtMs, "ses_2", "openai", "gpt", 200, 20, 6, 30, 2, 258)
	insertTpsSample(t, dbConn, recordedAtMs, "ses_1", "openai", "gpt", 100, 1000, 100)
	insertTpsSample(t, dbConn, recordedAtMs, "ses_2", "openai", "gpt", 200, 2000, 100)
	insertRequest(t, dbConn, recordedAtMs, "ses_1", "openai", "gpt", 1, "low")
	insertRequest(t, dbConn, recordedAtMs, "ses_1", "openai", "gpt", 2, "high")

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

	for _, expected := range []string{"session id", "thinking", "2026-04-24", "ses_1", "ses_2", "high", "openai", "gpt", "100.00", "136", "258", "requests", "retries"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing %q in %q", expected, output.String())
		}
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
