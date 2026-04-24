# Proposal: Maintainable Go+SQLite patterns for `tokeninspector-cli`

## Context

- `tokeninspector-cli` is a read-only Go CLI over a SQLite DB written by the `oc-tokeninspector` OpenCode TUI plugin (TypeScript).
- Single query today; schema is small; low churn.
- Driver: `modernc.org/sqlite` (pure Go). Flat `main.go`, no migrations, no codegen, in-memory filtering/aggregation.
- Two-repo project: writer (plugin) + reader (this CLI). Schema contract is implicit.

Goal: align with patterns used by production Go+SQLite CLIs (atuin, litestream, sq, grype/syft, pocketbase, dolt, sqlc itself) without over-engineering.

---

## Current-state audit

| Dimension | Production norm | This CLI | Verdict |
|---|---|---|---|
| Driver | `modernc.org/sqlite` for portable CLIs | `modernc.org/sqlite` | aligned |
| Open helper | single func, pragmas, ctx | single func, `mode=ro` only | partial |
| `busy_timeout` pragma | always set | missing | gap |
| `query_only` pragma (reader) | common | missing | minor gap |
| Reader schema version check | `PRAGMA user_version` or shape check | none | gap |
| Schema contract between writer/reader | one source of truth | split across code, tests, two docs | significant gap |
| Table name constant | single const | literal scattered; tests use stale `oc_tps_samples` vs code `oc_token_events` | bug risk |
| Filters | pushed into SQL | in-memory after fetch | scales poorly |
| Aggregation | SQL `GROUP BY` or parameterized | 3 near-duplicate Go funcs | duplication |
| Codegen (sqlc) | adopt at ~3+ queries | 1 query | not needed yet |
| Tests | real SQLite, fresh DB | real SQLite but stale table name | likely broken |
| Distribution | pure-Go static | pure-Go | aligned |
| Observability | optional `--debug-sql` | none | optional |

Key drift risk: AGENTS.md already warns contributors to update plugin + CLI together. A runtime schema-contract check would enforce it.

---

## Proposed target architecture

### Code organisation

Move from flat `main.go` to:

```
/cmd/tokeninspector-cli/main.go    # flag parsing, dispatch only
/internal/db/
  open.go            # Open(path) *sql.DB with pragmas
  schema.go          # table/column constants, required-schema check
  events.go          # Events(ctx, Filter) ([]Event, error)
  aggregate.go       # single aggregator parameterized by GroupBy
/internal/cli/
  table.go           # runTable; depends on internal/db
  render.go          # renderTable
  flags.go           # groupByFlag, stringList
main_test.go         # stays, or split per package
```

Tradeoffs:
- **Pro**: testable boundaries, table name in one place, reusable if a second command (`json`, `csv`) lands.
- **Con**: more files for a currently single-command CLI. Acceptable because the duplication in aggregation already justifies a package boundary.

Minimalist alternative: keep flat layout but extract a `schema.go` file with constants + `db.go` with `Open`/`Query`. Lower churn, still fixes the drift bug. Recommended if you want the smallest possible step.

### Driver

Keep `modernc.org/sqlite`. No change. Rationale:
- Pure Go, trivial `CGO_ENABLED=0` builds for goreleaser.
- Perf is not a bottleneck for a one-shot aggregation CLI.
- Tradeoff vs `mattn/go-sqlite3`: cgo version is ~20–40% faster on heavy workloads and has richer extension support. Not worth the cross-compile pain here.

### DB open helper

`internal/db/open.go` builds DSN with:

- `mode=ro`
- `_pragma=busy_timeout(5000)`
- `_pragma=query_only(true)`
- `_pragma=foreign_keys(on)` (harmless for a reader; keeps parity if ever used for writes in a test)

Plus `db.PingContext(ctx)` to fail fast on corrupt/locked files.

Tradeoffs:
- **Pro**: matches atuin/sq/litestream norms; clean errors on transient WAL contention.
- **Con**: none meaningful.

### Schema contract

Two options, pick one:

**Option A — `PRAGMA user_version` check (lightest)**

- Plugin sets `PRAGMA user_version = N` after each migration.
- CLI declares `const supportedSchemaVersion = N` and errors if the DB reports a different or zero value.
- Pros: one integer, zero SQL parsing.
- Cons: requires the plugin to set it; coordinates with TS side.

**Option B — `sqlite_master` shape check**

- CLI runs `SELECT sql FROM sqlite_master WHERE type='table' AND name='oc_token_events'` at startup and asserts required column names appear in the DDL.
- Pros: doesn't require plugin cooperation; self-validating.
- Cons: string matching against DDL is brittle if the plugin rewrites the table.

