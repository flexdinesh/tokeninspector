# tokeninspector: How It Works

`tokeninspector` records local OpenCode token usage and prints aggregate token usage tables.

- `plugins/oc-tokeninspector.tsx`: OpenCode TUI plugin that records token and TPS data to SQLite.
- `cli/`: Go CLI that reads the SQLite DB and renders aggregate tables.

## Install OpenCode Plugin

Add the plugin path to OpenCode TUI config:

```json
{
  "plugin": [
    "/Users/dineshpandiyan/workspace/tokeninspector/plugins/oc-tokeninspector.tsx"
  ]
}
```

Plugin id: `oc-tokeninspector`.

## Purpose

North star: session-centric token time series.

Every durable token row has `session_id`. Token usage should be queryable by time, session, provider, and model without vendor dashboards.

TPS is also a first-class project metric. Do not remove `oc_tps_samples`, `tps avg`, `tps mean`, or `tps median` when changing token columns.

## Storage

Default DB path:

```text
api.state.path.state/oc-tps.sqlite
```

On this machine:

```text
~/.local/state/opencode/oc-tps.sqlite
```

The DB filename remains `oc-tps.sqlite` for compatibility with existing local paths, but durable tables are `oc_token_events` and `oc_tps_samples`.

Plugin options:

```json
{
  "dbPath": "oc-tps.sqlite",
  "retentionDays": 365
}
```

`dbPath` may be absolute. Relative paths resolve under `api.state.path.state`.

`retentionDays` defaults to `365`. Values `<= 0` keep data forever.

The plugin uses `bun:sqlite`, WAL, and `busy_timeout = 5000`.

The CLI opens the DB read-only using `modernc.org/sqlite` with a `file:` URL and `mode=ro`.

## Token Event Table

Table name:

```text
oc_token_events
```

Columns:

```text
id INTEGER PRIMARY KEY AUTOINCREMENT
recorded_at TEXT NOT NULL
recorded_at_ms INTEGER NOT NULL
session_id TEXT NOT NULL
message_id TEXT NOT NULL
part_id TEXT NOT NULL
source TEXT NOT NULL CHECK (source IN ('step-finish', 'message-fallback'))
provider TEXT NOT NULL DEFAULT 'unknown'
model TEXT NOT NULL DEFAULT 'unknown'
input_tokens INTEGER NOT NULL
output_tokens INTEGER NOT NULL
reasoning_tokens INTEGER NOT NULL
cache_read_tokens INTEGER NOT NULL
cache_write_tokens INTEGER NOT NULL
total_tokens INTEGER NOT NULL
UNIQUE(session_id, message_id, part_id)
```

Indexes:

```text
recorded_at_ms
(session_id, recorded_at_ms)
(provider, model, recorded_at_ms)
```

Required columns read by the CLI:

```text
recorded_at_ms
session_id
provider
model
input_tokens
output_tokens
reasoning_tokens
cache_read_tokens
cache_write_tokens
total_tokens
```

## TPS Table

Table name:

```text
oc_tps_samples
```

Columns:

```text
id INTEGER PRIMARY KEY AUTOINCREMENT
recorded_at TEXT NOT NULL
recorded_at_ms INTEGER NOT NULL
session_id TEXT NOT NULL
message_id TEXT NOT NULL UNIQUE
provider TEXT NOT NULL DEFAULT 'unknown'
model TEXT NOT NULL DEFAULT 'unknown'
output_tokens INTEGER NOT NULL
reasoning_tokens INTEGER NOT NULL
total_tokens INTEGER NOT NULL
duration_ms INTEGER NOT NULL
ttft_ms INTEGER NOT NULL
tokens_per_second REAL NOT NULL
```

TPS `total_tokens` is separate from token-event `total_tokens`. It is `output + reasoning`, used as the throughput numerator.

Indexes:

```text
recorded_at_ms
(session_id, recorded_at_ms)
(provider, model, recorded_at_ms)
```

Required TPS columns read by the CLI:

```text
recorded_at_ms
session_id
provider
model
total_tokens
duration_ms
tokens_per_second
```

## Token Semantics

`total_tokens` uses OpenCode `tokens.total` when present.

Fallback formula:

```text
input + output + reasoning + cache.read + cache.write
```

Missing provider/model is stored and rendered as `unknown`. Missing session id is not allowed.

## Plugin Event Flow

`message.part.delta`

Used only for live TUI display. Text/reasoning deltas are estimated as `ceil(byteLength(delta) / 5)`, minimum 1 token. These estimates are not persisted.

`message.part.updated`

When `part.type == "step-finish"`, write a true time-series token event with source `step-finish`.

If provider/model metadata is not known yet, write the row with `unknown`; later `message.updated` backfills rows for the message.

Tool parts still affect live TPS timing:

- `pending`: records first response time if text has not arrived yet.
- `running`: records `lastToolCallAt`.
- `running`, `completed`, `error`: clears live stream samples so tool time does not look like streaming TPS.

`message.updated`

Assistant messages update message metadata: `session_id`, provider, model.

When a completed assistant message has no step rows, queue a `message-fallback` token row. This protects against missing step-finish data.

Completed assistant messages also write one `oc_tps_samples` row when timing data is available. This is the durable source for CLI `tps avg`, `tps mean`, and `tps median`.

`session.idle`

Scans current session state for completed assistant messages, queues missing fallback rows, then flushes pending rows for that session.

`session.deleted`

Attempts the same fallback scan, then flushes pending rows for that session.

Plugin dispose

Scans seen sessions for fallback rows, flushes all pending rows, unsubscribes events, clears timer, and closes DB.

