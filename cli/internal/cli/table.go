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

type reloadMsg struct {
	rows []renderRow
	err  error
}

type interactiveModel struct {
	rows         []renderRow
	groupBy      groupByMode
	activeTab    tabMode
	period       period
	width        int
	height       int
	scrollOffset int
	showPopup    bool
	popupCursor  int
	ctx          context.Context
	options      tableOptions
	now          time.Time
	err          error
	cachedWidth  int
	baseHeight   int
	perRowHeight int
}

var groupingOptions = []groupByMode{groupBySession, groupByNone, groupByHour}

func (m interactiveModel) Init() tea.Cmd {
	return m.reloadCmd()
}

func (m interactiveModel) reloadCmd() tea.Cmd {
	return func() tea.Msg {
		rows, err := loadRows(m.ctx, m.options, m.now, m.groupBy)
		if err != nil {
			return reloadMsg{err: err}
		}
		return reloadMsg{rows: rows}
	}
}

func popupIndexForGroupBy(g groupByMode) int {
	for i, opt := range groupingOptions {
		if opt == g {
			return i
		}
	}
	return 0
}

const minVisibleRows = 5

func (m interactiveModel) maxVisibleRows() int {
	if m.height <= 0 {
		return 0
	}
	if m.width == m.cachedWidth && m.perRowHeight > 0 {
		available := m.height - m.baseHeight
		if available <= 0 {
			return minVisibleRows
		}
		// Leave a 3-row safety margin so wrapped rows don't push the table
		// over the terminal edge and cause the view to jump during scroll.
		maxRows := max(0, available-3) / m.perRowHeight
		return max(minVisibleRows, maxRows)
	}
	if m.groupBy == groupByHour {
		return max(minVisibleRows, (m.height-14)/3)
	}
	return max(minVisibleRows, (m.height-14)/2)
}

func (m interactiveModel) measureHeights() interactiveModel {
	if m.width <= 0 {
		return m
	}

	title := titleStyle.Render(fmt.Sprintf("Token Inspector %s", m.period))

	var tabs []string
	for i := tabTokens; i <= tabRequests; i++ {
		label := i.String()
		if i == m.activeTab {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}
	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	tabBox := sectionBorderStyle.Width(m.width - 4).Render(tabBar)

	hintText := "tab/shift+tab switch · ↑/↓ j/k scroll · g grouping · q quit  ·  99999-99999 of 99999"
	hint := hintStyle.Render(hintText)
	hintBox := sectionBorderStyle.Width(m.width - 4).Render(hint)

	emptyTable := renderTableWithWidth([]renderRow{}, m.groupBy, m.activeTab, m.width-2)
	contentBase := lipgloss.JoinVertical(lipgloss.Left, title, tabBox, emptyTable, hintBox)
	baseFull := outerBorderStyle.Width(m.width - 2).Render(contentBase)
	m.baseHeight = lipgloss.Height(baseFull)

	sampleRow := renderRow{
		day: "2006-01-01", harness: "oc", provider: "openai", model: "gpt-4o",
		inputTokens: "1000", outputTokens: "100", reasoningTokens: "10",
		cacheReadTokens: "5", cacheWriteTokens: "1", totalTokens: "1116",
		tpsAvg: "12.34", tpsMean: "56.78", tpsMedian: "45.67",
		requests: "3", retries: "1",
	}
	if m.groupBy == groupByHour {
		sampleRow.hour = "12:00"
	}
	if m.groupBy == groupBySession {
		sampleRow.sessionID = "sess_12345678"
		sampleRow.thinkingLevels = "low"
	}

	// Measure cost of a single data row (no separators).
	oneRowTable := renderTableWithWidth([]renderRow{sampleRow}, m.groupBy, m.activeTab, m.width-2)
	contentOneRow := lipgloss.JoinVertical(lipgloss.Left, title, tabBox, oneRowTable, hintBox)
	oneRowFull := outerBorderStyle.Width(m.width - 2).Render(contentOneRow)
	perDataRow := lipgloss.Height(oneRowFull) - m.baseHeight
	if perDataRow <= 0 {
		perDataRow = 1
	}

	m.perRowHeight = perDataRow
	m.cachedWidth = m.width

	return m
}

func clampScroll(offset int, totalRows int, visible int) int {
	if totalRows <= 0 || visible <= 0 {
		return 0
	}
	maxOffset := totalRows - visible
	if maxOffset < 0 {
		return 0
	}
	if offset < 0 {
		return 0
	}
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

func (m interactiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case reloadMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.rows = msg.rows
		m.scrollOffset = clampScroll(m.scrollOffset, len(m.rows), m.maxVisibleRows())
		return m, nil
	case tea.KeyMsg:
		if m.showPopup {
			return m.handlePopupKey(msg)
		}
		switch msg.Type {
		case tea.KeyTab:
			m.activeTab++
			if m.activeTab > tabRequests {
				m.activeTab = tabTokens
			}
			m = m.measureHeights()
			return m, nil
		case tea.KeyShiftTab:
			m.activeTab--
			if m.activeTab < 0 {
				m.activeTab = tabRequests
			}
			m = m.measureHeights()
			return m, nil
		case tea.KeyUp:
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
			return m, nil
		case tea.KeyDown:
			visible := m.maxVisibleRows()
			m.scrollOffset = clampScroll(m.scrollOffset+1, len(m.rows), visible)
			return m, nil
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyRunes:
			switch string(msg.Runes) {
			case "q":
				return m, tea.Quit
			case "g":
				m.showPopup = true
				m.popupCursor = popupIndexForGroupBy(m.groupBy)
				return m, nil
			case "j":
				visible := m.maxVisibleRows()
				m.scrollOffset = clampScroll(m.scrollOffset+1, len(m.rows), visible)
				return m, nil
			case "k":
				if m.scrollOffset > 0 {
					m.scrollOffset--
				}
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.measureHeights()
		m.scrollOffset = clampScroll(m.scrollOffset, len(m.rows), m.maxVisibleRows())
	}
	return m, nil
}

func (m interactiveModel) handlePopupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.popupCursor--
		if m.popupCursor < 0 {
			m.popupCursor = len(groupingOptions) - 1
		}
		return m, nil
	case tea.KeyDown:
		m.popupCursor++
		if m.popupCursor >= len(groupingOptions) {
			m.popupCursor = 0
		}
		return m, nil
	case tea.KeyEnter:
		newGroupBy := groupingOptions[m.popupCursor]
		m.showPopup = false
		if newGroupBy != m.groupBy {
			m.groupBy = newGroupBy
			m.scrollOffset = 0
			m = m.measureHeights()
			return m, m.reloadCmd()
		}
		return m, nil
	case tea.KeyEsc:
		m.showPopup = false
		return m, nil
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "q":
			m.showPopup = false
			return m, nil
		case "j":
			m.popupCursor++
			if m.popupCursor >= len(groupingOptions) {
				m.popupCursor = 0
			}
			return m, nil
		case "k":
			m.popupCursor--
			if m.popupCursor < 0 {
				m.popupCursor = len(groupingOptions) - 1
			}
			return m, nil
		case " ":
			m.popupCursor++
			if m.popupCursor >= len(groupingOptions) {
				m.popupCursor = 0
			}
			return m, nil
		}
	}
	return m, nil
}

