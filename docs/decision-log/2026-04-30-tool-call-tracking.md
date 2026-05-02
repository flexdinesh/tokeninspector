---
title: Tool call tracking schema and CLI views
description: Add OpenCode and Pi tool-call lifecycle tracking with CLI aggregate and breakdown views.
date: 2026-04-30
slug: tool-call-tracking
status: implemented
tags:
  - schema
  - tool-calls
  - cli
  - opencode
  - pi
related_paths:
  - schema/schema.sql
  - plugins/opencode-server/oc-tokeninsights-server.ts
  - plugins/pi/index.ts
  - cli/internal/db/aggregate.go
  - cli/internal/cli/render.go
  - docs/spec/tool-runtime-duration-metrics.md
---

## Why

Users need to observe tool-call activity from both OpenCode and Pi alongside existing token, TPS, and request metrics. The CLI should expose both high-level tool-call counts and per-tool breakdowns while preserving the existing session/day/hour grouping modes.

## What

- Schema v2 adds `oc_tool_calls` and `pi_tool_calls`.
- Tool calls are stored as lifecycle rows with `status` values: `started`, `completed`, and `error`.
- The CLI adds two views:
  - `tool calls`: total started calls and errors per existing group.
  - `tool breakdown`: the same metrics additionally grouped by `tool_name`.
- Runtime duration metrics are intentionally deferred. Future duration storage and rendering are planned in `docs/spec/tool-runtime-duration-metrics.md`.

## How

- OpenCode records tool lifecycle via `tool.execute.before` and `tool.execute.after` in the server plugin.
- Pi records tool lifecycle via `tool_execution_start` and `tool_execution_end` in the extension.
- `tool calls` count is derived from `status = 'started'`.
- `errors` count is derived from `status = 'error'`.
- Existing token/TPS/request rows in v1 databases are preserved because the migration is additive. New tool-call tables start empty; historical tool calls are not backfilled.
- The CLI remains read-only. Users must run the upgraded OpenCode plugin or Pi extension once to migrate an old DB from schema v1 to v2 before using the new CLI.

## Gotchas

- Old tool-call history is unavailable unless a future importer can reconstruct it from session logs.
- If only one harness migrates/writes a DB, the CLI must tolerate the other harness's tool-call table being absent.
- The documented Go build target is `./cmd/tokeninsights-cli`, not the `cli/` module root.
