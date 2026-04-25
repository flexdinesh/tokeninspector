# tokeninspector Design

## North Star

Track OpenCode token usage locally over time, without relying on vendor dashboards.

The durable data model is a **session-centric token time series**. Every token row must relate to a `session_id`. No token data should exist without it.

TPS (tokens per second) is a first-class project metric. Do not remove persisted TPS columns, tables, or `tps avg`, `tps mean`, and `tps median` when changing token schema.

## System Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  OpenCode TUI   │     │  OpenCode Server│     │   Pi Extension  │
│  oc-tokeninspector│     │oc-tokeninspector│     │ pi-tokeninspector│
│     .tsx        │     │   -server.ts    │     │    index.ts     │
└────────┬────────┘     └────────┬────────┘     └────────┬────────┘
         │                       │                       │
         │ reads                 │ writes                │ writes
         │ (live display)        │ (worker thread)       │ (direct)
         │                       │                       │
         ▼                       ▼                       ▼
┌───────────────────────────────────────────────────────────────┐
│                SQLite DB (~/.local/state/opencode/oc-tps.sqlite) │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │ oc_token_events │  │ oc_tps_samples  │  │ oc_llm_requests │  │
│  │  (token rows)   │  │ (throughput)    │  │ (attempts)      │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │ pi_token_events │  │ pi_tps_samples  │  │ pi_llm_requests │  │
│  │  (token rows)   │  │ (throughput)    │  │ (attempts)      │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
└───────────────────────────────────────────────────────────────┘
         ▲
         │ reads (read-only)
         │
