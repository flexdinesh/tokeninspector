package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Filter holds CLI filter criteria. All slices use exact-match IN clauses.
type Filter struct {
	Start      time.Time
	SessionIDs []string
	Providers  []string
	Models     []string
	Days       []string // YYYY-MM-DD local
}

// Event is a single durable token-event row from oc_token_events.
type Event struct {
	RecordedAtMs     int64
	SessionID        string
	Provider         string
	Model            string
	InputTokens      int64
	OutputTokens     int64
	ReasoningTokens  int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	TotalTokens      int64
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

func buildFilterArgs(f Filter) ([]any, []string) {
	var args []any
	var where []string

	where = append(where, fmt.Sprintf("%s >= ?", ColRecordedAtMs))
	args = append(args, f.Start.UnixMilli())

	if len(f.SessionIDs) > 0 {
		where = append(where, fmt.Sprintf("%s IN (%s)", ColSessionID, placeholders(len(f.SessionIDs))))
		for _, id := range f.SessionIDs {
			args = append(args, id)
		}
	}
	if len(f.Providers) > 0 {
		where = append(where, fmt.Sprintf("%s IN (%s)", ColProvider, placeholders(len(f.Providers))))
		for _, p := range f.Providers {
			args = append(args, p)
		}
	}
	if len(f.Models) > 0 {
		where = append(where, fmt.Sprintf("%s IN (%s)", ColModel, placeholders(len(f.Models))))
		for _, m := range f.Models {
			args = append(args, m)
		}
	}
	if len(f.Days) > 0 {
		where = append(where, fmt.Sprintf("date(%s/1000, 'unixepoch', 'localtime') IN (%s)", ColRecordedAtMs, placeholders(len(f.Days))))
		for _, d := range f.Days {
			args = append(args, d)
		}
	}
	return args, where
}

// Events queries oc_token_events with filters pushed into SQL.
func Events(ctx context.Context, db *sql.DB, f Filter) ([]Event, error) {
	args, where := buildFilterArgs(f)
	clause := ""
	if len(where) > 0 {
		clause = "WHERE " + strings.Join(where, " AND ")
	}
	query := fmt.Sprintf(`
		SELECT %s, %s, %s, %s, %s, %s, %s, %s, %s, %s
		FROM %s
		%s
	`,
		ColRecordedAtMs, ColSessionID, ColProvider, ColModel,
		ColInputTokens, ColOutputTokens, ColReasoningTokens,
		ColCacheReadTokens, ColCacheWriteTokens, ColTotalTokens,
		TableTokenEvents, clause,
	)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(
			&e.RecordedAtMs, &e.SessionID, &e.Provider, &e.Model,
			&e.InputTokens, &e.OutputTokens, &e.ReasoningTokens,
			&e.CacheReadTokens, &e.CacheWriteTokens, &e.TotalTokens,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}
