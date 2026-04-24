package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	_ "modernc.org/sqlite"
)

type period string

const (
	periodDay   period = "day"
	periodWeek  period = "week"
	periodMonth period = "month"
)

type groupByMode string

const (
	groupByNone    groupByMode = ""
	groupByHour    groupByMode = "hour"
	groupBySession groupByMode = "session"
)

var errUsage = errors.New("usage: tokeninspector-cli --db-path PATH [--day|--week|--month] [--group-by=hour|session] [--session-id ID] [--provider ID] [--model ID] [--filter-day YYYY-MM-DD]\n       tokeninspector-cli table --db-path PATH [--day|--week|--month] [--group-by=hour|session] [--session-id ID] [--provider ID] [--model ID] [--filter-day YYYY-MM-DD]")

type groupByFlag struct {
	value groupByMode
	set   bool
}

func (value *groupByFlag) String() string {
	return string(value.value)
}

func (value *groupByFlag) Set(input string) error {
	if value.set {
		return errors.New("only one --group-by can be passed")
	}

	switch input {
	case string(groupByHour):
		value.value = groupByHour
	case string(groupBySession):
		value.value = groupBySession
	default:
		return fmt.Errorf("invalid --group-by %q: expected hour or session", input)
	}
	value.set = true
	return nil
}

type stringList []string

func (values *stringList) String() string {
	return strings.Join(*values, ",")
}

func (values *stringList) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			*values = append(*values, trimmed)
		}
	}
	return nil
}

type filters struct {
	sessionIDs stringList
	providers  stringList
	models     stringList
	days       stringList
}

type sample struct {
	recordedAtMs     int64
	sessionID        string
	messageID        string
	provider         string
	model            string
	inputTokens      int64
	outputTokens     int64
	reasoningTokens  int64
	cacheReadTokens  int64
	cacheWriteTokens int64
	totalTokens      int64
	throughputTokens int64
	durationMs       int64
	tokensPerSecond  float64
	requests         int64
	retries          int64
	thinkingLevel    string
}

type tableRow struct {
	day              string
	hour             string
	sessionID        string
	provider         string
	model            string
	tpsAvg           string
	tpsMean          string
	tpsMedian        string
	inputTokens      string
	outputTokens     string
	reasoningTokens  string
	cacheReadTokens  string
	cacheWriteTokens string
	totalTokens      string
	requests         string
	retries          string
	thinkingLevels   string
}

type aggregate struct {
	inputTokens      int64
	outputTokens     int64
	reasoningTokens  int64
	cacheReadTokens  int64
	cacheWriteTokens int64
	totalTokens      int64
	throughputTokens int64
	durationMs       int64
	sumTPS           float64
	tpsValues        []float64
	requests         int64
	retries          int64
	thinkingLevels   map[string]bool
}

type tableOptions struct {
	dbPath  string
	period  period
	groupBy groupByMode
	filters filters
}

type interactiveModel struct {
	rows    []tableRow
	groupBy groupByMode
	period  period
	width   int
	height  int
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, errUsage) {
			fmt.Fprintln(os.Stderr, errUsage)
			os.Exit(2)
		}

		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) == 0 {
		return errUsage
	}

	switch args[0] {
	case "table":
		return runInteractive(ctx, args[1:], stdout, stderr, time.Now())
	case "help", "--help", "-h":
		fmt.Fprintln(stdout, errUsage)
		return nil
	default:
		return runInteractive(ctx, args, stdout, stderr, time.Now())
	}
}

func runTable(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
	options, err := parseTableOptions(args, stderr, true, "")
	if err != nil {
		return err
	}

	rows, err := loadRows(ctx, options, now)
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, renderTable(rows, options.groupBy))
	return nil
}

func runInteractive(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
	options, err := parseTableOptions(args, stderr, false, periodWeek)
	if err != nil {
		return err
	}

	rows, err := loadRows(ctx, options, now)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(interactiveModel{
		rows:    rows,
		groupBy: options.groupBy,
		period:  options.period,
	}, tea.WithInput(os.Stdin), tea.WithOutput(stdout)).Run()
	return err
}

