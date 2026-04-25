package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

func Open(dbPath string) (*sql.DB, error) {
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("db not found: %s", absPath)
		}
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("db path is a directory: %s", absPath)
	}

	fileURL := url.URL{Scheme: "file", Path: absPath}
	query := fileURL.Query()
	query.Set("mode", "ro")
	query.Set("_pragma", "busy_timeout(5000)")
	query.Set("_pragma", "query_only(true)")
	query.Set("_pragma", "foreign_keys(on)")
	fileURL.RawQuery = query.Encode()

	db, err := sql.Open("sqlite", fileURL.String())
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("db ping failed: %w", err)
	}

	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("schema version check failed: %w", err)
	}
	if version != SupportedSchemaVersion {
		if version == 0 {
			// Legacy DB created before schema versioning; verify required tables exist.
			if err := verifyRequiredTables(ctx, db); err != nil {
				_ = db.Close()
				return nil, fmt.Errorf("db missing required tables (schema version 0): %w", err)
			}
			return db, nil
		}
		_ = db.Close()
		return nil, fmt.Errorf("unsupported schema version %d (expected %d): run a newer plugin or delete the db", version, SupportedSchemaVersion)
	}

	return db, nil
}

func verifyRequiredTables(ctx context.Context, db *sql.DB) error {
	for _, name := range []string{TableTokenEvents, TableTpsSamples} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return fmt.Errorf("missing table %s", name)
		}
	}
	return nil
}
