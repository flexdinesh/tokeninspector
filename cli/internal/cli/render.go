package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

type column struct {
	name    string
	field   string
	numeric bool
}

func columnsForMode(g groupByMode) []column {
	base := []column{
		{name: "day", field: "day"},
		{name: "provider", field: "provider"},
		{name: "model", field: "model"},
		{name: "tps avg", field: "tpsAvg", numeric: true},
		{name: "tps mean", field: "tpsMean", numeric: true},
		{name: "tps median", field: "tpsMedian", numeric: true},
		{name: "input", field: "inputTokens", numeric: true},
		{name: "output", field: "outputTokens", numeric: true},
		{name: "reasoning", field: "reasoningTokens", numeric: true},
		{name: "cache read", field: "cacheReadTokens", numeric: true},
		{name: "cache write", field: "cacheWriteTokens", numeric: true},
		{name: "total", field: "totalTokens", numeric: true},
		{name: "requests", field: "requests", numeric: true},
		{name: "retries", field: "retries", numeric: true},
	}

	switch g {
	case groupByHour:
		return []column{
			{name: "day", field: "day"},
			{name: "hour", field: "hour"},
			{name: "provider", field: "provider"},
			{name: "model", field: "model"},
			{name: "tps avg", field: "tpsAvg", numeric: true},
			{name: "tps mean", field: "tpsMean", numeric: true},
			{name: "tps median", field: "tpsMedian", numeric: true},
			{name: "input", field: "inputTokens", numeric: true},
			{name: "output", field: "outputTokens", numeric: true},
			{name: "reasoning", field: "reasoningTokens", numeric: true},
			{name: "cache read", field: "cacheReadTokens", numeric: true},
			{name: "cache write", field: "cacheWriteTokens", numeric: true},
			{name: "total", field: "totalTokens", numeric: true},
			{name: "requests", field: "requests", numeric: true},
			{name: "retries", field: "retries", numeric: true},
		}
	case groupBySession:
		return []column{
			{name: "day", field: "day"},
			{name: "session id", field: "sessionID"},
			{name: "thinking", field: "thinkingLevels"},
			{name: "provider", field: "provider"},
			{name: "model", field: "model"},
			{name: "tps avg", field: "tpsAvg", numeric: true},
			{name: "tps mean", field: "tpsMean", numeric: true},
			{name: "tps median", field: "tpsMedian", numeric: true},
			{name: "input", field: "inputTokens", numeric: true},
			{name: "output", field: "outputTokens", numeric: true},
			{name: "reasoning", field: "reasoningTokens", numeric: true},
			{name: "cache read", field: "cacheReadTokens", numeric: true},
			{name: "cache write", field: "cacheWriteTokens", numeric: true},
			{name: "total", field: "totalTokens", numeric: true},
			{name: "requests", field: "requests", numeric: true},
			{name: "retries", field: "retries", numeric: true},
		}
	default:
		return base
	}
}

type renderRow struct {
	day              string
	hour             string
	sessionID        string
	provider         string
	model            string
	thinkingLevels   string
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

func formatWeightedTPS(throughputTokens int64, durationMs int64) string {
	if durationMs <= 0 || throughputTokens <= 0 {
		return ""
	}
	return formatTPS(float64(throughputTokens) / (float64(durationMs) / 1000))
}

func formatMeanTPS(tpsMean float64) string {
	if tpsMean <= 0 {
		return ""
	}
	return formatTPS(tpsMean)
}

func formatMedianTPS(tpsMedian float64) string {
	if tpsMedian <= 0 {
		return ""
	}
	return formatTPS(tpsMedian)
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

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	hintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("63")).Padding(0, 1)
	cellStyle   = lipgloss.NewStyle().Padding(0, 1)
	oddStyle    = cellStyle.Foreground(lipgloss.Color("252"))
	evenStyle   = cellStyle.Foreground(lipgloss.Color("245"))
	numberStyle = cellStyle.Align(lipgloss.Right)
	borderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func renderTable(rows []renderRow, g groupByMode) string {
	return renderTableWithWidth(rows, g, 0)
}

func renderTableWithWidth(rows []renderRow, g groupByMode, width int) string {
	cols := columnsForMode(g)

	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = c.name
	}

	formatted := make([][]string, 0, len(rows))
	for _, row := range rows {
		values := make([]string, len(cols))
		for i, c := range cols {
			switch c.field {
			case "day":
				values[i] = row.day
			case "hour":
				values[i] = row.hour
			case "sessionID":
				values[i] = displaySessionID(row.sessionID)
			case "thinkingLevels":
				values[i] = row.thinkingLevels
			case "provider":
				values[i] = row.provider
			case "model":
				values[i] = displayModel(row.model)
			case "tpsAvg":
				values[i] = row.tpsAvg
			case "tpsMean":
				values[i] = row.tpsMean
			case "tpsMedian":
				values[i] = row.tpsMedian
			case "inputTokens":
				values[i] = row.inputTokens
			case "outputTokens":
				values[i] = row.outputTokens
			case "reasoningTokens":
				values[i] = row.reasoningTokens
			case "cacheReadTokens":
				values[i] = row.cacheReadTokens
			case "cacheWriteTokens":
				values[i] = row.cacheWriteTokens
			case "totalTokens":
				values[i] = row.totalTokens
			case "requests":
				values[i] = row.requests
			case "retries":
				values[i] = row.retries
			default:
				values[i] = ""
			}
		}
		formatted = append(formatted, values)
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
			if col < len(cols) && cols[col].numeric {
				return numberStyle.Inherit(base)
			}
			return base
		})
	if width > 0 {
		uiTable = uiTable.Width(width)
	}

	return uiTable.String() + "\n"
}
