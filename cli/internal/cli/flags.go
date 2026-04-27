package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

type period string

const (
	periodToday   period = "today"
	periodWeek    period = "week"
	periodMonth   period = "month"
	periodAllTime period = "all"
)

type groupByMode string

const (
	groupByNone    groupByMode = ""
	groupByHour    groupByMode = "hour"
	groupBySession groupByMode = "session"
)

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
	dayFrom    string
	dayTo      string
}

type tableOptions struct {
	dbPath  string
	period  period
	filters filters
}

func parseTableOptions(args []string, stderr io.Writer, requirePeriod bool, defaultPeriod period) (tableOptions, error) {
	flags := flag.NewFlagSet("tokeninspector-cli", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var dbPath string
	var today bool
	var week bool
	var month bool
	var allTime bool
	var queryFilters filters
	flags.StringVar(&dbPath, "db-path", "", "path to tokeninspector sqlite db")
	flags.BoolVar(&today, "today", false, "show today")
	flags.BoolVar(&week, "week", false, "show current calendar week (Mon-Sun)")
	flags.BoolVar(&month, "month", false, "show current calendar month")
	flags.BoolVar(&allTime, "all-time", false, "show all time")
	flags.Var(&queryFilters.sessionIDs, "session-id", "filter by session id; repeat or comma-separate")
	flags.Var(&queryFilters.providers, "provider", "filter by provider; repeat or comma-separate")
	flags.Var(&queryFilters.models, "model", "filter by model; repeat or comma-separate")
	flags.StringVar(&queryFilters.dayFrom, "filter-day-from", "", "filter from local day YYYY-MM-DD")
	flags.StringVar(&queryFilters.dayTo, "filter-day-to", "", "filter to local day YYYY-MM-DD")

	if err := flags.Parse(args); err != nil {
		return tableOptions{}, fmt.Errorf("%v\n%w", err, ErrUsage)
	}
	if flags.NArg() > 0 {
		return tableOptions{}, fmt.Errorf("unexpected argument %q\n%w", flags.Arg(0), ErrUsage)
	}
	if strings.TrimSpace(dbPath) == "" {
		return tableOptions{}, fmt.Errorf("missing --db-path\n%w", ErrUsage)
	}

	selected, err := selectedPeriod(today, week, month, allTime, requirePeriod, defaultPeriod)
	if err != nil {
		return tableOptions{}, err
	}

	// Validate date range filters
	if queryFilters.dayFrom != "" {
		if _, err := time.Parse("2006-01-02", queryFilters.dayFrom); err != nil {
			return tableOptions{}, fmt.Errorf("invalid --filter-day-from %q: must be YYYY-MM-DD\n%w", queryFilters.dayFrom, ErrUsage)
		}
	}
	if queryFilters.dayTo != "" {
		if _, err := time.Parse("2006-01-02", queryFilters.dayTo); err != nil {
			return tableOptions{}, fmt.Errorf("invalid --filter-day-to %q: must be YYYY-MM-DD\n%w", queryFilters.dayTo, ErrUsage)
		}
	}
	if queryFilters.dayFrom != "" && queryFilters.dayTo != "" {
		from, _ := time.Parse("2006-01-02", queryFilters.dayFrom)
		to, _ := time.Parse("2006-01-02", queryFilters.dayTo)
		if from.After(to) {
			return tableOptions{}, fmt.Errorf("--filter-day-from must not be after --filter-day-to\n%w", ErrUsage)
		}
	}

	return tableOptions{dbPath: dbPath, period: selected, filters: queryFilters}, nil
}

func selectedPeriod(today bool, week bool, month bool, allTime bool, required bool, fallback period) (period, error) {
	selected := 0
	if today {
		selected++
	}
	if week {
		selected++
	}
	if month {
		selected++
	}
	if allTime {
		selected++
	}
	if selected == 0 && !required {
		return fallback, nil
	}
	if selected != 1 {
		return "", fmt.Errorf("choose exactly one of --today, --week, --month, --all-time\n%w", ErrUsage)
	}

	switch {
	case today:
		return periodToday, nil
	case week:
		return periodWeek, nil
	case month:
		return periodMonth, nil
	default:
		return periodAllTime, nil
	}
}

func periodStart(now time.Time, selected period) time.Time {
	local := now.Local()

	switch selected {
	case periodToday:
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	case periodWeek:
		// Go weekday: Sunday=0, Monday=1, ..., Saturday=6
		// We want Monday as start of week
		offset := int(local.Weekday() - time.Monday)
		if offset < 0 {
			offset += 7
		}
		monday := local.AddDate(0, 0, -offset)
		return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, local.Location())
	case periodMonth:
		return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	case periodAllTime:
		return time.Time{}
	default:
		return time.Time{}
	}
}

var ErrUsage = errors.New("usage: tokeninspector-cli --db-path PATH [--today|--week|--month|--all-time] [--session-id ID] [--provider ID] [--model ID] [--filter-day-from YYYY-MM-DD] [--filter-day-to YYYY-MM-DD]")