func parseTableOptions(args []string, stderr io.Writer, requirePeriod bool, defaultPeriod period) (tableOptions, error) {
	flags := flag.NewFlagSet("table", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var dbPath string
	var day bool
	var week bool
	var month bool
	var groupBy groupByFlag
	var queryFilters filters
	flags.StringVar(&dbPath, "db-path", "", "path to tokeninspector sqlite db")
	flags.BoolVar(&day, "day", false, "show today")
	flags.BoolVar(&week, "week", false, "show current 7-day window")
	flags.BoolVar(&month, "month", false, "show current calendar month")
	flags.Var(&groupBy, "group-by", "group output by hour or session")
	flags.Var(&queryFilters.sessionIDs, "session-id", "filter by session id; repeat or comma-separate")
	flags.Var(&queryFilters.providers, "provider", "filter by provider; repeat or comma-separate")
	flags.Var(&queryFilters.models, "model", "filter by model; repeat or comma-separate")
	flags.Var(&queryFilters.days, "filter-day", "filter by local day YYYY-MM-DD; repeat or comma-separate")

	if err := flags.Parse(args); err != nil {
		return tableOptions{}, fmt.Errorf("%v\n%w", err, errUsage)
	}
	if flags.NArg() > 0 {
		return tableOptions{}, fmt.Errorf("unexpected argument %q\n%w", flags.Arg(0), errUsage)
	}
	if strings.TrimSpace(dbPath) == "" {
		return tableOptions{}, fmt.Errorf("missing --db-path\n%w", errUsage)
	}

	selected, err := selectedPeriod(day, week, month, requirePeriod, defaultPeriod)
	if err != nil {
		return tableOptions{}, err
	}

	return tableOptions{dbPath: dbPath, period: selected, groupBy: groupBy.value, filters: queryFilters}, nil
}

func loadRows(ctx context.Context, options tableOptions, now time.Time) ([]tableRow, error) {
	start := periodStart(now, options.period)
	samples, err := querySamples(ctx, options.dbPath, start)
	if err != nil {
		return nil, err
	}
	samples, err = filterSamples(samples, options.filters)
	if err != nil {
		return nil, err
	}

	return aggregateSamples(samples, options.groupBy), nil
}

func selectedPeriod(day bool, week bool, month bool, required bool, fallback period) (period, error) {
	selected := 0
	if day {
		selected++
	}
	if week {
		selected++
	}
	if month {
		selected++
	}
	if selected == 0 && !required {
		return fallback, nil
	}
	if selected != 1 {
		return "", fmt.Errorf("choose exactly one of --day, --week, --month\n%w", errUsage)
	}

	switch {
	case day:
		return periodDay, nil
	case week:
		return periodWeek, nil
	default:
		return periodMonth, nil
	}
}

func periodStart(now time.Time, selected period) time.Time {
	local := now.Local()
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())

	switch selected {
	case periodDay:
		return today
	case periodWeek:
		return today.AddDate(0, 0, -6)
	case periodMonth:
		return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	default:
		return today
	}
}

