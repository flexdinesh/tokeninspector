package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"tokeninsights-cli/internal/db"
)

type reloadMsg struct {
	rows []renderRow
	err  error
}

type filterValuesMsg struct {
	dimension filterDimension
	values    []string
	err       error
}

type popupMode int

const (
	popupNone popupMode = iota
	popupGrouping
	popupFilterDimension
	popupFilterValues
)

type filterDimension int

const (
	filterProvider filterDimension = iota
	filterHarness
)

type interactiveModel struct {
	rows             []renderRow
	groupBy          groupByMode
	activeTab        tabMode
	period           period
	width            int
	height           int
	scrollOffset     int
	horizontalOffset int
	popup            popupMode
	popupCursor      int
	filterDimension  filterDimension
	filterValues     []string
	filterSelections map[string]bool
	filterLoading    bool
	filterErr        error
	ctx              context.Context
	options          tableOptions
	now              time.Time
	err              error
	cachedWidth      int
	baseHeight       int
	perRowHeight     int
}

var groupingOptions = []groupByMode{groupBySession, groupByNone, groupByHour}
var filterDimensions = []filterDimension{filterProvider, filterHarness}

func (m interactiveModel) Init() tea.Cmd {
	return m.reloadCmd()
}

func (m interactiveModel) reloadCmd() tea.Cmd {
	return func() tea.Msg {
		rows, err := loadRows(m.ctx, m.options, m.now, m.groupBy, m.activeTab)
		if err != nil {
			return reloadMsg{err: err}
		}
		return reloadMsg{rows: rows}
	}
}