## TUI Display

The plugin registers `session_prompt_right`.

Display format:

```text
TPS <live> | AVG <session avg> | TTFT <session avg ttft>
```

Live TPS uses the last 5 seconds of estimated stream deltas and hides when idle/stale. Session average and TTFT are in-memory only and reset when the TUI process restarts.

## CLI Command

Main command:

```sh
tokeninspector-cli table --db-path ~/.local/state/opencode/oc-tps.sqlite --day
```

Usage shape:

```text
tokeninspector-cli table --db-path PATH (--day|--week|--month) [--group-by=hour|session] [--session-id ID] [--provider ID] [--model ID] [--filter-day YYYY-MM-DD]
```

Only one of `--day`, `--week`, or `--month` is allowed.

Only one `--group-by` is allowed. Valid values are `hour` and `session`.

## Query Flow

`runTable`:

1. Parse flags.
2. Validate `--db-path`.
3. Validate exactly one period flag.
4. Compute period start.
5. Query rows where `recorded_at_ms >= periodStart`.
6. Apply optional filters in memory.
7. Aggregate rows.
8. Render an ASCII table.

Period starts:

```text
--day: today 00:00 local time
--week: today 00:00 local time minus 6 days
--month: first day of current month 00:00 local time
```

The CLI uses local time for day/hour buckets and `--filter-day` matching.

## Filters

All filters can be repeated or comma-separated.

Examples:

```sh
tokeninspector-cli table --db-path ~/.local/state/opencode/oc-tps.sqlite --week --provider openai --provider github-copilot
tokeninspector-cli table --db-path ~/.local/state/opencode/oc-tps.sqlite --week --provider openai,github-copilot
tokeninspector-cli table --db-path ~/.local/state/opencode/oc-tps.sqlite --month --filter-day 2026-04-24,2026-04-23
tokeninspector-cli table --db-path ~/.local/state/opencode/oc-tps.sqlite --month --session-id ses_abc,ses_xyz
```

Filter implementation:

```text
--session-id: exact match against session_id
--provider: exact match against provider
--model: exact match against model
--filter-day: local YYYY-MM-DD derived from recorded_at_ms
```

Filtering currently happens in memory after the period query. If the DB grows large, move filters into SQL.

## Aggregation Modes

Default mode:

```text
group by day, provider, model
```

`--group-by=hour`:

```text
group by day, hour, provider, model
```

The `hour` column is inserted after `day`. Empty hour buckets are not printed.

`--group-by=session`:

```text
group by day, session_id, provider, model
```

The `session id` column is inserted after `day`.

## Metrics

Token columns are sums over matching `oc_token_events` rows:

```text
input
output
reasoning
cache read
cache write
total
```

`total` means OpenCode `tokens.total` when present in the plugin, otherwise input + output + reasoning + cache read + cache write.

TPS columns come from `oc_tps_samples`:

```text
tps avg: sum(total_tokens) / (sum(duration_ms) / 1000)
tps mean: arithmetic mean of row-level tokens_per_second
tps median: median of row-level tokens_per_second
```

TPS `total_tokens` is output + reasoning and is separate from token-event `total_tokens`.

## Sorting

Default:

```text
day desc, provider asc, model asc
```

Hour grouping:

```text
day desc, hour desc, provider asc, model asc
```

Session grouping:

```text
day desc, session id asc, provider asc, model asc
```

## Rendering

`renderTable` builds rows as strings, calculates widths, left-aligns text columns, and right-aligns numeric TPS/token columns.

Numeric columns start at:

```text
default: column index 3
group-by hour/session: column index 4
```

### Compact Token Units

Token columns render in compact units. Raw integer values remain in SQLite; formatting is display-only.

Rules:

```text
value == 0         -> blank
value < 1,000      -> raw integer
value < 1,000,000  -> <value/1000>K
value >= 1,000,000 -> <value/1,000,000>M
```

Integer-only, truncated toward zero. Decimal thresholds, not binary. Applies to all six token columns: input, output, reasoning, cache read, cache write, total.

Examples:

```text
0       -> ""
999     -> "999"
1000    -> "1K"
687979  -> "687K"
999999  -> "999K"
1000000 -> "1M"
6835769 -> "6M"
```

## Extension Notes

If plugin schema changes, update:

```text
plugin SQLite schema and migrations/reset logic
plugin OpenCode event handling
plugin row shape and token semantics
CLI sample struct
CLI querySamples SELECT list and Scan order
CLI aggregation and rendering
tests that create oc_token_events and oc_tps_samples
README.md
docs/how-it-works.md
```

If adding more grouping modes, update:

```text
groupByMode constants
groupByFlag.Set
aggregateSamples switch
renderTable header/row building
sorting and table alignment tests
README.md
docs/how-it-works.md
```

## Verification

Run plugin smoke check:

```sh
bun build plugins/oc-tokeninspector.tsx --target=bun --outfile=/tmp/oc-tokeninspector-check.js --external "solid-js" --external "@opentui/solid" --external "@opentui/solid/jsx-dev-runtime"
```

Run CLI tests and build from `cli/`:

```sh
go test ./...
go build -o tokeninspector-cli .
```

Smoke check against the real DB from `cli/`:

```sh
./tokeninspector-cli table --db-path ~/.local/state/opencode/oc-tps.sqlite --day
./tokeninspector-cli table --db-path ~/.local/state/opencode/oc-tps.sqlite --day --group-by=hour
./tokeninspector-cli table --db-path ~/.local/state/opencode/oc-tps.sqlite --day --group-by=session
```
