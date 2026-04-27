# Plan: Maximize CLI Table Vertical Space

## Context
The Bubbletea TUI runs fullscreen (`tea.WithAltScreen()`). The current `maxVisibleRows()` uses hardcoded divisors (`/2` and `/3`) that assume each table row consumes 2-3 lines. Actual rendered table rows are ~1 line each, so on a typical 50-line terminal we show ~18 rows instead of the ~37 that fit.

## Decisions (from user)
1. **Use lipgloss height measurement** for precise calculation
2. **Minimum visible row floor** of 5 rows
3. **Scroll hint always appears** (no behavior change needed)

## Implementation

### Changes to `cli/internal/cli/table.go`

1. **Add cache fields to `interactiveModel`**:
   - `cachedWidth int` — width at which cache is valid
   - `baseHeight int` — lipgloss-measured height of chrome (title + tabs + hint + borders + empty table header)
   - `perRowHeight int` — lipgloss-measured incremental height per data row

2. **Add `measureHeights()` method**:
   - Render the full chrome (title, tab bar, hint box, outer border) with an empty table to get `baseHeight`
   - Render the full chrome with a 1-row sample table to get `baseHeight + perRowHeight`
   - Cache results; invalidate when `width` or `activeTab` or `groupBy` changes (since column count affects table width/wrapping)

3. **Update `maxVisibleRows()`**:
   - If cache is warm and valid: `max(5, (m.height - m.baseHeight) / m.perRowHeight)`
   - If cache is cold or invalid: use a conservative fallback (`max(5, (m.height-14)/2)` for safety)

4. **Trigger measurement**:
   - On `tea.WindowSizeMsg` (terminal resize)
   - After tab switch or grouping change (column set changes)

### Testing
- Existing `render_test.go` golden files should remain unchanged (rendering itself doesn't change)
- `go test ./...` in `cli/` should pass
- Smoke build should pass

## Verification
```sh
cd cli
go test ./...
go build -o tokeninspector-cli .
```