func (m interactiveModel) filterValuesCmd(dimension filterDimension) tea.Cmd {
	return func() tea.Msg {
		values, err := loadFilterValues(m.ctx, m.options, m.now, dimension)
		return filterValuesMsg{dimension: dimension, values: values, err: err}
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
		// Leave a 3-row safety margin so dynamic content doesn't push the table
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

	title := titleStyle.Render(fmt.Sprintf("Token Insights %s", m.period))

	var tabs []string
	for i := tabTokens; i <= tabToolBreakdown; i++ {
		label := i.String()
		if i == m.activeTab {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}
	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	tabBox := sectionBorderStyle.Width(m.width - 4).Render(tabBar)

	hintText := "tab/shift+tab switch · ↑/↓ j/k scroll · ←/→ h/l scroll · home/end horizontal · g grouping · f filter · q quit  ·  99999-99999 of 99999  ·  x 99999/99999"
	if filters := activeFiltersLabel(m.options.filters); filters != "" {
		hintText += "  ·  " + filters
	}
	hint := hintStyle.Render(hintText)
	hintBox := sectionBorderStyle.Width(m.width - 4).Render(hint)

	emptyTable := renderTableViewport([]renderRow{}, m.groupBy, m.activeTab, m.tableViewportWidth(), 0)
	contentBase := lipgloss.JoinVertical(lipgloss.Left, title, tabBox, emptyTable, hintBox)
	baseFull := outerBorderStyle.Width(m.width - 2).Render(contentBase)
	m.baseHeight = lipgloss.Height(baseFull)

	sampleRow := renderRow{
		day: "2006-01-01", harness: "oc", provider: "openai", model: "gpt-4o",
		inputTokens: "1000", outputTokens: "100", reasoningTokens: "10",
		cacheReadTokens: "5", cacheWriteTokens: "1", totalTokens: "1116",
		tpsAvg: "12.34", tpsMean: "56.78", tpsMedian: "45.67",
		requests: "3", retries: "1", toolName: "bash", toolCalls: "5", toolErrors: "1",
	}
	if m.groupBy == groupByHour {
		sampleRow.hour = "12:00"
	}
	if m.groupBy == groupBySession {
		sampleRow.sessionID = "sess_12345678"
		sampleRow.thinkingLevels = "low"
	}

	// Measure cost of a single data row (no separators).
	oneRowTable := renderTableViewport([]renderRow{sampleRow}, m.groupBy, m.activeTab, m.tableViewportWidth(), 0)
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

func clampHorizontalScroll(offset int, contentWidth int, viewportWidth int) int {
	if contentWidth <= 0 || viewportWidth <= 0 || contentWidth <= viewportWidth {
		return 0
	}
	maxOffset := contentWidth - viewportWidth
	if offset < 0 {
		return 0
	}
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

func (m interactiveModel) tableViewportWidth() int {
	return max(0, m.width-2)
}

func (m interactiveModel) visibleRows() []renderRow {
	visible := m.maxVisibleRows()
	if visible <= 0 || len(m.rows) <= visible {
		return m.rows
	}
	end := m.scrollOffset + visible
	if end > len(m.rows) {
		end = len(m.rows)
	}
	return m.rows[m.scrollOffset:end]
}

func (m interactiveModel) maxHorizontalOffset(rows []renderRow) int {
	contentWidth := renderTableWidth(rows, m.groupBy, m.activeTab)
	viewportWidth := m.tableViewportWidth()
	if contentWidth <= 0 || viewportWidth <= 0 || contentWidth <= viewportWidth {
		return 0
	}
	return contentWidth - viewportWidth
}

func (m interactiveModel) clampHorizontalOffset() interactiveModel {
	rows := m.visibleRows()
	m.horizontalOffset = clampHorizontalScroll(m.horizontalOffset, renderTableWidth(rows, m.groupBy, m.activeTab), m.tableViewportWidth())
	return m
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
		m = m.clampHorizontalOffset()
		return m, nil
	case filterValuesMsg:
		if m.popup != popupFilterValues || msg.dimension != m.filterDimension {
			return m, nil
		}
		m.filterLoading = false
		m.filterErr = msg.err
		if msg.err != nil {
			return m, nil
		}
		current := m.currentFilterValues(m.filterDimension)
		m.filterValues = mergeSortedValues(msg.values, current)
		m.filterSelections = selectedValuesMap(current)
		m.popupCursor = clampPopupCursor(m.popupCursor, len(m.filterValues))
		return m, nil
	case tea.KeyMsg:
		if m.popup != popupNone {
			return m.handlePopupKey(msg)
		}
		switch msg.Type {
		case tea.KeyTab:
			m.activeTab++
			if m.activeTab > tabToolBreakdown {
				m.activeTab = tabTokens
			}
			m.scrollOffset = 0
			m.horizontalOffset = 0
			m = m.measureHeights()
			return m, m.reloadCmd()
		case tea.KeyShiftTab:
			m.activeTab--
			if m.activeTab < 0 {
				m.activeTab = tabToolBreakdown
			}
			m.scrollOffset = 0
			m.horizontalOffset = 0
			m = m.measureHeights()
			return m, m.reloadCmd()
		case tea.KeyUp:
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
			m = m.clampHorizontalOffset()
			return m, nil
		case tea.KeyDown:
			visible := m.maxVisibleRows()
			m.scrollOffset = clampScroll(m.scrollOffset+1, len(m.rows), visible)
			m = m.clampHorizontalOffset()
			return m, nil
		case tea.KeyRight:
			rows := m.visibleRows()
			m.horizontalOffset = clampHorizontalScroll(m.horizontalOffset+1, renderTableWidth(rows, m.groupBy, m.activeTab), m.tableViewportWidth())
			return m, nil
		case tea.KeyLeft:
			rows := m.visibleRows()
			m.horizontalOffset = clampHorizontalScroll(m.horizontalOffset-1, renderTableWidth(rows, m.groupBy, m.activeTab), m.tableViewportWidth())
			return m, nil
		case tea.KeyHome:
			m.horizontalOffset = 0
			return m, nil
		case tea.KeyEnd:
			rows := m.visibleRows()
			m.horizontalOffset = m.maxHorizontalOffset(rows)
			return m, nil
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyRunes:
			switch string(msg.Runes) {
			case "q":
				return m, tea.Quit
			case "g":
				m.popup = popupGrouping
				m.popupCursor = popupIndexForGroupBy(m.groupBy)
				return m, nil
			case "f":
				m.popup = popupFilterDimension
				m.popupCursor = 0
				m.filterErr = nil
				return m, nil
			case "j":
				visible := m.maxVisibleRows()
				m.scrollOffset = clampScroll(m.scrollOffset+1, len(m.rows), visible)
				m = m.clampHorizontalOffset()
				return m, nil
			case "k":
				if m.scrollOffset > 0 {
					m.scrollOffset--
				}
				m = m.clampHorizontalOffset()
				return m, nil
			case "l":
				rows := m.visibleRows()
				m.horizontalOffset = clampHorizontalScroll(m.horizontalOffset+1, renderTableWidth(rows, m.groupBy, m.activeTab), m.tableViewportWidth())
				return m, nil
			case "h":
				rows := m.visibleRows()
				m.horizontalOffset = clampHorizontalScroll(m.horizontalOffset-1, renderTableWidth(rows, m.groupBy, m.activeTab), m.tableViewportWidth())
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.measureHeights()
		m.scrollOffset = clampScroll(m.scrollOffset, len(m.rows), m.maxVisibleRows())
		m = m.clampHorizontalOffset()
	}
	return m, nil
}

func (m interactiveModel) handlePopupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.popup {
	case popupGrouping:
		return m.handleGroupingPopupKey(msg)
	case popupFilterDimension:
		return m.handleFilterDimensionKey(msg)
	case popupFilterValues:
		return m.handleFilterValuesKey(msg)
	default:
		return m, nil
	}
}

func (m interactiveModel) handleGroupingPopupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.popupCursor = movePopupCursor(m.popupCursor, len(groupingOptions), -1)
		return m, nil
	case tea.KeyDown:
		m.popupCursor = movePopupCursor(m.popupCursor, len(groupingOptions), 1)
		return m, nil
	case tea.KeyEnter:
		return m.applyGroupingPopup()
	case tea.KeySpace:
		return m.applyGroupingPopup()
	case tea.KeyEsc:
		m.popup = popupNone
		return m, nil
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "q":
			m.popup = popupNone
			return m, nil
		case "j":
			m.popupCursor = movePopupCursor(m.popupCursor, len(groupingOptions), 1)
			return m, nil
		case "k":
			m.popupCursor = movePopupCursor(m.popupCursor, len(groupingOptions), -1)
			return m, nil
		case " ":
			return m.applyGroupingPopup()
		}
	}
	return m, nil
}

func (m interactiveModel) applyGroupingPopup() (tea.Model, tea.Cmd) {
	newGroupBy := groupingOptions[m.popupCursor]
	m.popup = popupNone
	if newGroupBy != m.groupBy {
		m.groupBy = newGroupBy
		m.scrollOffset = 0
		m.horizontalOffset = 0
		m = m.measureHeights()
		return m, m.reloadCmd()
	}
	return m, nil
}

func (m interactiveModel) handleFilterDimensionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.popupCursor = movePopupCursor(m.popupCursor, len(filterDimensions), -1)
		return m, nil
	case tea.KeyDown:
		m.popupCursor = movePopupCursor(m.popupCursor, len(filterDimensions), 1)
		return m, nil
	case tea.KeyEnter:
		return m.openFilterValues()
	case tea.KeySpace:
		return m.openFilterValues()
	case tea.KeyEsc:
		m.popup = popupNone
		return m, nil
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "q":
			m.popup = popupNone
			return m, nil
		case "j":
			m.popupCursor = movePopupCursor(m.popupCursor, len(filterDimensions), 1)
			return m, nil
		case "k":
			m.popupCursor = movePopupCursor(m.popupCursor, len(filterDimensions), -1)
			return m, nil
		case " ":
			return m.openFilterValues()
		}
	}
	return m, nil
}