**Recommended**: Option A if you control the plugin (you do). Option B as the fallback if the plugin can't adopt `user_version` quickly. Can layer both.

### Table and column names

One file, `internal/db/schema.go`:

```go
const TableEvents = "oc_token_events"

const (
    ColRecordedAtMs    = "recorded_at_ms"
    ColSessionID       = "session_id"
    ColProvider        = "provider"
    ColModel           = "model"
    ColInputTokens     = "input_tokens"
    ColOutputTokens    = "output_tokens"
    ColReasoningTokens = "reasoning_tokens"
    ColCacheReadTokens = "cache_read_tokens"
    ColCacheWriteTokens= "cache_write_tokens"
    ColTotalTokens     = "total_tokens"
)
```

SQL strings built from these constants. Tests use the same constants to `CREATE TABLE`. Fixes the `oc_tps_samples` vs `oc_token_events` drift immediately.

### Data access pattern

Single typed entrypoint, no ORM, no sqlc (yet):

```go
type Filter struct {
    Start      time.Time
    SessionIDs []string
    Providers  []string
    Models     []string
    Days       []string // YYYY-MM-DD, local
}

type Event struct {
    RecordedAtMs     int64
    SessionID        string
    Provider         string
    Model            string
    InputTokens      int64
    OutputTokens     int64
    ReasoningTokens  int64
    CacheReadTokens  int64
    CacheWriteTokens int64
    TotalTokens      int64
}

func Events(ctx context.Context, db *sql.DB, f Filter) ([]Event, error)
```

Inside `Events`:
- Build WHERE with bound params. `IN (?,?,?)` expansion for slice filters.
- Push `--filter-day` into SQL via `date(recorded_at_ms/1000, 'unixepoch', 'localtime') IN (?, ?)`.
- Still return raw events when Go-side aggregation is needed; see next section for pushing aggregation down.

Tradeoffs:
- **Pro**: replaces in-memory filter loop (`main.go:284`), uses any indexes on `recorded_at_ms`, scales to month windows and high event counts.
- **Con**: SQL string concat with dynamic `IN (...)` lists needs care. Use a small helper `placeholders(n int) string`.

### Aggregation

Replace the three near-duplicate Go funcs with either:

**Option A — SQL-side aggregation (preferred)**

One function, one `GROUP BY` assembled from a `GroupBy` enum:

```go
type GroupBy int
const (
    GroupByDay GroupBy = iota
    GroupByDayHour
    GroupByDaySession
)

func Aggregate(ctx context.Context, db *sql.DB, f Filter, g GroupBy) ([]Row, error)
```

Group expression:
- `GroupByDay`: `date(recorded_at_ms/1000,'unixepoch','localtime')`
- `GroupByDayHour`: same + `strftime('%H:00', ...)`
- `GroupByDaySession`: day + `session_id`

Pros: single source of truth for aggregation; collapses three funcs to one; SQLite does the math.
Cons: SQL templating slightly more complex; local-time handling must match current Go formatting exactly. Verify with existing tests.

**Option B — Parameterized Go aggregator**

Keep fetch-then-aggregate but parameterize the key. Less invasive, still deduplicates.

Pros: smallest diff; keeps time formatting in Go.
Cons: still scans full result set in memory; does not fix scaling.

**Recommended**: Option A. Option B only if you want a minimal refactor this session.

### Rendering

Leave `renderTable` alone functionally. Extract to `internal/cli/render.go`. The column-start-index logic (`numberStart := 3` / `4`) becomes explicit:

```go
type column struct {
    name    string
    numeric bool
}
```

Row builder iterates columns; numeric ones right-align. Eliminates the magic index.

Tradeoffs:
- **Pro**: adding a new group-by mode no longer needs an index tweak.
- **Con**: small refactor with no behaviour change.

### Testing patterns

Align with atuin/sq/grype conventions:

- **Test helper** `newTestDB(t *testing.T) *sql.DB` that opens `file::memory:?cache=shared` or a `t.TempDir()` file, runs `CREATE TABLE` using the schema constants, returns a ready DB.
- **Seed helpers** `insertEvent(t, db, Event)` using the same column constants.
- **Table-driven tests** for `Events` (filter combinations) and `Aggregate` (each `GroupBy`).
- **Golden-file tests** for `renderTable` output under each group-by mode; store under `testdata/`.
- **Integration test**: end-to-end `run(ctx, args, stdout, stderr)` with a seeded temp DB; compare stdout to a golden file.
- **Schema-drift test**: create a table missing a required column; assert the CLI errors out with a clear message.

Tradeoffs:
- **Pro**: removes the current stale-name risk; regression safety for aggregation math; CLI output becomes reviewable as diffs.
- **Con**: golden files need maintenance when output format changes intentionally — standard cost.

