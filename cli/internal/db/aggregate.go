package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// GroupBy controls the aggregation grouping.
type GroupBy int

const (
	GroupByDay GroupBy = iota
	GroupByDayHour
	GroupByDaySession
)

// Row is an aggregated result row with raw numeric fields.
type Row struct {
	Day              string
	Hour             string
	SessionID        string
	Provider         string
	Model            string
	InputTokens      int64
	OutputTokens     int64
	ReasoningTokens  int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	TotalTokens      int64
	ThroughputTokens int64
	DurationMs       int64
	TpsMean          float64
	TpsMedian        float64
	Requests         int64
	Retries          int64
	ThinkingLevels   string
}

type rowKey struct {
	day       string
	hour      string
	sessionID string
	provider  string
	model     string
}

const (
	exprDay  = "date(recorded_at_ms/1000, 'unixepoch', 'localtime')"
	exprHour = "strftime('%H:00', recorded_at_ms/1000, 'unixepoch', 'localtime')"
)

func groupAliases(g GroupBy) string {
	switch g {
	case GroupByDayHour:
		return fmt.Sprintf("%s as day, %s as hour, %s, %s", exprDay, exprHour, ColProvider, ColModel)
	case GroupByDaySession:
		return fmt.Sprintf("%s as day, %s, %s, %s", exprDay, ColSessionID, ColProvider, ColModel)
	default:
		return fmt.Sprintf("%s as day, %s, %s", exprDay, ColProvider, ColModel)
	}
}

func groupByAliases(g GroupBy) string {
	switch g {
	case GroupByDayHour:
		return "day, hour, provider, model"
	case GroupByDaySession:
		return "day, session_id, provider, model"
	default:
		return "day, provider, model"
	}
}

func partitionBy(g GroupBy) string {
	switch g {
	case GroupByDayHour:
		return fmt.Sprintf("%s, %s, %s, %s", exprDay, exprHour, ColProvider, ColModel)
	case GroupByDaySession:
		return fmt.Sprintf("%s, %s, %s, %s", exprDay, ColSessionID, ColProvider, ColModel)
	default:
		return fmt.Sprintf("%s, %s, %s", exprDay, ColProvider, ColModel)
	}
}

func scanKey(row *Row, g GroupBy) rowKey {
	switch g {
	case GroupByDayHour:
		return rowKey{day: row.Day, hour: row.Hour, provider: row.Provider, model: row.Model}
	case GroupByDaySession:
		return rowKey{day: row.Day, sessionID: row.SessionID, provider: row.Provider, model: row.Model}
	default:
		return rowKey{day: row.Day, provider: row.Provider, model: row.Model}
	}
}

func scanTokenEventRow(rows *sql.Rows, g GroupBy) (*Row, error) {
	var day, provider, model string
	var hour, sessionID string
	var input, output, reasoning, cacheRead, cacheWrite, total int64

	var scanArgs []any
	switch g {
	case GroupByDayHour:
		scanArgs = []any{&day, &hour, &provider, &model}
	case GroupByDaySession:
		scanArgs = []any{&day, &sessionID, &provider, &model}
	default:
		scanArgs = []any{&day, &provider, &model}
	}
	scanArgs = append(scanArgs, &input, &output, &reasoning, &cacheRead, &cacheWrite, &total)

	if err := rows.Scan(scanArgs...); err != nil {
		return nil, err
	}
	return &Row{
		Day: day, Hour: hour, SessionID: sessionID,
		Provider: provider, Model: model,
		InputTokens: input, OutputTokens: output,
		ReasoningTokens: reasoning, CacheReadTokens: cacheRead,
		CacheWriteTokens: cacheWrite, TotalTokens: total,
	}, nil
}

func scanTpsRow(rows *sql.Rows, g GroupBy) (*Row, error) {
	var day, provider, model string
	var hour, sessionID string
	var throughput, duration int64
	var mean, median float64

	var scanArgs []any
	switch g {
	case GroupByDayHour:
		scanArgs = []any{&day, &hour, &provider, &model}
	case GroupByDaySession:
		scanArgs = []any{&day, &sessionID, &provider, &model}
	default:
		scanArgs = []any{&day, &provider, &model}
	}
	scanArgs = append(scanArgs, &throughput, &duration, &mean, &median)

	if err := rows.Scan(scanArgs...); err != nil {
		return nil, err
	}
	return &Row{
		Day: day, Hour: hour, SessionID: sessionID,
		Provider: provider, Model: model,
		ThroughputTokens: throughput, DurationMs: duration,
		TpsMean: mean, TpsMedian: median,
	}, nil
}