func (m interactiveModel) openFilterValues() (tea.Model, tea.Cmd) {
	m.filterDimension = filterDimensions[m.popupCursor]
	m.popup = popupFilterValues
	m.popupCursor = 0
	m.filterValues = nil
	m.filterSelections = selectedValuesMap(m.currentFilterValues(m.filterDimension))
	m.filterLoading = true
	m.filterErr = nil
	return m, m.filterValuesCmd(m.filterDimension)
}

func (m interactiveModel) handleFilterValuesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.popupCursor = movePopupCursor(m.popupCursor, len(m.filterValues), -1)
		return m, nil
	case tea.KeyDown:
		m.popupCursor = movePopupCursor(m.popupCursor, len(m.filterValues), 1)
		return m, nil
	case tea.KeyEnter:
		return m.applyFilterValues()
	case tea.KeySpace:
		return m.toggleFilterValue(), nil
	case tea.KeyEsc:
		m.popup = popupNone
		return m, nil
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "q":
			m.popup = popupNone
			return m, nil
		case "j":
			m.popupCursor = movePopupCursor(m.popupCursor, len(m.filterValues), 1)
			return m, nil
		case "k":
			m.popupCursor = movePopupCursor(m.popupCursor, len(m.filterValues), -1)
			return m, nil
		case " ":
			return m.toggleFilterValue(), nil
		}
	}
	return m, nil
}

func (m interactiveModel) toggleFilterValue() interactiveModel {
	if len(m.filterValues) == 0 {
		return m
	}
	if m.filterSelections == nil {
		m.filterSelections = make(map[string]bool)
	}
	value := m.filterValues[m.popupCursor]
	m.filterSelections[value] = !m.filterSelections[value]
	return m
}