┌─────────────────┐
│   Go CLI        │
│ tokeninspector-cli│
└─────────────────┘
```

### Plugin / CLI Boundary

- **Server plugin writes** all durable OpenCode data using `bun:sqlite` in a Bun worker thread (`oc-tokeninspector-writer.ts`). Writes are queued in memory and flushed about once per second. A hard crash can lose the most recent queued batch.
- **TUI plugin reads only** — it queries `oc_tps_samples` for session averages/TTFT and estimates live TPS from `message.part.delta` events in memory.
- **Pi extension writes** using `better-sqlite3` directly from the extension event handler. Volume is low enough that synchronous writes do not block the Pi TUI.
- **CLI reads** using `modernc.org/sqlite` with a `file:` URL and `mode=ro`. It never writes.
- Both sides share **one schema file**: `schema/schema.sql`.

### Why This Boundary Matters

Plugin DB writes must stay lightweight; the TUI should never feel blocked by analytics. The CLI is free to run expensive aggregates because it only reads.

**Top priority**: plugin initialization must never block OpenCode startup. DB open and schema migration happen lazily on first write (server plugin) or inside a worker thread (server plugin token writer). If the DB is locked by another process, the plugin degrades gracefully rather than hanging OpenCode.

**TUI defensiveness**: the TUI plugin never opens the DB for writes. It queries `oc_tps_samples` read-only for the live banner. If the DB is unavailable, averages display as `-`.

**Server worker defensiveness**: the writer thread times out DB init after 10 s, runs a passive WAL checkpoint after migration to prevent unbounded WAL growth, and the main thread only flushes rows once the worker signals `ready`. If init fails, rows are dropped in-memory rather than queued forever.

## Data Model

`schema/schema.sql` is the single source of truth for table and column definitions.

### `oc_token_events`

One row per real token event, normally from an OpenCode `step-finish` part.

| Column | Meaning |
|--------|---------|
| `recorded_at_ms` | UTC millis when the event occurred |
| `session_id` | Required. OpenCode session ID |
| `message_id` | OpenCode message ID |
| `part_id` | OpenCode part ID |
| `source` | `'step-finish'` or `'message-fallback'` |
| `provider` | `'unknown'` if missing |
| `model` | `'unknown'` if missing |
| `input_tokens` | Prompt tokens |
| `output_tokens` | Completion tokens |
| `reasoning_tokens` | Reasoning / thinking tokens |
| `cache_read_tokens` | Cache read tokens |
| `cache_write_tokens` | Cache write tokens |
| `total_tokens` | `tokens.total` when present, otherwise sum of the above |

Unique on `(session_id, message_id, part_id)`.

### `oc_tps_samples`

One row per completed assistant message that has timing data.

| Column | Meaning |
|--------|---------|
| `recorded_at_ms` | UTC millis when the message completed |
| `session_id` | Required |
| `message_id` | Unique per message |
| `provider` | `'unknown'` if missing |
| `model` | `'unknown'` if missing |
| `output_tokens` | Output tokens from the message |
| `reasoning_tokens` | Reasoning tokens from the message |
| `total_tokens` | `output + reasoning` (throughput numerator) |
| `duration_ms` | Streaming duration |
| `ttft_ms` | Time to first token |
| `tokens_per_second` | `total_tokens / (duration_ms / 1000)` |

### `oc_llm_requests`

One row per LLM provider request attempt.

| Column | Meaning |
|--------|---------|
| `recorded_at_ms` | UTC millis when the request was initiated |
| `session_id` | Required |
| `message_id` | OpenCode message ID |
| `provider` | `'unknown'` if missing |
| `model` | `'unknown'` if missing |
| `attempt_index` | `1` for initial attempt, `> 1` for retries |
| `thinking_level` | `low`, `medium`, `high`, `xhigh`, or `unknown` |

### `pi_token_events`

One row per Pi assistant message completion.

Same columns as `oc_token_events` except `part_id` and `source` are omitted (Pi has no part-level granularity). `UNIQUE(session_id, message_id)`.

**Pi gap**: `reasoning_tokens` is always `0` because Pi's `Usage` type does not expose a separate reasoning count.

### `pi_tps_samples`

One row per Pi assistant message with timing data.

Identical schema to `oc_tps_samples`.

### `pi_llm_requests`

One row per Pi provider request attempt.

Identical schema to `oc_llm_requests`. `attempt_index` is always `1` because Pi does not expose retry detection to extensions.

### Schema Contract

Plugin writers auto-migrate the DB using `plugins/schema-migrate.ts`, which reads `schema/schema.sql` at init time. The migration parses `CREATE TABLE IF NOT EXISTS` and `ALTER TABLE ADD COLUMN` for missing columns.

Cross-language contract is validated by:
- `scripts/check-schema.ts` — parses SQL, Go constants, and TS types
- `cli/internal/db/schema_test.go` — Go-level contract test

Run `bun run scripts/check-schema.ts` before any schema-related commit.

## Token Semantics

`total_tokens` in `oc_token_events` means OpenCode `tokens.total` when present. Fallback:

```text
input + output + reasoning + cache_read + cache_write
```

`total_tokens` in `oc_tps_samples` is **separate**: it is `output + reasoning` only, used as the throughput numerator.

Missing `provider` or `model` must not drop token data. Store, render, and query it as `unknown`. Missing `session_id` is **not allowed**.

## Plugin Event Flow

### TUI Plugin (`oc-tokeninspector.tsx`)

The TUI plugin is **read-only** for durable data. It handles:

1. **`message.part.delta`**
   - Used **only** for live TUI display.
   - Text/reasoning deltas are estimated as `ceil(byteLength(delta) / 5)`, minimum 1 token.
   - These estimates are **not persisted**.

2. **`message.part.updated`** (tool parts only)
   - `running`, `completed`, `error`: clears live stream samples so tool time does not look like streaming TPS.

### TUI Display

The plugin registers `session_prompt_right`:

```text
TPS <live> | AVG <session avg> | TTFT <session avg ttft>
```

Live TPS uses the last 5 seconds of estimated stream deltas and hides when idle/stale. Session average and TTFT are queried from `oc_tps_samples` every `BANNER_REFRESH_MS` (default 2000 ms) and persist across TUI restarts.

### Server Plugin (`oc-tokeninspector-server.ts`)

Runs as an OpenCode server plugin and is the **sole writer** of OpenCode token data.

- **Initialization is lazy**: DB open and schema migration are deferred to the first event that requires a write (`message.part.updated` or `message.updated`). If init fails (e.g. DB locked), the plugin logs to stderr and disables tracking for that session; OpenCode startup is never blocked.
- **`chat.params`**: captures `thinking_level` from known params/options shapes before request headers are built.
- **`chat.headers`**: writes one `oc_llm_requests` row per invocation (direct DB write). Attempts are tracked in memory by `session_id:message_id:provider:model`.
- **`message.part.updated`**:
  - `step-finish`: queues a true time-series token event with source `step-finish`.
- **`message.updated`**:
  - Assistant messages queue message metadata updates: `session_id`, provider, model.
  - When a completed assistant message has no step rows, queues a `message-fallback` token row. This protects against missing `step-finish` data.
  - Completed assistant messages also queue one `oc_tps_samples` row when timing data is available. This is the durable source for CLI `tps avg`, `tps mean`, and `tps median`.
- **`session.idle`** / **`session.deleted`**: scans an in-memory message tracker for completed assistant messages, queues missing fallback rows, then flushes pending rows for that session to the writer worker. Cleans up per-session state.

### Session Lifecycle (Server Plugin)

- **`session.idle`**: scans in-memory message tracker for completed assistant messages, queues missing fallback rows, then sends pending rows for that session to the writer worker.
- **`session.deleted`**: attempts the same fallback scan, then sends pending rows for that session to the writer worker, and cleans up per-session in-memory state.

`attempt_index == 1` contributes to `requests`. `attempt_index > 1` contributes to `retries`.

Limitations: counts request attempts, not confirmed HTTP success. Does not count tool network calls, MCP traffic, auth/OAuth, provider metadata lookups, plugin-owned fetches, install/update checks, or local TUI/server API calls.

### Pi Extension (`pi-extension/index.ts`)

Runs as a Pi coding-agent extension. **One extension collects all data**: tokens, TPS, and requests (unlike OpenCode which splits this across TUI and server plugins).

- **Initialization is lazy**: DB open and schema migration are deferred to the first event that requires a write. If init fails, the extension logs to stderr and disables tracking; Pi startup is never blocked.
- **`turn_start`**: resets per-turn timing state (`requestStartAt`, `firstTokenAt`, `lastTokenAt`).
- **`before_provider_request`**: extracts `thinking_level` from the provider payload, captures provider/model from `ctx.model`, increments `turnSeq`, generates `currentTurnId`, writes one `pi_llm_requests` row.
- **`message_update`** (assistant only): records `firstTokenAt` on the first streaming update and updates `lastTokenAt` on every update.
- **`message_end`** (assistant only): reads `usage` from the completed message, writes one `pi_token_events` row; computes `durationMs` = `lastTokenAt − requestStartAt`, `ttftMs` = `firstTokenAt − requestStartAt`, and `tokens_per_second`, writes one `pi_tps_samples` row.
- **`session_shutdown`**: closes the DB connection and clears all session state.
- `reasoning_tokens` is always `0`.
- `message_id` is a single synthetic `currentTurnId` per turn, shared across `pi_llm_requests`, `pi_token_events`, and `pi_tps_samples` (Pi messages do not carry stable IDs).

## CLI Architecture

### Entry Point

`cli/cmd/tokeninspector-cli/main.go` dispatches to `cli/internal/cli.RunInteractive()`.

### Query Flow (`RunInteractive`)

1. Parse flags (`--db-path`, `--day`, `--week`, `--month`, `--group-by`, filters).
2. Validate `--db-path` exists and is a file.
3. Validate exactly one period flag.
4. Compute period start:
   - `--day`: today 00:00 local time
   - `--week`: today 00:00 minus 6 days
   - `--month`: first day of current month 00:00 local time
5. Open DB read-only.
6. Load rows asynchronously.
7. Apply optional filters in memory (session-id, provider, model, filter-day).
8. Aggregate rows.
9. Render an ASCII table in the active tab.

### Aggregation

Aggregation is SQL-side for performance. The CLI queries both `oc_*` and `pi_*` table families independently and merges results by group key in memory.

- **Token events**: `SUM(input_tokens, output_tokens, reasoning_tokens, cache_read_tokens, cache_write_tokens, total_tokens)` grouped by day/hour/session + provider + model + harness.
- **TPS samples**: window CTE for median, `AVG` for mean, `SUM(total_tokens) / SUM(duration_ms)` for avg, grouped by day/hour/session + provider + model + harness.
- **LLM requests**: `SUM(CASE WHEN attempt_index = 1 THEN 1)` for requests, `SUM(CASE WHEN attempt_index > 1 THEN 1)` for retries, grouped by day/hour/session + provider + model + harness.

The `harness` column is a literal (`'oc'` or `'pi'`) selected in the query and included in the group key. Sort order is `harness asc`, then existing order.

### Tabs

The interactive TUI has three tabs. Only columns relevant to the active tab are rendered.

| Tab | Columns |
|-----|---------|
| **tokens** (default) | day, [hour \| session id, thinking], harness, provider, model, input, output, reasoning, cache read, cache write, total |
| **tps** | day, [hour \| session id, thinking], harness, provider, model, tps avg, tps mean, tps median |
| **requests** | day, [hour \| session id, thinking], harness, provider, model, requests, retries |

### Grouping Modes

| Mode | Group Key | Sort Order |
|------|-----------|------------|
| `day` (default) | day, provider, model, harness | harness asc, day desc, provider asc, model asc |
| `hour` (`--group-by=hour`) | day, hour, provider, model, harness | harness asc, day desc, hour desc, provider asc, model asc |
| `session` (`--group-by=session`) | day, session_id, provider, model, harness | harness asc, day desc, session_id asc, provider asc, model asc |

### Rendering

- `renderTableWithWidth` builds rows as strings, calculates widths, left-aligns text columns, right-aligns numeric columns.
- Numeric columns start after the grouping columns and provider/model.
- **Compact token units**:
  - `0` → blank
  - `< 1,000` → raw integer
  - `< 1,000,000` → `<value/1000>K`
  - `>= 1,000,000` → `<value/1,000,000>M`
- Session IDs are shortened to the last 8 characters.
- Model names with `/` are shortened to the last path segment (e.g. `openai/gpt-5.5` → `gpt-5.5`).

### Filters

All filters can be repeated or comma-separated:

- `--session-id`: exact match against `session_id`
- `--provider`: exact match against `provider`
- `--model`: exact match against `model`
- `--filter-day`: local `YYYY-MM-DD` derived from `recorded_at_ms`

Filtering currently happens in memory after the period query. If the DB grows large, move filters into SQL.

## Invariants (What Can & Cannot Change)

### Must Never Change

- **Plugin init must never block OpenCode**. DB open and schema migration must be lazy (server) or inside a worker thread (TUI). Failures degrade gracefully.
- `session_id` is required for every durable row.
- TPS tables, columns, and metrics (`tps avg`, `tps mean`, `tps median`) must remain.
- Plugin DB writes must stay lightweight.
- CLI must open the DB read-only.
- Missing provider/model must be stored as `unknown`, never dropped.
- `schema/schema.sql` is the single source of truth for table definitions.

### Can Evolve With Care

- New token columns can be added if both plugin and CLI are updated.
- New grouping modes can be added if sort order, SQL, and rendering are updated.
- New filters can be added if in-memory filter logic is updated.
- Event sources can change if plugin handling, CLI expectations, and docs are updated.
- New harnesses (other agents) can be added by following the `oc_*` / `pi_*` table family pattern and updating the aggregation merge logic.

## File Organization & Naming Conventions

| Directory / File | Role |
|-----------------|------|
| `plugins/oc-tokeninspector.tsx` | TUI plugin entry point; live display, DB queries |
| `plugins/oc-tokeninspector-writer.ts` | Bun worker; SQLite writes, schema migration, pruning |
| `plugins/writer-client.ts` | Shared worker client; used by both TUI and server plugins |
| `plugins/oc-tokeninspector-server.ts` | Server plugin; durable collection, LLM request tracking |
| `plugins/types.ts` | Shared TypeScript types (plugin + worker + server) |
| `plugins/schema-migrate.ts` | Auto-migration logic parsed from `schema/schema.sql` |
| `schema/schema.sql` | Single source of truth for SQLite schema |
| `scripts/check-schema.ts` | Cross-language schema contract validator |
| `cli/cmd/tokeninspector-cli/main.go` | CLI entry point |
| `cli/internal/db/open.go` | Read-only DB open + schema version check |
| `cli/internal/db/schema.go` | Go string constants for table/column names |
| `cli/internal/db/schema_test.go` | Go schema contract test |
| `cli/internal/db/events.go` | `oc_token_events` query + filter builder |
| `cli/internal/db/aggregate.go` | SQL aggregation for all three tables + merge |
| `cli/internal/cli/flags.go` | CLI flag parsing |
| `cli/internal/cli/table.go` | Bubbletea TUI model, key handling, tab switching |
| `cli/internal/cli/render.go` | Table rendering, formatting, compact units |
| `cli/internal/cli/render_test.go` | Golden file tests for rendered tables |
| `pi-extension/index.ts` | Pi extension entry point; event handlers, DB writes |
| `pi-extension/package.json` | Pi extension dependency manifest (`better-sqlite3`)

## Testing & Verification

### Schema Changes

```sh
bun run scripts/check-schema.ts
```

### TypeScript / Plugin Changes

```sh
bun build plugins/oc-tokeninspector.tsx --target=bun --outfile=/tmp/oc-tokeninspector-check.js --external "solid-js" --external "@opentui/solid" --external "@opentui/solid/jsx-dev-runtime"
bun build plugins/oc-tokeninspector-writer.ts --target=bun --outfile=/tmp/oc-tokeninspector-writer-check.js
bun build plugins/oc-tokeninspector-server.ts --target=bun --outfile=/tmp/oc-tokeninspector-server-check.js --external "@opencode-ai/plugin"
cd pi-extension && npx tsc --noEmit --module esnext --moduleResolution node --esModuleInterop --skipLibCheck index.ts
```

### Go / CLI Changes

```sh
cd cli
go test ./...
go build -o tokeninspector-cli .
```

### Smoke Test Against Real DB

```sh
cd cli
./tokeninspector-cli --db-path ~/.local/state/opencode/oc-tps.sqlite --day
```
