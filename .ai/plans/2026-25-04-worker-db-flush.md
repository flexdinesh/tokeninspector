# Worker DB Flush Plan

## Summary

Move SQLite writes out of TUI event handlers into a dedicated worker. The TUI keeps lightweight in-memory state, batches writes, and tolerates losing the last queued batch on crash.

## Key Implementation Changes

- Add worker-backed storage for the TUI plugin.
- Keep `message.part.delta` unchanged: live UI only, no DB.
- Change `message.part.updated` and `message.updated` to enqueue write jobs only.
- Move SQLite operations into the worker:
  - insert token rows
  - insert TPS rows
  - metadata backfill updates
  - fallback replacement logic
  - retention prune
- Batch queue flush:
  - scheduled flush every `1000ms`
  - immediate flush request on `session.idle`
  - immediate flush request on `session.deleted`
  - best-effort final flush on dispose
- Daily retention prune:
  - worker tracks last prune day in memory
  - prune at worker startup if needed
  - prune at most once per local day after that
- Failure handling:
  - worker reports errors back to TUI
  - TUI shows existing one-time toast
  - queued rows may be dropped if worker dies or process crashes before flush; accepted

## Tests Or Verification

- Run TUI smoke build:

```sh
bun build plugins/oc-tokeninsights.tsx --target=bun --outfile=/tmp/oc-tokeninsights-check.js --external "solid-js" --external "@opentui/solid" --external "@opentui/solid/jsx-dev-runtime"
```

- If worker is not covered by that build, run:

```sh
bun build plugins/oc-tokeninsights-writer.ts --target=bun --outfile=/tmp/oc-tokeninsights-writer-check.js
```

- Run CLI verification only if schema/query assumptions change:

```sh
go test ./...
go build -o tokeninsights-cli .
```

## Decisions Made

- Use worker, not in-process async queue.
- Losing last <1s queued batch on crash is acceptable.
- Retention pruning cadence: daily.

## Tradeoffs And Risks

- Worker path/bundling behavior may be the main implementation risk.
- Dispose flush is best-effort; hard process exit can still lose queued rows.
- More moving parts than in-process queue.

## Remaining Questions

None.

## Execution Guidance

If execution deviates, update this plan file to reflect the latest approved plan and surface the deviation to the user.
