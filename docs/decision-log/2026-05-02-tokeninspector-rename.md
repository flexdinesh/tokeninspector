---
title: Rename project from tokeninspector to tokeninsights
description: Full branding and naming migration across all code, files, environment variables, and documentation.
date: 2026-05-02
slug: tokeninspector-rename
status: implemented
tags:
  - naming
  - branding
  - breaking-change
  - migration
related_paths:
  - docs/rename-migration.md
  - README.md
  - cli/go.mod
  - plugins/opencode-tui/oc-tokeninsights.tsx
  - plugins/opencode-server/oc-tokeninsights-server.ts
  - plugins/pi/package.json
---

## Why

The project name `tokeninspector` was generic and clunky. `tokeninsights` better reflects what the tool actually provides — not just inspection, but analytics, aggregation, and cross-harness insights over time.

## What

Every occurrence of `tokeninspector` was renamed to `tokeninsights` across the entire repo:

- **Go module**: `tokeninspector-cli` → `tokeninsights-cli`
- **CLI binary**: `tokeninspector-cli` → `tokeninsights-cli`
- **OpenCode TUI plugin ID**: `oc-tokeninspector` → `oc-tokeninsights`
- **OpenCode server plugin export**: `OcTokenInspectorServer` → `OcTokenInsightsServer`
- **Pi extension package name**: `pi-tokeninspector` → `pi-tokeninsights`
- **Environment variables**: `TOKENINSPECTOR_DB_PATH` → `TOKENINSIGHTS_DB_PATH`, `TOKENINSPECTOR_RETENTION_DAYS` → `TOKENINSIGHTS_RETENTION_DAYS`
- **Default DB file**: `tokeninspector.sqlite` → `tokeninsights.sqlite`
- **Default state directory**: `~/.local/state/tokeninspector/` → `~/.local/state/tokeninsights/`
- **TUI banner title**: `Token Inspector` → `Token Insights`
- **All documentation**: README, CLI README, design doc, AGENTS.md, proposal, all decision logs, all plans
- **Historical file names**: decision logs and plans with `tokeninspector` in their filenames were renamed too

**Not changed** (intentionally):
- DB table families (`oc_*`, `pi_*`) — they identify data source, not project name
- Schema version (`2`) — no schema migration needed
- SQL column names — all remain the same

## How

1. Edited all file contents first (env vars, imports, exports, IDs, titles, paths)
2. Renamed source files (5 plugin/CLI files)
3. Renamed historical docs (4 decision log/plan files)
4. Rebuilt CLI binary (`go build -o tokeninsights-cli ./cmd/tokeninsights-cli`)
5. Ran all validations: `go test ./...`, schema contract check, plugin smoke builds
6. Wrote `docs/rename-migration.md` with user-facing migration instructions

## Tradeoffs

- **Severe breaking change** for existing users. DB file, state directory, env vars, plugin IDs, and binary name all changed simultaneously. This was accepted because the project is currently private-use and the rename is one-time.
- **Historical docs renamed** even though they're archival records. This keeps the repo grep-clean and consistent, but makes the historical slugs/filenames anachronistic. Accepted as a cosmetic tradeoff.

## Gotchas

- Users must **manually migrate their DB file** from the old default path to the new one, or set `TOKENINSIGHTS_DB_PATH` to the old location.
- OpenCode plugin symlinks in `~/.config/opencode/plugins/` must be recreated pointing to the renamed files.
- Pi extension symlink in `~/.pi/agent/extensions/` must be recreated as `pi-tokeninsights`.
- Git remote URL (`github.com:flexdinesh/tokeninspector`) must be updated after the GitHub repo is renamed.
- `scripts/check-schema.ts` previously used hardcoded absolute paths (`/Users/dineshpandiyan/workspace/...`). Fixed as part of this work to use `new URL(..., import.meta.url).pathname`.
