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
	Harness          string
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
	LatestAtMs       int64
}

type rowKey struct {
	harness   string
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

func groupAliases(g GroupBy, harness string) string {
	h := fmt.Sprintf("'%s' as harness", harness)
	switch g {
	case GroupByDayHour:
		return fmt.Sprintf("%s as day, %s as hour, %s, %s, %s", exprDay, exprHour, ColProvider, ColModel, h)
	case GroupByDaySession:
		return fmt.Sprintf("%s as day, %s, %s, %s, %s", exprDay, ColSessionID, ColProvider, ColModel, h)
	default:
		return fmt.Sprintf("%s as day, %s, %s, %s", exprDay, ColProvider, ColModel, h)
	}
}

func groupByAliases(g GroupBy) string {
	switch g {
	case GroupByDayHour:
		return "day, hour, provider, model, harness"
	case GroupByDaySession:
		return "day, session_id, provider, model, harness"
	default:
		return "day, provider, model, harness"
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
		return rowKey{harness: row.Harness, day: row.Day, hour: row.Hour, provider: row.Provider, model: row.Model}
	case GroupByDaySession:
		return rowKey{harness: row.Harness, day: row.Day, sessionID: row.SessionID, provider: row.Provider, model: row.Model}
	default:
		return rowKey{harness: row.Harness, day: row.Day, provider: row.Provider, model: row.Model}
	}
}

func scanTokenEventRow(rows *sql.Rows, g GroupBy) (*Row, error) {
	var day, provider, model string
	var hour, sessionID, harness string
	var input, output, reasoning, cacheRead, cacheWrite, total, latestAtMs int64

	var scanArgs []any
	switch g {
	case GroupByDayHour:
		scanArgs = []any{&day, &hour, &provider, &model, &harness}
	case GroupByDaySession:
		scanArgs = []any{&day, &sessionID, &provider, &model, &harness}
	default:
		scanArgs = []any{&day, &provider, &model, &harness}
	}
	scanArgs = append(scanArgs, &input, &output, &reasoning, &cacheRead, &cacheWrite, &total, &latestAtMs)

	if err := rows.Scan(scanArgs...); err != nil {
		return nil, err
	}
	return &Row{
		Harness: harness, Day: day, Hour: hour, SessionID: sessionID,
		Provider: provider, Model: model,
		InputTokens: input, OutputTokens: output,
		ReasoningTokens: reasoning, CacheReadTokens: cacheRead,
		CacheWriteTokens: cacheWrite, TotalTokens: total,
		LatestAtMs: latestAtMs,
	}, nil
}

func scanTpsRow(rows *sql.Rows, g GroupBy) (*Row, error) {
	var day, provider, model string
	var hour, sessionID, harness string
	var throughput, duration, latestAtMs int64
	var mean, median float64

	var scanArgs []any
	switch g {
	case GroupByDayHour:
		scanArgs = []any{&day, &hour, &provider, &model, &harness}
	case GroupByDaySession:
		scanArgs = []any{&day, &sessionID, &provider, &model, &harness}
	default:
		scanArgs = []any{&day, &provider, &model, &harness}
	}
	scanArgs = append(scanArgs, &throughput, &duration, &mean, &median, &latestAtMs)

	if err := rows.Scan(scanArgs...); err != nil {
		return nil, err
	}
	return &Row{
		Harness: harness, Day: day, Hour: hour, SessionID: sessionID,
		Provider: provider, Model: model,
		ThroughputTokens: throughput, DurationMs: duration,
		TpsMean: mean, TpsMedian: median,
		LatestAtMs: latestAtMs,
	}, nil
}

func scanRequestRow(rows *sql.Rows, g GroupBy) (*Row, error) {
	var day, provider, model string
	var hour, sessionID, harness string
	var requests, retries, latestAtMs int64
	var thinking sql.NullString

	var scanArgs []any
	switch g {
	case GroupByDayHour:
		scanArgs = []any{&day, &hour, &provider, &model, &harness}
	case GroupByDaySession:
		scanArgs = []any{&day, &sessionID, &provider, &model, &harness}
	default:
		scanArgs = []any{&day, &provider, &model, &harness}
	}
	scanArgs = append(scanArgs, &requests, &retries, &thinking, &latestAtMs)

	if err := rows.Scan(scanArgs...); err != nil {
		return nil, err
	}
	return &Row{
		Harness: harness, Day: day, Hour: hour, SessionID: sessionID,
		Provider: provider, Model: model,
		Requests: requests, Retries: retries,
		ThinkingLevels: thinking.String,
		LatestAtMs: latestAtMs,
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
	if r.LatestAtMs > existing.LatestAtMs {
		existing.LatestAtMs = r.LatestAtMs
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

// queryTokenEvents aggregates token events from a single table.
func queryTokenEvents(ctx context.Context, db *sql.DB, f Filter, g GroupBy, table, harness string, result map[rowKey]*Row) error {
	whereClause, args := buildWhereClause(f)

	query := fmt.Sprintf(`
		SELECT %s,
			SUM(%s) as input_tokens,
			SUM(%s) as output_tokens,
			SUM(%s) as reasoning_tokens,
			SUM(%s) as cache_read_tokens,
			SUM(%s) as cache_write_tokens,
			SUM(%s) as total_tokens,
			MAX(%s) as latest_at_ms
		FROM %s
		%s
		GROUP BY %s
	`,
		groupAliases(g, harness),
		ColInputTokens, ColOutputTokens, ColReasoningTokens,
		ColCacheReadTokens, ColCacheWriteTokens, ColTotalTokens,
		ColRecordedAtMs,
		table, whereClause, groupByAliases(g),
	)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		r, err := scanTokenEventRow(rows, g)
		if err != nil {
			return err
		}
		mergeRow(result, r, g)
	}
	return rows.Err()
}

// aggregateTokenEvents queries oc_token_events and pi_token_events with SQL-side SUM grouping.
func aggregateTokenEvents(ctx context.Context, db *sql.DB, f Filter, g GroupBy) (map[rowKey]*Row, error) {
	result := make(map[rowKey]*Row)

	if err := queryTokenEvents(ctx, db, f, g, TableTokenEvents, "oc", result); err != nil {
		return nil, err
	}

	exists, err := tableExists(ctx, db, TablePiTokenEvents)
	if err != nil {
		return nil, err
	}
	if exists {
		if err := queryTokenEvents(ctx, db, f, g, TablePiTokenEvents, "pi", result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// queryTpsSamples aggregates TPS samples from a single table.
func queryTpsSamples(ctx context.Context, db *sql.DB, f Filter, g GroupBy, table, harness string, result map[rowKey]*Row) error {
	whereClause, args := buildWhereClause(f)

	query := fmt.Sprintf(`
		WITH ranked AS (
			SELECT
				%s,
				%s as total_tokens,
				%s as duration_ms,
				%s as tps,
				%s as recorded_at_ms,
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
			AVG(CASE WHEN rn IN ((cnt+1)/2, (cnt+2)/2) THEN tps END) as tps_median,
			MAX(recorded_at_ms) as latest_at_ms
		FROM ranked
		GROUP BY %s
	`,
		groupAliases(g, harness), ColTpsTotalTokens, ColDurationMs, ColTokensPerSecond,
		ColRecordedAtMs,
		partitionBy(g), ColTokensPerSecond, partitionBy(g),
		table, whereClause,
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

// aggregateTpsSamples queries oc_tps_samples and pi_tps_samples with SQL-side GROUP BY and median via window CTE.
func aggregateTpsSamples(ctx context.Context, db *sql.DB, f Filter, g GroupBy, result map[rowKey]*Row) error {
	exists, err := tableExists(ctx, db, TableTpsSamples)
	if err != nil || !exists {
		return err
	}
	if err := queryTpsSamples(ctx, db, f, g, TableTpsSamples, "oc", result); err != nil {
		return err
	}

	exists, err = tableExists(ctx, db, TablePiTpsSamples)
	if err != nil {
		return err
	}
	if exists {
		if err := queryTpsSamples(ctx, db, f, g, TablePiTpsSamples, "pi", result); err != nil {
			return err
		}
	}

	return nil
}

// queryLLMRequests aggregates LLM requests from a single table.
func queryLLMRequests(ctx context.Context, db *sql.DB, f Filter, g GroupBy, table, harness string, result map[rowKey]*Row) error {
	whereClause, args := buildWhereClause(f)

	thinkingSelect := fmt.Sprintf(
		", GROUP_CONCAT(DISTINCT CASE WHEN %s != 'unknown' THEN %s END) as thinking_levels",
		ColThinkingLevel, ColThinkingLevel,
	)

	query := fmt.Sprintf(`
		SELECT %s,
			SUM(CASE WHEN %s = 1 THEN 1 ELSE 0 END) as requests,
			SUM(CASE WHEN %s > 1 THEN 1 ELSE 0 END) as retries%s,
			MAX(%s) as latest_at_ms
		FROM %s
		%s
		GROUP BY %s
	`,
		groupAliases(g, harness), ColAttemptIndex, ColAttemptIndex, thinkingSelect,
		ColRecordedAtMs,
		table, whereClause, groupByAliases(g),
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

// aggregateLLMRequests queries oc_llm_requests and pi_llm_requests with SQL-side COUNT grouping.
func aggregateLLMRequests(ctx context.Context, db *sql.DB, f Filter, g GroupBy, result map[rowKey]*Row) error {
	exists, err := tableExists(ctx, db, TableLLMRequests)
	if err != nil || !exists {
		return err
	}
	if err := queryLLMRequests(ctx, db, f, g, TableLLMRequests, "oc", result); err != nil {
		return err
	}

	exists, err = tableExists(ctx, db, TablePiLLMRequests)
	if err != nil {
		return err
	}
	if exists {
		if err := queryLLMRequests(ctx, db, f, g, TablePiLLMRequests, "pi", result); err != nil {
			return err
		}
	}

	return nil
}

// tableExists checks whether a table exists in sqlite_master.
func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// Aggregate queries all three table families with SQL-side GROUP BY and merges by group key.
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
		if rows[i].Harness != rows[j].Harness {
			return rows[i].Harness < rows[j].Harness
		}
		if rows[i].Day != rows[j].Day {
			return rows[i].Day > rows[j].Day
		}
		if rows[i].Hour != rows[j].Hour {
			return rows[i].Hour > rows[j].Hour
		}
		if g == GroupByDaySession {
			if rows[i].LatestAtMs != rows[j].LatestAtMs {
				return rows[i].LatestAtMs > rows[j].LatestAtMs
			}
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
