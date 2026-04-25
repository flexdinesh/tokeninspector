package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

type tabMode int

const (
	tabTokens tabMode = iota
	tabTPS
	tabRequests
)

func (t tabMode) String() string {
	switch t {
	case tabTokens:
		return "tokens"
	case tabTPS:
		return "tps"
	case tabRequests:
		return "requests"
	default:
		return ""
	}
}

type column struct {
	name    string
	field   string
	numeric bool
}

func columnsForModeAndTab(g groupByMode, t tabMode) []column {
	grouping := []column{{name: "day", field: "day"}}
	switch g {
	case groupByHour:
		grouping = append(grouping, column{name: "hour", field: "hour"})
	case groupBySession:
		grouping = append(grouping,
			column{name: "session id", field: "sessionID"},
			column{name: "thinking", field: "thinkingLevels"},
		)
	}
	grouping = append(grouping,
		column{name: "provider", field: "provider"},
		column{name: "model", field: "model"},
	)

	switch t {
	case tabTokens:
		return append(grouping,
			column{name: "input", field: "inputTokens", numeric: true},
			column{name: "output", field: "outputTokens", numeric: true},
			column{name: "reasoning", field: "reasoningTokens", numeric: true},
			column{name: "cache read", field: "cacheReadTokens", numeric: true},
			column{name: "cache write", field: "cacheWriteTokens", numeric: true},
			column{name: "total", field: "totalTokens", numeric: true},
		)
	case tabTPS:
		return append(grouping,
			column{name: "tps avg", field: "tpsAvg", numeric: true},
			column{name: "tps mean", field: "tpsMean", numeric: true},
			column{name: "tps median", field: "tpsMedian", numeric: true},
		)
	case tabRequests:
		return append(grouping,
			column{name: "requests", field: "requests", numeric: true},
			column{name: "retries", field: "retries", numeric: true},
		)
	default:
		return grouping
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
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	hintStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("63")).Padding(0, 1)
	cellStyle      = lipgloss.NewStyle().Padding(0, 1)
	oddStyle       = cellStyle.Foreground(lipgloss.Color("252"))
	evenStyle      = cellStyle.Foreground(lipgloss.Color("245"))
	separatorStyle = cellStyle.Foreground(lipgloss.Color("240"))
	numberStyle    = cellStyle.Align(lipgloss.Right)
	borderStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	outerBorderStyle   = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240"))
	sectionBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("245"))
)

type rowMeta struct {
	isSeparator bool
}

func insertHourSeparators(rows []renderRow, formatted [][]string) ([][]string, []rowMeta) {
	if len(rows) == 0 {
		return formatted, nil
	}

	expanded := make([][]string, 0, len(formatted)*2)
	metas := make([]rowMeta, 0, len(formatted)*2)

	emptyRow := make([]string, len(formatted[0]))

	for i, values := range formatted {
		expanded = append(expanded, values)
		metas = append(metas, rowMeta{isSeparator: false})

		if i < len(rows)-1 {
			if rows[i].day != rows[i+1].day || rows[i].hour != rows[i+1].hour {
				expanded = append(expanded, emptyRow)
				metas = append(metas, rowMeta{isSeparator: true})
			}
		}
	}

	return expanded, metas
}

func renderTable(rows []renderRow, g groupByMode, tab tabMode) string {
	return renderTableWithWidth(rows, g, tab, 0)
}

func renderTableWithWidth(rows []renderRow, g groupByMode, tab tabMode, width int) string {
	cols := columnsForModeAndTab(g, tab)

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

	var metas []rowMeta
	if g == groupByHour {
		formatted, metas = insertHourSeparators(rows, formatted)
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
			if g == groupByHour && row >= 1 && row-1 < len(metas) && metas[row-1].isSeparator {
				if col < len(cols) && cols[col].numeric {
					return numberStyle.Inherit(separatorStyle)
				}
				return separatorStyle
			}
			var base lipgloss.Style
			if g == groupByHour {
				dataRowCount := 0
				for i := 1; i < row; i++ {
					if i-1 < len(metas) && !metas[i-1].isSeparator {
						dataRowCount++
					}
				}
				base = oddStyle
				if dataRowCount%2 == 0 {
					base = evenStyle
				}
			} else {
				base = oddStyle
				if row%2 == 0 {
					base = evenStyle
				}
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