func scanRequestRow(rows *sql.Rows, g GroupBy) (*Row, error) {
	var day, provider, model string
	var hour, sessionID string
	var requests, retries int64
	var thinking sql.NullString

	var scanArgs []any
	switch g {
	case GroupByDayHour:
		scanArgs = []any{&day, &hour, &provider, &model}
	case GroupByDaySession:
		scanArgs = []any{&day, &sessionID, &provider, &model}
	default:
		scanArgs = []any{&day, &provider, &model}
	}
	scanArgs = append(scanArgs, &requests, &retries, &thinking)

	if err := rows.Scan(scanArgs...); err != nil {
		return nil, err
	}
	return &Row{
		Day: day, Hour: hour, SessionID: sessionID,
		Provider: provider, Model: model,
		Requests: requests, Retries: retries,
		ThinkingLevels: thinking.String,
	}, nil
}

func mergeRow(result map[rowKey]*Row, r *Row, g GroupBy) {
	key := scanKey(r, g)
	existing, ok := result[key]
	if !ok {
		result[key] = r
		return
	}
	existing.InputTokens += r.InputTokens
	existing.OutputTokens += r.OutputTokens
	existing.ReasoningTokens += r.ReasoningTokens
	existing.CacheReadTokens += r.CacheReadTokens
	existing.CacheWriteTokens += r.CacheWriteTokens
	existing.TotalTokens += r.TotalTokens
	existing.ThroughputTokens += r.ThroughputTokens
	existing.DurationMs += r.DurationMs
	if r.TpsMean > 0 {
		existing.TpsMean = r.TpsMean
	}
	if r.TpsMedian > 0 {
		existing.TpsMedian = r.TpsMedian
	}
	existing.Requests += r.Requests
	existing.Retries += r.Retries
	if r.ThinkingLevels != "" {
		if existing.ThinkingLevels != "" {
			existing.ThinkingLevels = mergeThinkingLevels(existing.ThinkingLevels, r.ThinkingLevels)
		} else {
			existing.ThinkingLevels = r.ThinkingLevels
		}
	}
}

func mergeThinkingLevels(a, b string) string {
	set := make(map[string]bool)
	for _, v := range strings.Split(a, ",") {
		set[v] = true
	}
	for _, v := range strings.Split(b, ",") {
		set[v] = true
	}
	order := []string{"low", "medium", "high", "xhigh"}
	var result []string
	seen := make(map[string]bool)
	for _, v := range order {
		if set[v] {
			result = append(result, v)
			seen[v] = true
		}
	}
	var extra []string
	for v := range set {
		if !seen[v] && v != "" && v != "unknown" {
			extra = append(extra, v)
		}
	}
	sort.Strings(extra)
	result = append(result, extra...)
	return strings.Join(result, ",")
}

func buildWhereClause(f Filter) (string, []any) {
	args, where := buildFilterArgs(f)
	if len(where) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(where, " AND "), args
}

