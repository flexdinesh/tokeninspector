---
title: Neutral TokenInsights DB path and env vars
description: Move default SQLite storage and writer env vars from harness-scoped names to TokenInsights-owned names.
date: 2026-05-01
slug: neutral-tokeninsights-db-path
status: implemented
tags:
  - sqlite
  - storage
  - config
  - opencode
  - pi
related_paths:
  - plugins/opencode-server/oc-tokeninsights-server.ts
  - plugins/opencode-tui/oc-tokeninsights.tsx
  - plugins/pi/index.ts
  - README.md
  - cli/README.md
  - docs/design.md
  - AGENTS.md
---

## Why

TokenInsights started with an OpenCode-specific SQLite filename and state path, but now stores data from both OpenCode and Pi. Keeping the default path under `opencode` and the database filename as `oc-tps.sqlite` made the storage contract look harness-specific even though the database is TokenInsights-owned.

## What

Default storage is now TokenInsights-scoped:

```text
~/.local/state/tokeninsights/tokeninsights.sqlite
```

With XDG and fallback resolution:

```text
$XDG_STATE_HOME/tokeninsights/tokeninsights.sqlite
$PWD/.tokeninsights-state/tokeninsights.sqlite
```

Writer configuration env vars are now neutral:

```text
TOKENINSIGHTS_DB_PATH
TOKENINSIGHTS_RETENTION_DAYS
```

Old harness-scoped env vars are intentionally unsupported:

```text
OC_TOKENINSIGHTS_DB_PATH
OC_TOKENINSIGHTS_RETENTION_DAYS
PI_TOKENINSIGHTS_DB_PATH
PI_TOKENINSIGHTS_RETENTION_DAYS
```

Relative configured DB paths resolve under the TokenInsights state directory. Absolute configured DB paths remain unchanged.

## How

OpenCode server, OpenCode TUI, and Pi defaults all resolve to the same TokenInsights-owned state directory and `tokeninsights.sqlite` filename. The CLI still requires an explicit `--db-path`; docs and tests use the new default path/name for consistency.

## Tradeoffs

This intentionally breaks old local env var configuration instead of preserving compatibility aliases. The project is currently private-use, and removing aliases avoids stale `OC_*` or `PI_*` env vars silently redirecting writes to the old location.

## Gotchas

This is not a schema change. Table families such as `oc_*` and `pi_*` remain unchanged because they identify data source families inside the shared TokenInsights database.