func (m interactiveModel) applyFilterValues() (tea.Model, tea.Cmd) {
	if m.filterLoading || m.filterErr != nil {
		return m, nil
	}
	selected := selectedValues(m.filterValues, m.filterSelections)
	switch m.filterDimension {
	case filterProvider:
		m.options.filters.providers = stringList(selected)
	case filterHarness:
		m.options.filters.harnesses = stringList(selected)
	}
	m.popup = popupNone
	m.scrollOffset = 0
	m.horizontalOffset = 0
	m = m.measureHeights()
	return m, m.reloadCmd()
}

func movePopupCursor(cursor int, length int, delta int) int {
	if length <= 0 {
		return 0
	}
	cursor += delta
	for cursor < 0 {
		cursor += length
	}
	for cursor >= length {
		cursor -= length
	}
	return cursor
}

func clampPopupCursor(cursor int, length int) int {
	if length <= 0 || cursor < 0 {
		return 0
	}
	if cursor >= length {
		return length - 1
	}
	return cursor
}

func (m interactiveModel) currentFilterValues(dimension filterDimension) []string {
	switch dimension {
	case filterProvider:
		return []string(m.options.filters.providers)
	case filterHarness:
		return []string(m.options.filters.harnesses)
	default:
		return nil
	}
}

func selectedValuesMap(values []string) map[string]bool {
	result := make(map[string]bool)
	for _, value := range values {
		result[value] = true
	}
	return result
}

func selectedValues(values []string, selections map[string]bool) []string {
	var result []string
	for _, value := range values {
		if selections[value] {
			result = append(result, value)
		}
	}
	return result
}