var (
	activeTabStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("63")).
				Padding(0, 2)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Padding(0, 2)

	popupStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("212")).
			Padding(1, 2)

	popupTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	popupCursorStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	popupItemStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
)

func groupByLabel(g groupByMode) string {
	switch g {
	case groupBySession:
		return "session"
	case groupByHour:
		return "hour"
	default:
		return "day"
	}
}

func (m interactiveModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	if m.showPopup {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderPopup())
	}

	title := titleStyle.Render(fmt.Sprintf("Token Inspector %s", m.period))

	var tabs []string
	for i := tabTokens; i <= tabRequests; i++ {
		label := i.String()
		if i == m.activeTab {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}
	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	tabBox := sectionBorderStyle.Width(m.width - 4).Render(tabBar)

	visible := m.maxVisibleRows()
	hintText := "tab/shift+tab switch · ↑/↓ j/k scroll · g grouping · q quit"
	if visible > 0 && len(m.rows) > visible {
		end := m.scrollOffset + visible
		if end > len(m.rows) {
			end = len(m.rows)
		}
		hintText += fmt.Sprintf("  ·  %5d-%5d of %5d", m.scrollOffset+1, end, len(m.rows))
	}
	hint := hintStyle.Render(hintText)
	hintBox := sectionBorderStyle.Width(m.width - 4).Render(hint)

	visibleRows := m.rows
	if visible > 0 && len(m.rows) > visible {
		end := m.scrollOffset + visible
		if end > len(m.rows) {
			end = len(m.rows)
		}
		visibleRows = m.rows[m.scrollOffset:end]
	}

	body := renderTableWithWidth(visibleRows, m.groupBy, m.activeTab, m.width-2)

	content := lipgloss.JoinVertical(lipgloss.Left, title, tabBox, body, hintBox)
	return outerBorderStyle.Width(m.width - 2).Render(content)
}

func (m interactiveModel) renderPopup() string {
	title := popupTitleStyle.Render("Select grouping")
	var options []string
	for i, opt := range groupingOptions {
		cursor := "  "
		style := popupItemStyle
		if i == m.popupCursor {
			cursor = "> "
			style = popupCursorStyle
		}
		options = append(options, style.Render(cursor+groupByLabel(opt)))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, options...)
	return popupStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, "", body))
}

func RunInteractive(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
	options, err := parseTableOptions(args, stderr, false, periodWeek)
	if err != nil {
		return err
	}

	_, err = tea.NewProgram(interactiveModel{
		ctx:         ctx,
		options:     options,
		now:         now,
		groupBy:     groupBySession,
		activeTab:   tabTokens,
		popupCursor: 0,
	}, tea.WithAltScreen(), tea.WithInput(os.Stdin), tea.WithOutput(stdout)).Run()
	return err
}

func loadRows(ctx context.Context, options tableOptions, now time.Time, groupBy groupByMode) ([]renderRow, error) {
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
		DayFrom:    options.filters.dayFrom,
		DayTo:      options.filters.dayTo,
	}

	var g db.GroupBy
	switch groupBy {
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
			harness:          r.Harness,
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
