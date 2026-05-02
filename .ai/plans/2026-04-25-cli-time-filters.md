# Plan: CLI Time Period & Date Range Refactor

## Summary

Replace `--day` with `--today`, redefine `--week` as calendar-week-from-Monday, keep `--week` as the default, add `--all-time`, and replace the comma-separated `--filter-day` with `--filter-day-from` / `--filter-day-to` inclusive range filters with strict YYYY-MM-DD validation.

## Decisions Made

| Decision | User Choice |
|---|---|
| `--day` flag | Replace with `--today` (breaking change) |
| `--week` semantics | Calendar week from Monday 12:00 local (breaking change from rolling 7-day) |
| Default (no flag) | Keep `--week` |
| New flag | `--all-time` — no `Start` time filter |
| `--filter-day` | Remove entirely |
| New filters | `--filter-day-from YYYY-MM-DD` and `--filter-day-to YYYY-MM-DD` |
| Validation | Strict date format (`2006-01-02`), range sanity (`from ≤ to`) |

## Files to Change

1. **`cli/internal/cli/flags.go`** — flag parsing, validation, period computation
2. **`cli/internal/db/events.go`** — `Filter` struct, SQL WHERE builder
3. **`cli/internal/db/db_test.go`** — new tests for date range and all-time
4. **`cli/internal/cli/table.go`** — `loadRows` passes new filter fields
5. **`cli/README.md`** — usage, examples, flag descriptions
6. **`docs/design.md`** — architecture docs, query flow, filter docs, examples
7. **`README.md`** (root) — CLI examples

## Key Implementation Details

### `flags.go` changes

- Rename `periodDay` → `periodToday`, update string value to `"today"`
- Add `periodAllTime period = "all"`
- Update `filters` struct: remove `days stringList`, add `dayFrom string`, `dayTo string`
- Flag registrations:
  - Remove `--day`, add `--today`
  - Change `--week` help text to "show current calendar week (Mon–Sun)"
  - Add `--all-time` boolean flag
  - Remove `--filter-day`, add `--filter-day-from` and `--filter-day-to` (`StringVar`)
- `selectedPeriod()`: add `allTime` bool parameter; if none selected, return `periodWeek` (unchanged default)
- `periodStart()`:
  - `periodToday`: today 00:00 local (unchanged logic)
  - `periodWeek`: compute Monday of current week. Go `Weekday()`: Sun=0, Mon=1…Sat=6. Offset = `weekday - Monday`; if negative, add 7. Return `local.AddDate(0,0,-offset)` truncated to 00:00.
  - `periodMonth`: first of month 00:00 (unchanged)
  - `periodAllTime`: return `time.Time{}` (zero)
- Date validation helper:
  ```go
  func parseDate(s string) (time.Time, error) {
      if s == "" { return time.Time{}, nil }
      return time.Parse("2006-01-02", s)
  }
  ```
- In `parseTableOptions`:
  - After flag parse, validate `dayFrom` and `dayTo` with `parseDate`; non-empty but invalid → `ErrUsage`
  - If both provided and `from.After(to)` → `ErrUsage`
- Update `ErrUsage` string

### `events.go` changes

- `Filter` struct: remove `Days []string`, add `DayFrom string`, `DayTo string`
- `buildFilterArgs`:
  - Only append `Start >= ?` when `!f.Start.IsZero()`
  - Remove `Days IN (...)` block
  - Add `date(...) >= ?` when `DayFrom` non-empty
  - Add `date(...) <= ?` when `DayTo` non-empty

### `table.go` changes

- `loadRows`: map `options.filters.dayFrom` → `f.DayFrom`, `options.filters.dayTo` → `f.DayTo`
- For `all-time`, `Start` is zero time, so `buildFilterArgs` skips the start clause

### Tests to add in `db_test.go`

- `TestFilterDayRange` — insert events across 3 days, filter with `DayFrom`/`DayTo`, assert only middle day returned
- `TestFilterDayRangeSingleDay` — from==to returns exactly that day
- `TestAggregateAllTime` — no `Start` set, verify events from all dates returned
- `TestPeriodStartWeekMonday` — table-driven: given specific `time.Time` values (Sunday, Monday, Wednesday, Saturday), assert `periodStart` returns the correct Monday 00:00

### Tests to add in flag parsing

- Create `flags_test.go` for:
  - `--today` selects `periodToday`
  - `--week` selects `periodWeek`
  - `--month` selects `periodMonth`
  - `--all-time` selects `periodAllTime`
  - No flag → `periodWeek` (default)
  - Multiple period flags → `ErrUsage`
  - Invalid `--filter-day-from` → `ErrUsage`
  - `--filter-day-from` > `--filter-day-to` → `ErrUsage`
  - Valid date range → success

### Documentation updates

- `cli/README.md`: replace all `--day` with `--today`; update `--week` description; add `--all-time`; replace `--filter-day` section with `--filter-day-from` / `--filter-day-to` section; update all examples
- `docs/design.md`: update Query Flow section (step 4 period start computation, step 5 default); update Filters section; update CLI examples
- `README.md` (root): update CLI examples

## Tradeoffs & Risks

| Risk | Mitigation |
|---|---|
| `--week` breaking change (rolling 7-day → Mon-based) | User explicitly requested this; clearly document in README |
| `--day` removed (breaking) | User explicitly requested; clearly document |
| `--filter-day` removed (breaking) | User explicitly requested; clearly document replacement |
| `--all-time` on large DB may be slow | Acceptable because user must explicitly opt in; SQL still groups |
| `--filter-day-from/to` without period flag means range filters act on all-time | Correct and expected; intersection with `--today` etc. still works |

## Verification Steps

1. `cd cli && go test ./...` — all existing + new tests pass
2. `cd cli && go build -o tokeninsights-cli .` — compiles
3. Manual smoke test: `./tokeninsights-cli --db-path ~/.local/state/opencode/oc-tps.sqlite --today`
4. Manual smoke test: `./tokeninsights-cli --db-path ~/.local/state/opencode/oc-tps.sqlite --week`
5. Manual smoke test: `./tokeninsights-cli --db-path ~/.local/state/opencode/oc-tps.sqlite --all-time`
6. Manual smoke test: `./tokeninsights-cli --db-path ~/.local/state/opencode/oc-tps.sqlite --month --filter-day-from 2026-04-20 --filter-day-to 2026-04-25`

## Execution Guidance

If implementation deviates from this plan, update the saved plan file to reflect the latest approved state and surface the deviation to the user.
