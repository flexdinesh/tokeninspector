# Plan: Hour-grouped table separators

## Summary
When rendering the CLI table in `groupByHour` mode, insert an empty-row separator after every group boundary (day change or hour change). Preserve alternating row colors for data rows and style separators dimly. Adjust TUI scroll math so separators don't overflow the visible window.

## Key Implementation Changes

### 1. `cli/internal/cli/render.go` — `renderTableWithWidth`
- After building `formatted [][]string`, walk `rows` to detect group boundaries (`day` or `hour` changes vs next row).
- Build an `expanded [][]string` with empty rows inserted at each boundary.
- Build a parallel `rowMetas []rowMeta` slice so `StyleFunc` can:
  - Apply a dim neutral style to separator rows
  - Preserve odd/even alternating colors **only on data rows** (skip separators in the count)
- Pass `expanded` to `lipgloss/table`. No changes to column definitions or formatting functions.

### 2. `cli/internal/cli/table.go` — `maxVisibleRows`
- When `m.groupBy == groupByHour`, reduce the row budget to account for separator rows.
- Use `max(1, (height-8)/3)` as a conservative estimate (day/session keep `max(1, (height-8)/2)`).

### 3. `cli/internal/cli/render_test.go`
- Update `TestRenderTableHourlyTokens` to use **3 rows**: 2 rows sharing one hour, 1 row in a different hour, so the separator is exercised.
- Regenerate `testdata/render_hourly_tokens.txt` golden file via `UPDATE_GOLDEN=1`.

### 4. Verification
- `cd cli && go test ./...`
- `cd cli && go build -o tokeninsights-cli .`

## Decisions Made
| Question | Answer |
|---|---|
| Separator after single-row groups? | Yes, always |
| Separator appearance | Empty cells with borders (dim foreground) |
| Day boundaries too? | Yes, insert separator on any day or hour change |

## Tradeoffs / Risks
- **TUI visible rows**: The `(height-8)/3` estimate may show slightly fewer data rows than could fit when separator density is low. Acceptable — prevents overflow when density is high.
- **Golden file churn**: Hourly golden file will change shape; test must include multi-row data to be meaningful.
- **Scroll indicator**: Keeps counting `renderRow`s (not visual lines), which is consistent with j/k scroll behavior.

## Open Questions
None — all answered above.

## Execution Guidance
If implementation deviates (e.g., separator style needs adjustment, or scroll math proves too conservative), update this plan and surface the deviation before continuing.
