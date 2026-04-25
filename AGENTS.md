# tokeninspector

This repo contains the OpenCode token usage project:

- `plugins/oc-tokeninspector.tsx`: OpenCode TUI plugin that writes local token/TPS data.
- `cli/`: Go CLI that queries the SQLite DB written by the plugin.
- `docs/how-it-works.md`: shared architecture and behavior doc for both sides.

The plugin and CLI are one project. When changing storage, schema, event handling, SQL, aggregation, metric names, table columns, docs, or tests, usually update both sides in the same task.

## Why This Exists

Track OpenCode token usage locally over time, without relying on vendor dashboards.

North star: a session-centric, queryable token time series. Every durable token row must relate to `session_id`; no token data should exist without it.

TPS is also a first-class project metric. Do not remove persisted TPS columns/tables or `tps avg`, `tps mean`, and `tps median` when changing token schema.

## Design Notes

- SQLite is the local source of truth.
- Default DB lives under OpenCode state path: `~/.local/state/opencode/oc-tps.sqlite`.
- Plugin DB writes must stay lightweight; the TUI should never feel blocked by analytics.
- CLI reads SQLite directly in read-only mode.
- CLI data is grouped by local day, hour, or session.
- `session_id` is first-class and required for every durable row.
- Prefer real OpenCode token data over estimated stream deltas.
- `message.part.delta` is only for live UI estimates.
- Durable token accounting should come from OpenCode events with real token fields.
- Preferred durable model: `oc_token_events`, one time-series row per token event.
- Required throughput model: `oc_tps_samples`, one row per completed assistant message.
- Preferred grain: one row per real token event, normally OpenCode `step-finish`.
- True time-series source: `message.part.updated` where `part.type == "step-finish"`.
- Resilience fallback: if a session/message ends without step rows, write a completed assistant message fallback row.
- Missing provider/model should not drop token data; store, render, and query it as `unknown`.
- Token columns should be shown by default.
- `total_tokens` means OpenCode `tokens.total` when present, otherwise input + output + reasoning + cache read + cache write.
- TPS metrics come from message-level throughput rows and must not be inferred from token-event totals.
- TPS `total_tokens` is separate: output + reasoning only, used as throughput numerator.
- Avoid silently changing token semantics; tests should make the meaning obvious.

## Schema Contract

- `schema/schema.sql` is the single source of truth for table and column definitions.
- Plugin writers auto-migrate DB using `plugins/schema-migrate.ts`, which reads `schema/schema.sql` at init time.
- Cross-language contract is validated by `scripts/check-schema.ts` (parses SQL, Go constants, and TS types).
- Run schema validation before any schema-related commit:

```sh
bun run scripts/check-schema.ts
```

## Change Checklist

- If schema changes, update `schema/schema.sql` and run `bun run scripts/check-schema.ts`.
- If schema changes, update `cli/internal/db/schema.go` constants so Go tests pass.
- If plugin row shape or token semantics change, update CLI query structs, SQL, aggregation, rendering, tests, README, and `docs/how-it-works.md`.
- If CLI query columns change, update `sample`, `querySamples`, scan order, aggregation, rendering, tests, README, and `docs/how-it-works.md`.
- If grouping changes, update sorting and table alignment tests.
- If event source changes, update plugin event handling, CLI expectations, and `docs/how-it-works.md`.
- If token semantics change, update tests with semantic examples.
- Run plugin smoke build when touching TypeScript:

```sh
bun build plugins/oc-tokeninspector.tsx --target=bun --outfile=/tmp/oc-tokeninspector-check.js --external "solid-js" --external "@opentui/solid" --external "@opentui/solid/jsx-dev-runtime"
```

- Run CLI verification when touching Go code:

```sh
go test ./...
go build -o tokeninspector-cli .
```
