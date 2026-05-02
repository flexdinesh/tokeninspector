# Tool Runtime Duration Metrics — Future Design

## Summary

Token Insights now stores tool-call lifecycle rows (`started`, `completed`, `error`) for OpenCode and Pi. The current CLI intentionally exposes only started-call counts and error counts. This spec outlines a future schema and aggregation path for tool runtime duration metrics without changing the current implementation scope.

## Goals

- Measure tool runtime per harness (`oc`, `pi`), session, provider, model, and tool name.
- Preserve existing started/completed/error counts.
- Support `duration avg`, `duration mean`, and `duration median` in future CLI views.
- Keep plugin writes lightweight and resilient to missing completion events.

## Proposed Data Semantics

A tool runtime sample should represent one completed tool execution attempt:

- Start time: observed at `tool.execute.before` in OpenCode or `tool_execution_start` in Pi.
- End time: observed at `tool.execute.after` in OpenCode or `tool_execution_end` in Pi.
- Status: `completed` or `error`.
- Duration: `end_at_ms - start_at_ms`, clamped to at least `1` ms when both timestamps exist.

Started rows without a matching terminal row should continue to count as attempted tool calls but should not contribute to duration metrics.

## Future Schema Option

Add dedicated runtime sample tables instead of overloading lifecycle rows:

```sql
CREATE TABLE IF NOT EXISTS oc_tool_runtime_samples (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  recorded_at TEXT NOT NULL,
  recorded_at_ms INTEGER NOT NULL,
  session_id TEXT NOT NULL,
  message_id TEXT NOT NULL,
  tool_call_id TEXT NOT NULL,
  tool_name TEXT NOT NULL DEFAULT 'unknown',
  provider TEXT NOT NULL DEFAULT 'unknown',
  model TEXT NOT NULL DEFAULT 'unknown',
  status TEXT NOT NULL CHECK (status IN ('completed', 'error')),
  duration_ms INTEGER NOT NULL CHECK (duration_ms > 0),
  UNIQUE(session_id, tool_call_id)
);
```

Mirror the same shape for `pi_tool_runtime_samples`.

## Why Dedicated Sample Tables

- Lifecycle rows remain an append-only event log for counts.
- Runtime samples have one row per terminal execution and are easy to aggregate.
- Missing terminal events do not require updating `started` rows.
- Median duration can use the same window-CTE pattern as TPS median.

## CLI Aggregation Plan

For total tool runtime metrics:

- `SUM(duration_ms) / COUNT(*)` for weighted average by sample count.
- `AVG(duration_ms)` for mean.
- Median via `ROW_NUMBER()` / `COUNT()` window CTE.
- Group by day/hour/session + provider + model + harness.

For tool breakdown:

- Same metrics, additionally grouped by `tool_name`.

Potential future columns:

```text
tool calls | errors | duration avg | duration mean | duration median
```

and for breakdown:

```text
tool | tool calls | errors | duration avg | duration mean | duration median
```

## Open Questions

- Should blocked tool calls get a separate terminal status and duration from preflight start to block?
- Should long-running tools that are cancelled be represented as `error`, `cancelled`, or both?
- Should durations include permission wait time, or only tool execution time after permission is granted?