// aggregateTokenEvents queries oc_token_events with SQL-side SUM grouping.
func aggregateTokenEvents(ctx context.Context, db *sql.DB, f Filter, g GroupBy) (map[rowKey]*Row, error) {
	whereClause, args := buildWhereClause(f)

	query := fmt.Sprintf(`
		SELECT %s,
			SUM(%s) as input_tokens,
			SUM(%s) as output_tokens,
			SUM(%s) as reasoning_tokens,
			SUM(%s) as cache_read_tokens,
			SUM(%s) as cache_write_tokens,
			SUM(%s) as total_tokens
		FROM %s
		%s
		GROUP BY %s
	`,
		groupAliases(g),
		ColInputTokens, ColOutputTokens, ColReasoningTokens,
		ColCacheReadTokens, ColCacheWriteTokens, ColTotalTokens,
		TableTokenEvents, whereClause, groupByAliases(g),
	)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[rowKey]*Row)
	for rows.Next() {
		r, err := scanTokenEventRow(rows, g)
		if err != nil {
			return nil, err
		}
		mergeRow(result, r, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// aggregateTpsSamples queries oc_tps_samples with SQL-side GROUP BY and median via window CTE.
func aggregateTpsSamples(ctx context.Context, db *sql.DB, f Filter, g GroupBy, result map[rowKey]*Row) error {
	exists, err := tableExists(ctx, db, TableTpsSamples)
	if err != nil || !exists {
		return err
	}

	whereClause, args := buildWhereClause(f)

	query := fmt.Sprintf(`
		WITH ranked AS (
			SELECT
				%s,
				%s as total_tokens,
				%s as duration_ms,
				%s as tps,
				ROW_NUMBER() OVER (
					PARTITION BY %s
					ORDER BY %s
				) as rn,
				COUNT(*) OVER (
					PARTITION BY %s
				) as cnt
			FROM %s
			%s
		)
		SELECT %s,
			SUM(total_tokens) as throughput_tokens,
			SUM(duration_ms) as duration_ms,
			AVG(tps) as tps_mean,
			AVG(CASE WHEN rn IN ((cnt+1)/2, (cnt+2)/2) THEN tps END) as tps_median
		FROM ranked
		GROUP BY %s
	`,
		groupAliases(g), ColTpsTotalTokens, ColDurationMs, ColTokensPerSecond,
		partitionBy(g), ColTokensPerSecond, partitionBy(g),
		TableTpsSamples, whereClause,
		groupByAliases(g), groupByAliases(g),
	)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		r, err := scanTpsRow(rows, g)
		if err != nil {
			return err
		}
		mergeRow(result, r, g)
	}
	return rows.Err()
}

// aggregateLLMRequests queries oc_llm_requests with SQL-side COUNT grouping.
func aggregateLLMRequests(ctx context.Context, db *sql.DB, f Filter, g GroupBy, result map[rowKey]*Row) error {
	exists, err := tableExists(ctx, db, TableLLMRequests)
	if err != nil || !exists {
		return err
	}

	whereClause, args := buildWhereClause(f)

	thinkingSelect := fmt.Sprintf(
		", GROUP_CONCAT(DISTINCT CASE WHEN %s != 'unknown' THEN %s END) as thinking_levels",
		ColThinkingLevel, ColThinkingLevel,
	)

	query := fmt.Sprintf(`
		SELECT %s,
			SUM(CASE WHEN %s = 1 THEN 1 ELSE 0 END) as requests,
			SUM(CASE WHEN %s > 1 THEN 1 ELSE 0 END) as retries%s
		FROM %s
		%s
		GROUP BY %s
	`,
		groupAliases(g), ColAttemptIndex, ColAttemptIndex, thinkingSelect,
		TableLLMRequests, whereClause, groupByAliases(g),
	)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		r, err := scanRequestRow(rows, g)
		if err != nil {
			return err
		}
		mergeRow(result, r, g)
	}
	return rows.Err()
}

// tableExists checks whether a table exists in sqlite_master.
func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// Aggregate queries all three tables with SQL-side GROUP BY and merges by group key.
func Aggregate(ctx context.Context, db *sql.DB, f Filter, g GroupBy) ([]Row, error) {
	result, err := aggregateTokenEvents(ctx, db, f, g)
	if err != nil {
		return nil, err
	}

	if err := aggregateTpsSamples(ctx, db, f, g, result); err != nil {
		return nil, err
	}

	if err := aggregateLLMRequests(ctx, db, f, g, result); err != nil {
		return nil, err
	}

	rows := make([]Row, 0, len(result))
	for _, r := range result {
		rows = append(rows, *r)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Day != rows[j].Day {
			return rows[i].Day > rows[j].Day
		}
		if rows[i].Hour != rows[j].Hour {
			return rows[i].Hour > rows[j].Hour
		}
		if rows[i].SessionID != rows[j].SessionID {
			return rows[i].SessionID < rows[j].SessionID
		}
		if rows[i].Provider != rows[j].Provider {
			return rows[i].Provider < rows[j].Provider
		}
		return rows[i].Model < rows[j].Model
	})

	return rows, nil
}
