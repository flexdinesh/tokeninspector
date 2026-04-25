package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"tokeninspector-cli/internal/db"
)

type interactiveModel struct {
	rows    []renderRow
	groupBy groupByMode
	period  period
	width   int
	height  int
}

func (model interactiveModel) Init() tea.Cmd {
	return nil
}

func (model interactiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch next := msg.(type) {
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

func RunTable(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
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

func RunInteractive(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
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

func loadRows(ctx context.Context, options tableOptions, now time.Time) ([]renderRow, error) {
	start := periodStart(now, options.period)

	database, err := db.Open(options.dbPath)
	if err != nil {
		return nil, err
	}
	defer database.Close()

	f := db.Filter{
		Start:      start,
		SessionIDs: []string(options.filters.sessionIDs),
		Providers:  []string(options.filters.providers),
		Models:     []string(options.filters.models),
		Days:       []string(options.filters.days),
	}

	var g db.GroupBy
	switch options.groupBy {
	case groupByHour:
		g = db.GroupByDayHour
	case groupBySession:
		g = db.GroupByDaySession
	default:
		g = db.GroupByDay
	}

	aggRows, err := db.Aggregate(ctx, database, f, g)
	if err != nil {
		return nil, err
	}

	result := make([]renderRow, len(aggRows))
	for i, r := range aggRows {
		result[i] = renderRow{
			day:              r.Day,
			hour:             r.Hour,
			sessionID:        r.SessionID,
			provider:         r.Provider,
			model:            r.Model,
			thinkingLevels:   r.ThinkingLevels,
			tpsAvg:           formatWeightedTPS(r.ThroughputTokens, r.DurationMs),
			tpsMean:          formatMeanTPS(r.TpsMean),
			tpsMedian:        formatMedianTPS(r.TpsMedian),
			inputTokens:      formatTokens(r.InputTokens),
			outputTokens:     formatTokens(r.OutputTokens),
			reasoningTokens:  formatTokens(r.ReasoningTokens),
			cacheReadTokens:  formatTokens(r.CacheReadTokens),
			cacheWriteTokens: formatTokens(r.CacheWriteTokens),
			totalTokens:      formatTokens(r.TotalTokens),
			requests:         formatTokens(r.Requests),
			retries:          formatTokens(r.Retries),
		}
	}
	return result, nil
}
