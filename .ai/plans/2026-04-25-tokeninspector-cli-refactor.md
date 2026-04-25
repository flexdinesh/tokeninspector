# Plan: Maintainable Go+SQLite patterns for tokeninspector-cli

## Decisions

- **Package split**: proper (`internal/db` + `internal/cli` + `cmd/tokeninspector-cli`)
- **Schema version check**: `PRAGMA user_version` (Option A), set in both TS writers
- **Aggregation**: SQL-side `GROUP BY` with `GroupBy` enum
- **Flag compatibility**: okay to break, no users yet
- **Tests**: golden-file tests under `testdata/`
- **Scope**: full refactor (steps 1–8)

## Schema version check A vs B

- **A (user_version)**: Writers set `PRAGMA user_version = N` after migrations. CLI reads it at open and hard-fails on mismatch. One integer, no DDL parsing. Requires plugin cooperation — fine since we own both sides.
- **B (sqlite_master shape check)**: CLI inspects `SELECT sql FROM sqlite_master` and asserts column names exist in the DDL string. Self-contained but brittle — rewrites column order, added whitespace, or `IF NOT EXISTS` can break the string match.

## Phase 1 — Foundation (parallel)

### Task A: Plugin side — set schema version
- Add `PRAGMA user_version = 1` after `CREATE TABLE IF NOT EXISTS` in `plugins/oc-tokeninspector-writer.ts`
- Add `PRAGMA user_version = 1` after `CREATE TABLE IF NOT EXISTS` in `plugins/oc-tokeninspector-server.ts`

### Task B: `internal/db` package — schema + open
- `internal/db/schema.go`: table/column constants + `supportedSchemaVersion = 1`
- `internal/db/open.go`: `Open(path) (*sql.DB, error)` with `mode=ro`, `busy_timeout(5000)`, `query_only(true)`, `foreign_keys(on)`, `PingContext`, and `PRAGMA user_version` check

## Phase 2 — Data access (parallel, depends on B)

### Task C: `internal/db/events.go`
- `Filter` struct with Start, SessionIDs, Providers, Models, Days
- `Event` struct
- `Events(ctx, db, filter) ([]Event, error)` — pushes all filters into SQL via `IN (?,?,?)` expansion helper
- `placeholders(n)` helper

### Task D: `internal/db/aggregate.go`
- `GroupBy` enum: `Day`, `DayHour`, `DaySession`
- `Aggregate(ctx, db, filter, groupBy) ([]Row, error)` — single function replacing three near-duplicates
- SQL GROUP BY for `oc_token_events` sums
- SQL GROUP BY for `oc_tps_samples` (avg/mean, median via CTE or Go fallback)
- SQL GROUP BY for `oc_llm_requests` (requests/retries, `GROUP_CONCAT(DISTINCT thinking_level)` for session mode)
- Merge three result sets by group key in Go
- Row type with raw numeric fields (formatting deferred to renderer)

## Phase 3 — CLI package + entrypoint (parallel where possible, depends on C+D)

### Task E: `internal/cli` package
- `internal/cli/flags.go`: move `groupByFlag`, `stringList`, `period`, `filters`, `tableOptions`, `parseTableOptions`, `selectedPeriod`, `periodStart`
- `internal/cli/render.go`: move `renderTable`, `renderTableWithWidth`, `displayModel`, `displaySessionID`, column-driven `[]column` table replacing magic `numberStart`
- `internal/cli/table.go`: `runTable`, `runInteractive`, `interactiveModel`

### Task F: `cmd/tokeninspector-cli/main.go`
- Flag parsing + dispatch only
- `run(ctx, args, stdout, stderr)` entrypoint

## Phase 4 — Tests (depends on all above)

### Task G: Package-level tests
- `internal/db/db_test.go`: `newTestDB(t)` helper using `:memory:`, table-driven `Events` filter tests, table-driven `Aggregate` GroupBy tests
- `internal/cli/render_test.go`: golden-file tests under `testdata/` for daily/hourly/session output

### Task H: Integration + drift tests
- `cmd/tokeninspector-cli/main_test.go`: end-to-end `run()` with seeded temp DB
- Schema-drift test: create DB with missing column, assert CLI errors cleanly

## Verification

```sh
go test ./...
go build -o tokeninspector-cli ./cmd/tokeninspector-cli
./tokeninspector-cli --db-path ~/.local/state/opencode/oc-tps.sqlite --day
```

## Rollout

- If implementation deviates, update this plan file and notify user with reasons.
- After execution: list changed files, decisions, and commands run.
