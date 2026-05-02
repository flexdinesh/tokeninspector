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

	"tokeninsights-cli/internal/db"
	_ "modernc.org/sqlite"
)

func newTestDBPath(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
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

func TestMissingDBPath(t *testing.T) {
	var stderr bytes.Buffer
	err := runWithTime(
		context.Background(),
		[]string{"--today"},
		io.Discard,
		&stderr,
		time.Date(2026, 4, 24, 15, 0, 0, 0, time.Local),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing --db-path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInvalidPeriod(t *testing.T) {
	var stderr bytes.Buffer
	err := runWithTime(
		context.Background(),
		[]string{"--db-path", filepath.Join(t.TempDir(), "test.sqlite"), "--today", "--week"},
		io.Discard,
		&stderr,
		time.Date(2026, 4, 24, 15, 0, 0, 0, time.Local),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "choose exactly one") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnexpectedArgument(t *testing.T) {
	var stderr bytes.Buffer
	err := runWithTime(
		context.Background(),
		[]string{"--db-path", filepath.Join(t.TempDir(), "test.sqlite"), "--today", "unexpected"},
		io.Discard,
		&stderr,
		time.Date(2026, 4, 24, 15, 0, 0, 0, time.Local),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unexpected argument") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHelp(t *testing.T) {
	var stdout bytes.Buffer
	err := runWithTime(
		context.Background(),
		[]string{"--help"},
		&stdout,
		io.Discard,
		time.Date(2026, 4, 24, 15, 0, 0, 0, time.Local),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "usage:") {
		t.Fatalf("expected usage in output: %q", stdout.String())
	}
}
