---
title: Neutral TokenInspector DB path and env vars
description: Move default SQLite storage and writer env vars from harness-scoped names to TokenInspector-owned names.
date: 2026-05-01
slug: neutral-tokeninspector-db-path
status: implemented
tags:
  - sqlite
  - storage
  - config
  - opencode
  - pi
related_paths:
  - plugins/opencode-server/oc-tokeninspector-server.ts
  - plugins/opencode-tui/oc-tokeninspector.tsx
  - plugins/pi/index.ts
  - README.md
  - cli/README.md
  - docs/design.md
  - AGENTS.md
---

## Why

TokenInspector started with an OpenCode-specific SQLite filename and state path, but now stores data from both OpenCode and Pi. Keeping the default path under `opencode` and the database filename as `oc-tps.sqlite` made the storage contract look harness-specific even though the database is TokenInspector-owned.

## What

Default storage is now TokenInspector-scoped:

```text
~/.local/state/tokeninspector/tokeninspector.sqlite
```

With XDG and fallback resolution:

```text
$XDG_STATE_HOME/tokeninspector/tokeninspector.sqlite
$PWD/.tokeninspector-state/tokeninspector.sqlite
```

Writer configuration env vars are now neutral:

```text
TOKENINSPECTOR_DB_PATH
TOKENINSPECTOR_RETENTION_DAYS
```

Old harness-scoped env vars are intentionally unsupported:

```text
OC_TOKENINSPECTOR_DB_PATH
OC_TOKENINSPECTOR_RETENTION_DAYS
PI_TOKENINSPECTOR_DB_PATH
PI_TOKENINSPECTOR_RETENTION_DAYS
```

Relative configured DB paths resolve under the TokenInspector state directory. Absolute configured DB paths remain unchanged.

## How

OpenCode server, OpenCode TUI, and Pi defaults all resolve to the same TokenInspector-owned state directory and `tokeninspector.sqlite` filename. The CLI still requires an explicit `--db-path`; docs and tests use the new default path/name for consistency.

## Tradeoffs

This intentionally breaks old local env var configuration instead of preserving compatibility aliases. The project is currently private-use, and removing aliases avoids stale `OC_*` or `PI_*` env vars silently redirecting writes to the old location.

## Gotchas

This is not a schema change. Table families such as `oc_*` and `pi_*` remain unchanged because they identify data source families inside the shared TokenInspector database.