func querySamples(ctx context.Context, dbPath string, start time.Time) ([]sample, error) {
	db, err := openDB(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	samples, err := queryTokenSamples(ctx, db, start)
	if err != nil {
		return nil, err
	}
	exists, err := tableExists(ctx, db, "oc_tps_samples")
	if err != nil {
		return nil, err
	}
	if exists {
		tpsSamples, err := queryTpsSamples(ctx, db, start)
		if err != nil {
			return nil, err
		}
		samples = append(samples, tpsSamples...)
	}

	exists, err = tableExists(ctx, db, "oc_llm_requests")
	if err != nil {
		return nil, err
	}
	if exists {
		requestSamples, err := queryRequestSamples(ctx, db, start)
		if err != nil {
			return nil, err
		}
		samples = append(samples, requestSamples...)
	}

	return samples, nil
}

func queryTokenSamples(ctx context.Context, db *sql.DB, start time.Time) ([]sample, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT recorded_at_ms, session_id, provider, model, input_tokens, output_tokens, reasoning_tokens, cache_read_tokens, cache_write_tokens, total_tokens
		FROM oc_token_events
		WHERE recorded_at_ms >= ?
	`, start.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []sample
	for rows.Next() {
		var next sample
		if err := rows.Scan(
			&next.recordedAtMs,
			&next.sessionID,
			&next.provider,
			&next.model,
			&next.inputTokens,
			&next.outputTokens,
			&next.reasoningTokens,
			&next.cacheReadTokens,
			&next.cacheWriteTokens,
			&next.totalTokens,
		); err != nil {
			return nil, err
		}
		samples = append(samples, next)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return samples, nil
}

func queryTpsSamples(ctx context.Context, db *sql.DB, start time.Time) ([]sample, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT recorded_at_ms, session_id, provider, model, total_tokens, duration_ms, tokens_per_second
		FROM oc_tps_samples
		WHERE recorded_at_ms >= ?
	`, start.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []sample
	for rows.Next() {
		var next sample
		if err := rows.Scan(
			&next.recordedAtMs,
			&next.sessionID,
			&next.provider,
			&next.model,
			&next.throughputTokens,
			&next.durationMs,
			&next.tokensPerSecond,
		); err != nil {
			return nil, err
		}
		samples = append(samples, next)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return samples, nil
}

func queryRequestSamples(ctx context.Context, db *sql.DB, start time.Time) ([]sample, error) {
	thinkingSelect := "'unknown'"
	exists, err := columnExists(ctx, db, "oc_llm_requests", "thinking_level")
	if err != nil {
		return nil, err
	}
	if exists {
		thinkingSelect = "thinking_level"
	}

	rows, err := db.QueryContext(ctx, `
		SELECT recorded_at_ms, session_id, message_id, provider, model, attempt_index, `+thinkingSelect+`
		FROM oc_llm_requests
		WHERE recorded_at_ms >= ?
		ORDER BY recorded_at_ms ASC, attempt_index ASC
	`, start.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []sample
	for rows.Next() {
		var next sample
		var attemptIndex int64
		if err := rows.Scan(
			&next.recordedAtMs,
			&next.sessionID,
			&next.messageID,
			&next.provider,
			&next.model,
			&attemptIndex,
			&next.thinkingLevel,
		); err != nil {
			return nil, err
		}
		if attemptIndex > 1 {
			next.retries = 1
		} else {
			next.requests = 1
		}
		samples = append(samples, next)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return samples, nil
}

func columnExists(ctx context.Context, db *sql.DB, tableName string, columnName string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM pragma_table_info(?)
		WHERE name = ?
	`, tableName, columnName).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type = 'table' AND name = ?
	`, name).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func filterSamples(samples []sample, queryFilters filters) ([]sample, error) {
	days, err := daySet(queryFilters.days)
	if err != nil {
		return nil, err
	}
	sessionIDs := valueSet(queryFilters.sessionIDs)
	providers := valueSet(queryFilters.providers)
	models := valueSet(queryFilters.models)

	filtered := make([]sample, 0, len(samples))
	for _, item := range samples {
		if len(sessionIDs) > 0 && !sessionIDs[item.sessionID] {
			continue
		}
		if len(providers) > 0 && !providers[item.provider] {
			continue
		}
		if len(models) > 0 && !models[item.model] {
			continue
		}
		if len(days) > 0 && !days[time.UnixMilli(item.recordedAtMs).Local().Format(time.DateOnly)] {
			continue
		}

		filtered = append(filtered, item)
	}

	return filtered, nil
}

func valueSet(values stringList) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func daySet(values stringList) (map[string]bool, error) {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		parsed, err := time.ParseInLocation(time.DateOnly, value, time.Local)
		if err != nil {
			return nil, fmt.Errorf("invalid --filter-day %q: expected YYYY-MM-DD", value)
		}
		result[parsed.Format(time.DateOnly)] = true
	}
	return result, nil
}

func openDB(dbPath string) (*sql.DB, error) {
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
	fileURL.RawQuery = query.Encode()

	db, err := sql.Open("sqlite", fileURL.String())
	if err != nil {
		return nil, err
	}
	return db, nil
}

func aggregateSamples(samples []sample, groupBy groupByMode) []tableRow {
	samples = resolveMessageThinking(samples)
	switch groupBy {
	case groupByHour:
		return aggregateHourlySamples(samples)
	case groupBySession:
		return aggregateSessionSamples(samples)
	default:
		return aggregateDailySamples(samples)
	}
}

func resolveMessageThinking(samples []sample) []sample {
	type messageKey struct {
		sessionID string
		messageID string
		provider  string
		model     string
	}

	thinkingByMessage := map[messageKey]string{}
	for _, item := range samples {
		if item.messageID == "" || item.thinkingLevel == "" || item.thinkingLevel == "unknown" {
			continue
		}
		thinkingByMessage[messageKey{
			sessionID: item.sessionID,
			messageID: item.messageID,
			provider:  item.provider,
			model:     item.model,
		}] = item.thinkingLevel
	}

	resolved := make([]sample, 0, len(samples))
	for _, item := range samples {
		if item.thinkingLevel != "" {
			item.thinkingLevel = ""
			if item.messageID != "" && item.requests > 0 {
				item.thinkingLevel = thinkingByMessage[messageKey{
					sessionID: item.sessionID,
					messageID: item.messageID,
					provider:  item.provider,
					model:     item.model,
				}]
			}
		}
		resolved = append(resolved, item)
	}
	return resolved
}

func aggregateDailySamples(samples []sample) []tableRow {
	type groupKey struct {
		day      string
		provider string
		model    string
	}

	groups := map[groupKey]*aggregate{}
	for _, item := range samples {
		key := groupKey{
			day:      time.UnixMilli(item.recordedAtMs).Local().Format(time.DateOnly),
			provider: item.provider,
			model:    item.model,
		}
		current := groups[key]
		if current == nil {
			current = &aggregate{}
			groups[key] = current
		}

		addSample(current, item)
	}

	result := make([]tableRow, 0, len(groups))
	for key, current := range groups {
		result = append(result, tableRow{
			day:              key.day,
			provider:         key.provider,
			model:            key.model,
			tpsAvg:           formatWeightedTPS(current),
			tpsMean:          formatMeanTPS(current),
			tpsMedian:        formatMedianTPS(current),
			inputTokens:      formatTokens(current.inputTokens),
			outputTokens:     formatTokens(current.outputTokens),
			reasoningTokens:  formatTokens(current.reasoningTokens),
			cacheReadTokens:  formatTokens(current.cacheReadTokens),
			cacheWriteTokens: formatTokens(current.cacheWriteTokens),
			totalTokens:      formatTokens(current.totalTokens),
			requests:         formatTokens(current.requests),
			retries:          formatTokens(current.retries),
			thinkingLevels:   formatThinkingLevels(current),
		})
	}

	sort.Slice(result, func(i int, j int) bool {
		if result[i].day != result[j].day {
			return result[i].day > result[j].day
		}
		if result[i].provider != result[j].provider {
			return result[i].provider < result[j].provider
		}
		return result[i].model < result[j].model
	})

	return result
}

func aggregateHourlySamples(samples []sample) []tableRow {
	type groupKey struct {
		day      string
		hour     string
		provider string
		model    string
	}

	groups := map[groupKey]*aggregate{}
	for _, item := range samples {
		recordedAt := time.UnixMilli(item.recordedAtMs).Local()
		key := groupKey{
			day:      recordedAt.Format(time.DateOnly),
			hour:     recordedAt.Format("15:00"),
			provider: item.provider,
			model:    item.model,
		}
		current := groups[key]
		if current == nil {
			current = &aggregate{}
			groups[key] = current
		}

		addSample(current, item)
	}

	result := make([]tableRow, 0, len(groups))
	for key, current := range groups {
		result = append(result, tableRow{
			day:              key.day,
			hour:             key.hour,
			provider:         key.provider,
			model:            key.model,
			tpsAvg:           formatWeightedTPS(current),
			tpsMean:          formatMeanTPS(current),
			tpsMedian:        formatMedianTPS(current),
			inputTokens:      formatTokens(current.inputTokens),
			outputTokens:     formatTokens(current.outputTokens),
			reasoningTokens:  formatTokens(current.reasoningTokens),
			cacheReadTokens:  formatTokens(current.cacheReadTokens),
			cacheWriteTokens: formatTokens(current.cacheWriteTokens),
			totalTokens:      formatTokens(current.totalTokens),
			requests:         formatTokens(current.requests),
			retries:          formatTokens(current.retries),
			thinkingLevels:   formatThinkingLevels(current),
		})
	}

	sort.Slice(result, func(i int, j int) bool {
		if result[i].day != result[j].day {
			return result[i].day > result[j].day
		}
		if result[i].hour != result[j].hour {
			return result[i].hour > result[j].hour
		}
		if result[i].provider != result[j].provider {
			return result[i].provider < result[j].provider
		}
		return result[i].model < result[j].model
	})

	return result
}

func aggregateSessionSamples(samples []sample) []tableRow {
	type groupKey struct {
		day       string
		sessionID string
		provider  string
		model     string
	}

	groups := map[groupKey]*aggregate{}
	for _, item := range samples {
		key := groupKey{
			day:       time.UnixMilli(item.recordedAtMs).Local().Format(time.DateOnly),
			sessionID: item.sessionID,
			provider:  item.provider,
			model:     item.model,
		}
		current := groups[key]
		if current == nil {
			current = &aggregate{}
			groups[key] = current
		}

		addSample(current, item)
	}

	result := make([]tableRow, 0, len(groups))
	for key, current := range groups {
		result = append(result, tableRow{
			day:              key.day,
			sessionID:        key.sessionID,
			provider:         key.provider,
			model:            key.model,
			tpsAvg:           formatWeightedTPS(current),
			tpsMean:          formatMeanTPS(current),
			tpsMedian:        formatMedianTPS(current),
			inputTokens:      formatTokens(current.inputTokens),
			outputTokens:     formatTokens(current.outputTokens),
			reasoningTokens:  formatTokens(current.reasoningTokens),
			cacheReadTokens:  formatTokens(current.cacheReadTokens),
			cacheWriteTokens: formatTokens(current.cacheWriteTokens),
			totalTokens:      formatTokens(current.totalTokens),
			requests:         formatTokens(current.requests),
			retries:          formatTokens(current.retries),
			thinkingLevels:   formatThinkingLevels(current),
		})
	}

	sort.Slice(result, func(i int, j int) bool {
		if result[i].day != result[j].day {
			return result[i].day > result[j].day
		}
		if result[i].sessionID != result[j].sessionID {
			return result[i].sessionID < result[j].sessionID
		}
		if result[i].provider != result[j].provider {
			return result[i].provider < result[j].provider
		}
		return result[i].model < result[j].model
	})

	return result
}

func addSample(current *aggregate, item sample) {
	current.inputTokens += item.inputTokens
	current.outputTokens += item.outputTokens
	current.reasoningTokens += item.reasoningTokens
	current.cacheReadTokens += item.cacheReadTokens
	current.cacheWriteTokens += item.cacheWriteTokens
	current.totalTokens += item.totalTokens
	current.requests += item.requests
	current.retries += item.retries
	if item.thinkingLevel != "" && item.thinkingLevel != "unknown" {
		if current.thinkingLevels == nil {
			current.thinkingLevels = map[string]bool{}
		}
		current.thinkingLevels[item.thinkingLevel] = true
	}
	if item.durationMs > 0 {
		current.throughputTokens += item.throughputTokens
		current.durationMs += item.durationMs
		current.sumTPS += item.tokensPerSecond
		current.tpsValues = append(current.tpsValues, item.tokensPerSecond)
	}
}

func formatThinkingLevels(current *aggregate) string {
	if len(current.thinkingLevels) == 0 {
		return ""
	}
	order := []string{"low", "medium", "high", "xhigh"}
	values := make([]string, 0, len(current.thinkingLevels))
	orderedValues := map[string]bool{}
	for _, value := range order {
		if current.thinkingLevels[value] {
			values = append(values, value)
		}
		orderedValues[value] = true
	}
	extra := make([]string, 0)
	for value := range current.thinkingLevels {
		if !orderedValues[value] {
			extra = append(extra, value)
		}
	}
	sort.Strings(extra)
	values = append(values, extra...)
	return strings.Join(values, ",")
}

func (model interactiveModel) Init() tea.Cmd {
	return nil
}

func (model interactiveModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch next := message.(type) {
	case tea.KeyMsg:
		switch next.String() {
		case "q", "ctrl+c":
			return model, tea.Quit
		}
	case tea.WindowSizeMsg:
		model.width = next.Width
		model.height = next.Height
	}
	return model, nil
}

func (model interactiveModel) View() string {
	title := titleStyle.Render(fmt.Sprintf("Token usage · %s", model.period))
	hint := hintStyle.Render("q quit")
	body := renderTableWithWidth(model.rows, model.groupBy, model.width)
	return lipgloss.JoinVertical(lipgloss.Left, title, hint, "", body) + "\n"
}

func renderTable(rows []tableRow, groupBy groupByMode) string {
	return renderTableWithWidth(rows, groupBy, 0)
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	hintStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("63")).Padding(0, 1)
	cellStyle   = lipgloss.NewStyle().Padding(0, 1)
	oddStyle    = cellStyle.Foreground(lipgloss.Color("252"))
	evenStyle   = cellStyle.Foreground(lipgloss.Color("245"))
	numberStyle = cellStyle.Align(lipgloss.Right)
	borderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func renderTableWithWidth(rows []tableRow, groupBy groupByMode, width int) string {
	header := []string{"day", "provider", "model", "tps avg", "tps mean", "tps median", "input", "output", "reasoning", "cache read", "cache write", "total", "requests", "retries"}
	switch groupBy {
	case groupByHour:
		header = []string{"day", "hour", "provider", "model", "tps avg", "tps mean", "tps median", "input", "output", "reasoning", "cache read", "cache write", "total", "requests", "retries"}
	case groupBySession:
		header = []string{"day", "session id", "thinking", "provider", "model", "tps avg", "tps mean", "tps median", "input", "output", "reasoning", "cache read", "cache write", "total", "requests", "retries"}
	}
	formatted := make([][]string, 0, len(rows))

	for _, row := range rows {
		values := []string{
			row.day,
			row.provider,
			displayModel(row.model),
			row.tpsAvg,
			row.tpsMean,
			row.tpsMedian,
			row.inputTokens,
			row.outputTokens,
			row.reasoningTokens,
			row.cacheReadTokens,
			row.cacheWriteTokens,
			row.totalTokens,
			row.requests,
			row.retries,
		}
		switch groupBy {
		case groupByHour:
			values = []string{
				row.day,
				row.hour,
				row.provider,
				displayModel(row.model),
				row.tpsAvg,
				row.tpsMean,
				row.tpsMedian,
				row.inputTokens,
				row.outputTokens,
				row.reasoningTokens,
				row.cacheReadTokens,
				row.cacheWriteTokens,
				row.totalTokens,
				row.requests,
				row.retries,
			}
		case groupBySession:
			values = []string{
				row.day,
				displaySessionID(row.sessionID),
				row.thinkingLevels,
				row.provider,
				displayModel(row.model),
				row.tpsAvg,
				row.tpsMean,
				row.tpsMedian,
				row.inputTokens,
				row.outputTokens,
				row.reasoningTokens,
				row.cacheReadTokens,
				row.cacheWriteTokens,
				row.totalTokens,
				row.requests,
				row.retries,
			}
		}
		formatted = append(formatted, values)
	}

	numberStart := 3
	if groupBy == groupByHour {
		numberStart = 4
	} else if groupBy == groupBySession {
		numberStart = 5
	}

	uiTable := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(borderStyle).
		Headers(header...).
		Rows(formatted...).
		StyleFunc(func(row int, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			base := oddStyle
			if row%2 == 0 {
				base = evenStyle
			}
			if col >= numberStart {
				return numberStyle.Inherit(base)
			}
			return base
		})
	if width > 0 {
		uiTable = uiTable.Width(width)
	}

	return uiTable.String() + "\n"
}

func displaySessionID(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[len(value)-8:]
}

func displayModel(value string) string {
	if index := strings.LastIndex(value, "/"); index >= 0 && index < len(value)-1 {
		return value[index+1:]
	}
	return value
}

func formatWeightedTPS(current *aggregate) string {
	if current.durationMs <= 0 || current.throughputTokens <= 0 {
		return ""
	}
	return formatTPS(float64(current.throughputTokens) / (float64(current.durationMs) / 1000))
}

func formatMeanTPS(current *aggregate) string {
	if len(current.tpsValues) == 0 {
		return ""
	}
	return formatTPS(current.sumTPS / float64(len(current.tpsValues)))
}

func formatMedianTPS(current *aggregate) string {
	if len(current.tpsValues) == 0 {
		return ""
	}
	return formatTPS(median(current.tpsValues))
}

func median(values []float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

func formatTPS(value float64) string {
	return fmt.Sprintf("%.2f", value)
}

func formatTokens(value int64) string {
	if value == 0 {
		return ""
	}
	abs := value
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs < 1000:
		return strconv.FormatInt(value, 10)
	case abs < 1_000_000:
		return fmt.Sprintf("%dK", value/1000)
	default:
		return fmt.Sprintf("%dM", value/1_000_000)
	}
}
