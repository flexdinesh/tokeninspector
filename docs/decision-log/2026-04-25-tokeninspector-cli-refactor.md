---
title: Maintainable Go+SQLite patterns for tokeninspector-cli
description: Refactored flat main.go into internal/db + internal/cli packages, added schema version contract, and moved aggregation into SQL.
date: 2026-04-25
slug: tokeninspector-cli-refactor
status: implemented
tags:
  - go
  - sqlite
  - schema-contract
  - sql-aggregation
related_paths:
  - cli/cmd/tokeninspector-cli/main.go
  - cli/internal/db/schema.go
  - cli/internal/db/open.go
  - cli/internal/db/aggregate.go
  - cli/internal/cli/render.go
  - plugins/oc-tokeninspector-writer.ts
  - plugins/oc-tokeninspector-server.ts
  - docs/how-it-works.md
---

## Why

The CLI was a single 1,068-line `main.go` with three near-duplicate aggregation functions, in-memory filtering after fetching all rows, hard-coded table names, and no runtime schema-contract enforcement. Drift between the plugin (writer) and CLI (reader) was already biting: tests used `oc_tps_samples` while code queried `oc_token_events`. Goal was to align with production Go+SQLite patterns (atuin, sq, litestream) without over-engineering a single-command CLI.

## What

- Flat `main.go` → `cmd/tokeninspector-cli/main.go` + `internal/db/` + `internal/cli/`
- Added `PRAGMA user_version = 1` to both TypeScript writers (TUI plugin + server plugin)
- CLI checks `PRAGMA user_version` on open and hard-fails on mismatch
- All filter criteria pushed into SQL via bound `IN (?,?,?)` expansion
- Three Go aggregation funcs → single `Aggregate()` with SQL-side `GROUP BY` per `GroupBy` enum
- Renderer switched from magic column-start index to explicit `[]column` table definition
- Adopted golden-file tests for rendered ASCII table output

## How

- `internal/db/schema.go`: single source of truth for table/column names + `SupportedSchemaVersion`
- `internal/db/open.go`: DSN with `mode=ro`, `busy_timeout(5000)`, `query_only(true)`, `foreign_keys(on)`; `PingContext`; version check
- `internal/db/aggregate.go`: `GroupByDay` / `GroupByDayHour` / `GroupByDaySession`; SQL `SUM` for token events; CTE with `ROW_NUMBER()` for TPS median; `GROUP_CONCAT(DISTINCT ...)` for session thinking levels; merge three result sets by group key in Go
- `internal/cli/render.go`: `columnsForMode(g)` returns header + numeric flag; `renderTable` maps `renderRow` fields via `column.field`
- Golden files under `internal/cli/testdata/` for daily/hourly/session output; `UPDATE_GOLDEN=1` to regenerate

## Tradeoffs

| Decision | Alternative | Chosen because |
|---|---|---|
| Proper package split (`internal/`) | Keep flat, extract two files only | Aggregation dedup + future second command (`json`, `csv`) justifies the boundary |
| `PRAGMA user_version` (Option A) | `sqlite_master` DDL shape check (Option B) | One integer, zero string parsing; we own both writer and reader sides |
| SQL-side aggregation | Go-side parameterized aggregator | Deletes 3 duplicate funcs; uses indexes; scales to month windows |
| Golden-file tests | Assertion-style only | Output diffs become reviewable in PRs; standard cost when format changes |
| Break flag compatibility | Strictly additive | No users yet; better to get naming right now |
| Strict `user_version` hard-fail | Always fail on version 0 | Pragmatically allow version 0 if required tables exist, to avoid bricking existing data before plugins are rebuilt |

## Risks

- `PRAGMA user_version` requires both TypeScript writers to set it. A writer deployed without the pragma will fail against the new CLI. Mitigation: writers updated in same changeset; CLI also accepts `user_version == 0` if required tables exist (legacy DB fallback).
- SQL-side `date(recorded_at_ms/1000, 'unixepoch', 'localtime')` must match Go's `time.Local` formatting across DST boundaries. No observed mismatch in tests; monitor if users report.
- Window-function CTE for TPS median is SQLite 3.25+. `modernc.org/sqlite v1.49.1` bundles a recent SQLite; safe.

## Assumptions

- Reader never writes; `query_only(true)` is acceptable. If tests ever need write helpers, they bypass `db.Open()` and use raw `sql.Open("sqlite", ...)`.
- `user_version` increments only on schema-breaking changes (new required columns, dropped tables). Adding optional columns or indexes does not need a bump.
