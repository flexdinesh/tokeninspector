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

type tableOptions struct {
	dbPath  string
	period  period
	filters filters
}

func parseTableOptions(args []string, stderr io.Writer, requirePeriod bool, defaultPeriod period) (tableOptions, error) {
	flags := flag.NewFlagSet("tokeninspector-cli", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var dbPath string
	var day bool
	var week bool
	var month bool
	var queryFilters filters
	flags.StringVar(&dbPath, "db-path", "", "path to tokeninspector sqlite db")
	flags.BoolVar(&day, "day", false, "show today")
	flags.BoolVar(&week, "week", false, "show current 7-day window")
	flags.BoolVar(&month, "month", false, "show current calendar month")
	flags.Var(&queryFilters.sessionIDs, "session-id", "filter by session id; repeat or comma-separate")
	flags.Var(&queryFilters.providers, "provider", "filter by provider; repeat or comma-separate")
	flags.Var(&queryFilters.models, "model", "filter by model; repeat or comma-separate")
	flags.Var(&queryFilters.days, "filter-day", "filter by local day YYYY-MM-DD; repeat or comma-separate")

	if err := flags.Parse(args); err != nil {
		return tableOptions{}, fmt.Errorf("%v\n%w", err, ErrUsage)
	}
	if flags.NArg() > 0 {
		return tableOptions{}, fmt.Errorf("unexpected argument %q\n%w", flags.Arg(0), ErrUsage)
	}
	if strings.TrimSpace(dbPath) == "" {
		return tableOptions{}, fmt.Errorf("missing --db-path\n%w", ErrUsage)
	}

	selected, err := selectedPeriod(day, week, month, requirePeriod, defaultPeriod)
	if err != nil {
		return tableOptions{}, err
	}

	return tableOptions{dbPath: dbPath, period: selected, filters: queryFilters}, nil
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
		return "", fmt.Errorf("choose exactly one of --day, --week, --month\n%w", ErrUsage)
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

var ErrUsage = errors.New("usage: tokeninspector-cli --db-path PATH [--day|--week|--month] [--session-id ID] [--provider ID] [--model ID] [--filter-day YYYY-MM-DD]")