### Observability (optional, deferred)

- `--debug-sql` flag that prints the rendered SQL + bound params to stderr before execution.
- Mirrors `sq` and `usql`. Zero runtime cost when unset.

### Migrations

Not adopting any migration tool in the CLI. Reader owns only a version check. Writer (plugin) owns migrations on its side.

Tradeoff: if the plugin ever needs to be rewritten in Go, adopt `goose` with `//go:embed migrations/*.sql` at that point. Not now.

### sqlc

Defer. Adopt when query count reaches ~3+ or when a second developer joins. Current single-query CLI does not clear the threshold.

---

## Refactor plan, ordered by leverage

1. **Single-source schema constants.** New `internal/db/schema.go` (or a top-level `schema.go` in the flat layout). Update `main.go` and `main_test.go` to use them. Fixes the `oc_tps_samples` vs `oc_token_events` drift. Smallest possible high-value change.
2. **DSN pragmas.** Add `busy_timeout(5000)`, `query_only(true)` to `openDB`. Add `db.PingContext`. Three-line change.
3. **Schema version check.** Read `PRAGMA user_version`; compare to constant; error cleanly on mismatch. Coordinate plugin change separately.
4. **Extract `internal/db` package with `Filter`, `Event`, `Events()`.** Move the `SELECT`. Push all filters into SQL with bound params and `IN (?,?,?)` expansion.
5. **Single parameterized aggregator.** SQL-side `GROUP BY` driven by `GroupBy` enum. Delete `aggregateDailySamples`, `aggregateHourlySamples`, `aggregateSessionSamples`.
6. **Extract `internal/cli` package with `runTable`, flags, renderer.** `main.go` shrinks to flag dispatch.
7. **Column-driven renderer.** Replace the numeric-start-index logic with a `[]column` table.
8. **Test overhaul.** `newTestDB` helper, table-driven tests, golden files for rendered output, schema-drift test.
9. **Optional: `--debug-sql` flag.**
10. **Defer: sqlc, goose, otelsql.** Revisit after step 8 lands and query count grows.

---

## Tradeoff summary

| Change | Effort | Risk | Payoff |
|---|---|---|---|
| Schema constants | XS | none | fixes a real bug |
| DSN pragmas + ping | XS | none | robustness |
| Version check | S | needs plugin change | prevents silent drift |
| Filters in SQL | S | SQL templating needs care | scales, removes in-memory loop |
| SQL aggregation | M | local-time parity must be preserved | deletes 3 duplicate funcs |
| Package split | M | churn across test files | testability, future second command |
| Column-driven renderer | S | output parity tests cover it | removes magic index |
| Test overhaul | M | upfront cost | regression safety, review-friendly |
| sqlc | L | premature | none yet |
| goose in CLI | L | wrong side of project | none |
| otel / debug sql | S | none | nice-to-have |

---

## Decisions to confirm before execution

- **Package split (flat vs `internal/db` + `internal/cli`)** — minimal vs proper.
- **Schema version check mechanism** — `PRAGMA user_version` (needs plugin buy-in) vs `sqlite_master` shape check (self-contained) vs both.
- **Aggregation: SQL-side vs Go-side parameterized** — scales-and-deletes vs smallest-diff.
- **CLI flag compatibility** — strictly additive, or willing to rename/reshape flags now.
- **Golden-file tests** — adopt, or keep assertion-style only.
- **Scope for next session** — steps 1–3 only, 1–5, or full 1–8.

---

## Unresolved questions to explore later

- plugin writes — still `oc_tps_samples` anywhere, or fully migrated to `oc_token_events`?
- does the plugin set `PRAGMA user_version`? willing to add it?
- expected row volume per month window — drives urgency of SQL-side filtering.
- indexes on `oc_token_events` — any on `recorded_at_ms`, `session_id`?
- is there ever a case where reader runs against a DB being actively written to by the plugin? (affects WAL/busy_timeout stance).
- acceptable to break flag compatibility, or strictly additive changes only?
- reader behaviour on schema mismatch — hard fail, or warn and best-effort continue?
- should `tokeninspector-cli` eventually gain a second subcommand (`json`, `csv`)? influences package split urgency.
- local-time parity: if aggregation moves to SQL, does `date(..., 'localtime')` match Go's `time.Local` formatting across DST boundaries?
- test DB strategy: `:memory:` shared cache vs `t.TempDir()` file — pick one convention.
- do we want `--debug-sql` from day one, or defer until a second query lands?
- is there value in publishing `internal/db` as a reusable library for other OpenCode consumers, or keep it `internal/`?
