# Neutralize TokenInsights DB path and env vars

## Summary

Rename the default SQLite DB from the OpenCode-scoped path/name:

```text
~/.local/state/opencode/oc-tps.sqlite
```

to the TokenInsights-owned path/name:

```text
~/.local/state/tokeninsights/tokeninsights.sqlite
```

Also replace harness-specific env vars:

```text
OC_TOKENINSIGHTS_*
PI_TOKENINSIGHTS_*
```

with shared neutral vars:

```text
TOKENINSIGHTS_*
```

No backwards-compatible fallback for old env vars. This is not a schema change.

## Key implementation changes

1. Use these defaults consistently across OpenCode server plugin, OpenCode TUI plugin, and Pi extension:
   - DB filename: `tokeninsights.sqlite`
   - State dir with `XDG_STATE_HOME`: `$XDG_STATE_HOME/tokeninsights`
   - State dir with `HOME`: `$HOME/.local/state/tokeninsights`
   - Fallback state dir: `$PWD/.tokeninsights-state`
   - Default DB path: `~/.local/state/tokeninsights/tokeninsights.sqlite`
   - Absolute configured DB paths remain unchanged.
   - Relative configured DB paths resolve under the TokenInsights state dir.
2. In `plugins/opencode-server/oc-tokeninsights-server.ts`:
   - Change `DEFAULT_DB_NAME` to `tokeninsights.sqlite`.
   - Change `defaultStatePath()` from OpenCode paths to TokenInsights paths.
   - Replace `OC_TOKENINSIGHTS_DB_PATH` and `OC_TOKENINSIGHTS_RETENTION_DAYS` with `TOKENINSIGHTS_DB_PATH` and `TOKENINSIGHTS_RETENTION_DAYS`.
   - Do not keep fallback support for old env vars.
3. In `plugins/pi/index.ts`:
   - Change `DEFAULT_DB_NAME` to `tokeninsights.sqlite`.
   - Change `defaultStatePath()` from OpenCode paths to TokenInsights paths.
   - Replace `PI_TOKENINSIGHTS_DB_PATH`, `OC_TOKENINSIGHTS_DB_PATH`, and `PI_TOKENINSIGHTS_RETENTION_DAYS` with `TOKENINSIGHTS_DB_PATH` and `TOKENINSIGHTS_RETENTION_DAYS`.
   - Do not keep fallback support for old env vars.
4. In `plugins/opencode-tui/oc-tokeninsights.tsx`:
   - Change `DEFAULT_DB_NAME` to `tokeninsights.sqlite`.
   - Change default and relative configured `dbPath` values to use the neutral TokenInsights state directory instead of OpenCode TUI state.
   - Do not add env var support to the TUI plugin in this task.
5. In CLI tests:
   - Update temporary DB filenames in `cli/internal/db/db_test.go` and `cli/cmd/tokeninsights-cli/main_test.go` to `tokeninsights.sqlite`.
   - Do not add a CLI default DB path; the CLI still requires `--db-path`.
6. Update current docs:
   - `README.md`
   - `cli/README.md`
   - `docs/design.md`
   - `AGENTS.md`
   - Replace current path examples with `~/.local/state/tokeninsights/tokeninsights.sqlite`.
   - Document `TOKENINSIGHTS_DB_PATH` and `TOKENINSIGHTS_RETENTION_DAYS`.
   - Remove current references to old env vars from current docs/code.
   - Do not edit historical planning records under `.ai/plans/` except this new saved plan.

## Tests / verification

Plugin smoke builds:

```sh
bun build plugins/opencode-tui/oc-tokeninsights.tsx --target=bun --outfile=/tmp/oc-tokeninsights-check.js --external "solid-js" --external "@opentui/solid" --external "@opentui/solid/jsx-dev-runtime"
bun build plugins/shared/oc-tokeninsights-writer.ts --target=bun --outfile=/tmp/oc-tokeninsights-writer-check.js
bun build plugins/opencode-server/oc-tokeninsights-server.ts --target=bun --outfile=/tmp/oc-tokeninsights-server-check.js --external "@opencode-ai/plugin"
```

CLI verification:

```sh
cd cli
go test ./...
go build -o tokeninsights-cli ./cmd/tokeninsights-cli
```

Search checks:

```sh
grep -R "oc-tps.sqlite" README.md cli/README.md docs/design.md plugins cli AGENTS.md
grep -R ".local/state/opencode" README.md cli/README.md docs/design.md plugins cli AGENTS.md
grep -R "OC_TOKENINSIGHTS\|PI_TOKENINSIGHTS" README.md cli/README.md docs/design.md plugins cli AGENTS.md
```

Expected: no matches in current code/docs.

## Decisions made by user

- Rename DB filename to `tokeninsights.sqlite`.
- Move default DB state directory to `~/.local/state/tokeninsights/`.
- Do not use `opencode` in the default path for TokenInsights-owned data.
- Relative configured paths should resolve under the TokenInsights state dir.
- Replace old harness-scoped env vars with neutral ones: `TOKENINSIGHTS_DB_PATH`, `TOKENINSIGHTS_RETENTION_DAYS`.
- Remove support for old env vars completely.
- The user will manually rename/move the existing local DB file and sidecar files if present.
- No schema migration or data migration code is needed.
- Update docs wherever needed, including `AGENTS.md`.
- Do not edit historical `.ai/plans/*` records except this new saved plan.

## Tradeoffs and risks

- Old environment variables will stop working. This is intentional to avoid stale harness-scoped config influencing neutral TokenInsights behavior.
- The plugins will not auto-discover the old DB at `~/.local/state/opencode/oc-tps.sqlite`; they will use/create `~/.local/state/tokeninsights/tokeninsights.sqlite`.
- Missing one component could split reads/writes across DB files, so server, TUI, Pi, CLI tests, and docs must be updated together.
- SQLite WAL mode may create `oc-tps.sqlite-wal` and `oc-tps.sqlite-shm`; stop writers first and rename sidecars when manually moving the DB.
- Table names such as `oc_tps_samples` are schema names and must remain unchanged.

## Execution guidance

If execution deviates from this plan, update this saved plan to reflect the latest approved approach and surface the deviation to the user before continuing.
