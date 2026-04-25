# Plan: Interactive-Only CLI with Tokens / TPS / Requests Tabs

## Summary
Drop the non-interactive `table` subcommand. The CLI starts directly into the bubbletea TUI. The TUI has three tabs — **tokens**, **tps**, **requests** — cycled with `Tab` / `Shift+Tab`. Only columns relevant to the active tab are rendered, fixing the too-wide table. Pressing `g` opens a popup to choose grouping: **session**, **day**, **hour** (default `session`). `q` or `ctrl+c` exits.

## Decisions made
1. Default grouping: `session`
2. Popup options: `session`, `day`, `hour` (in that order)
3. `--group-by` CLI flag removed; `g` handles it interactively
4. `thinking` column shown only in `session` grouping
5. Tab bar: styled boxes with active highlight
6. `Shift+Tab` cycles tabs in reverse

## Key changes

### 1. Entry point (`cli/cmd/tokeninspector-cli/main.go`)
- Remove `table` case from the `switch`.
- Route all non-help invocations to `RunInteractive`.
- Update `ErrUsage` string to reflect interactive-only syntax.

### 2. Flags (`cli/internal/cli/flags.go`)
- Remove `groupByFlag` and `--group-by` flag registration.
- Remove `groupBy` from `tableOptions`.
- Keep `--db-path`, `--day`, `--week`, `--month`, `--session-id`, `--provider`, `--model`, `--filter-day`.

### 3. Interactive model (`cli/internal/cli/table.go`)
- Introduce `tabMode` enum: `tabTokens`, `tabTPS`, `tabRequests`.
- Introduce `popupMode` enum for grouping popup.
- Add to `interactiveModel`:
  - `activeTab tabMode`
  - `groupBy groupByMode` (default `groupBySession`)
  - `showPopup bool`
  - `popupCursor int`
- `Update` key handling:
  - `Tab` → next tab
  - `Shift+Tab` → previous tab
  - `g` → open grouping popup
  - `q` / `ctrl+c` → `tea.Quit`
  - Popup open: `↑`/`↓`/`k`/`j` move; `Space` selects; `Enter` confirms, reloads rows from DB, closes popup; `Esc`/`q` cancels.
- `View` renders:
  - Tab bar with `lipgloss` styled boxes; active tab highlighted.
  - Table for active tab using tab-specific columns.
  - Centered popup overlay with grouped options when open.
- `loadRows` call moved into `Update` on init and after popup confirm; rows cached in model.

### 4. Rendering (`cli/internal/cli/render.go`)
- Rename `columnsForMode(g)` → `columnsForModeAndTab(g, tab)`.
- Return tab-specific columns:
  - `tokens`: grouping cols + `input`, `output`, `reasoning`, `cache read`, `cache write`, `total`
  - `tps`: grouping cols + `tps avg`, `tps mean`, `tps median`
  - `requests`: grouping cols + `requests`, `retries`
- `thinking` only for `groupBySession`.

### 5. Tests
- Remove `RunTable` and all `table` subcommand tests from `main_test.go`.
- Update `render_test.go` to assert per-tab, per-grouping golden outputs; regenerate `testdata/*.txt`.
- Add `interactiveModel.Update` unit tests for tab cycling and popup confirm/cancel.

### 6. Docs (`docs/how-it-works.md`)
- Remove `table` subcommand docs.
- Document keys: `Tab`, `Shift+Tab`, `g`, `q`.
- Update metrics and rendering sections to describe tab-scoped columns.

## Verification
- `go test ./...` must pass.
- `go build -o tokeninspector-cli ./cmd/tokeninspector-cli` must succeed.