func mergeSortedValues(a []string, b []string) []string {
	set := make(map[string]bool)
	for _, value := range a {
		if value != "" {
			set[value] = true
		}
	}
	for _, value := range b {
		if value != "" {
			set[value] = true
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
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

	popupTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
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

func filterDimensionLabel(dimension filterDimension) string {
	switch dimension {
	case filterProvider:
		return "provider"
	case filterHarness:
		return "harness"
	default:
		return ""
	}
}

func (m interactiveModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	if m.popup != popupNone {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderPopup())
	}

	title := titleStyle.Render(fmt.Sprintf("Token Insights %s", m.period))

	var tabs []string
	for i := tabTokens; i <= tabToolBreakdown; i++ {
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
	visibleRows := m.visibleRows()
	viewportWidth := m.tableViewportWidth()
	maxHorizontal := m.maxHorizontalOffset(visibleRows)
	horizontalOffset := clampHorizontalScroll(m.horizontalOffset, renderTableWidth(visibleRows, m.groupBy, m.activeTab), viewportWidth)

	hintText := "tab/shift+tab switch · ↑/↓ j/k scroll · ←/→ h/l scroll · home/end horizontal · g grouping · f filter · q quit"
	if visible > 0 && len(m.rows) > visible {
		end := m.scrollOffset + visible
		if end > len(m.rows) {
			end = len(m.rows)
		}
		hintText += fmt.Sprintf("  ·  %5d-%5d of %5d", m.scrollOffset+1, end, len(m.rows))
	}
	if maxHorizontal > 0 {
		hintText += fmt.Sprintf("  ·  x %5d/%5d", horizontalOffset+1, maxHorizontal+1)
	}
	if filters := activeFiltersLabel(m.options.filters); filters != "" {
		hintText += "  ·  " + filters
	}
	hint := hintStyle.Render(hintText)
	hintBox := sectionBorderStyle.Width(m.width - 4).Render(hint)

	body := renderTableViewport(visibleRows, m.groupBy, m.activeTab, viewportWidth, horizontalOffset)

	content := lipgloss.JoinVertical(lipgloss.Left, title, tabBox, body, hintBox)
	return outerBorderStyle.Width(m.width - 2).Render(content)
}

func activeFiltersLabel(f filters) string {
	var parts []string
	if len(f.providers) > 0 {
		parts = append(parts, "provider="+strings.Join([]string(f.providers), ","))
	}
	if len(f.harnesses) > 0 {
		parts = append(parts, "harness="+strings.Join([]string(f.harnesses), ","))
	}
	if len(parts) == 0 {
		return ""
	}
	return "filters: " + strings.Join(parts, " ")
}

func (m interactiveModel) renderPopup() string {
	switch m.popup {
	case popupGrouping:
		return m.renderGroupingPopup()
	case popupFilterDimension:
		return m.renderFilterDimensionPopup()
	case popupFilterValues:
		return m.renderFilterValuesPopup()
	default:
		return ""
	}
}

func (m interactiveModel) renderGroupingPopup() string {
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
	help := hintStyle.Render("space/enter = select · esc = close")
	return popupStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", help))
}

func (m interactiveModel) renderFilterDimensionPopup() string {
	title := popupTitleStyle.Render("Select filter")
	var options []string
	for i, opt := range filterDimensions {
		cursor := "  "
		style := popupItemStyle
		if i == m.popupCursor {
			cursor = "> "
			style = popupCursorStyle
		}
		options = append(options, style.Render(cursor+filterDimensionLabel(opt)))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, options...)
	help := hintStyle.Render("space/enter = select · esc = close without applying")
	return popupStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", help))
}

func (m interactiveModel) renderFilterValuesPopup() string {
	title := popupTitleStyle.Render("Filter " + filterDimensionLabel(m.filterDimension))
	var body string
	if m.filterLoading {
		body = popupItemStyle.Render("Loading values...")
	} else if m.filterErr != nil {
		body = popupItemStyle.Render(fmt.Sprintf("Error: %v", m.filterErr))
	} else if len(m.filterValues) == 0 {
		body = popupItemStyle.Render("No values available")
	} else {
		var options []string
		for i, value := range m.filterValues {
			cursor := "  "
			style := popupItemStyle
			if i == m.popupCursor {
				cursor = "> "
				style = popupCursorStyle
			}
			checked := "[ ] "
			if m.filterSelections[value] {
				checked = "[x] "
			}
			options = append(options, style.Render(cursor+checked+value))
		}
		body = lipgloss.JoinVertical(lipgloss.Left, options...)
	}
	help := hintStyle.Render("space = select · enter = apply · esc = close without applying")
	return popupStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", help))
}

func RunInteractive(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
	options, err := parseTableOptions(args, stderr, false, periodWeek)
	if err != nil {
		return err
	}

	_, err = tea.NewProgram(interactiveModel{
		ctx:              ctx,
		options:          options,
		now:              now,
		period:           options.period,
		groupBy:          groupBySession,
		activeTab:        tabTokens,
		popupCursor:      0,
		filterSelections: make(map[string]bool),
	}, tea.WithAltScreen(), tea.WithInput(os.Stdin), tea.WithOutput(stdout)).Run()
	return err
}

func filterFromOptions(options tableOptions, now time.Time) db.Filter {
	return db.Filter{
		Start:      periodStart(now, options.period),
		SessionIDs: []string(options.filters.sessionIDs),
		Providers:  []string(options.filters.providers),
		Models:     []string(options.filters.models),
		Harnesses:  []string(options.filters.harnesses),
		DayFrom:    options.filters.dayFrom,
		DayTo:      options.filters.dayTo,
	}
}

func loadFilterValues(ctx context.Context, options tableOptions, now time.Time, dimension filterDimension) ([]string, error) {
	database, err := db.Open(options.dbPath)
	if err != nil {
		return nil, err
	}
	defer database.Close()

	filter := filterFromOptions(options, now)
	switch dimension {
	case filterProvider:
		return db.AvailableProviders(ctx, database, filter)
	case filterHarness:
		return db.AvailableHarnesses(ctx, database, filter)
	default:
		return nil, nil
	}
}

func loadRows(ctx context.Context, options tableOptions, now time.Time, groupBy groupByMode, activeTab tabMode) ([]renderRow, error) {
	database, err := db.Open(options.dbPath)
	if err != nil {
		return nil, err
	}
	defer database.Close()

	f := filterFromOptions(options, now)

	var g db.GroupBy
	switch groupBy {
	case groupByHour:
		g = db.GroupByDayHour
	case groupBySession:
		g = db.GroupByDaySession
	default:
		g = db.GroupByDay
	}

	var aggRows []db.Row
	if activeTab == tabToolBreakdown {
		aggRows, err = db.AggregateToolBreakdown(ctx, database, f, g)
	} else {
		aggRows, err = db.Aggregate(ctx, database, f, g)
	}
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
			toolName:         r.ToolName,
			toolCalls:        formatTokens(r.ToolCalls),
			toolErrors:       formatTokens(r.ToolErrors),
		}
	}
	return result, nil
}
