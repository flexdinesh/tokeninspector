# Neutralize TokenInspector DB path and env vars

## Summary

Rename the default SQLite DB from the OpenCode-scoped path/name:

```text
~/.local/state/opencode/oc-tps.sqlite
```

to the TokenInspector-owned path/name:

```text
~/.local/state/tokeninspector/tokeninspector.sqlite
```

Also replace harness-specific env vars:

```text
OC_TOKENINSPECTOR_*
PI_TOKENINSPECTOR_*
```

with shared neutral vars:

```text
TOKENINSPECTOR_*
```

No backwards-compatible fallback for old env vars. This is not a schema change.

## Key implementation changes

1. Use these defaults consistently across OpenCode server plugin, OpenCode TUI plugin, and Pi extension:
   - DB filename: `tokeninspector.sqlite`
   - State dir with `XDG_STATE_HOME`: `$XDG_STATE_HOME/tokeninspector`
   - State dir with `HOME`: `$HOME/.local/state/tokeninspector`
   - Fallback state dir: `$PWD/.tokeninspector-state`
   - Default DB path: `~/.local/state/tokeninspector/tokeninspector.sqlite`
   - Absolute configured DB paths remain unchanged.
   - Relative configured DB paths resolve under the TokenInspector state dir.
2. In `plugins/opencode-server/oc-tokeninspector-server.ts`:
   - Change `DEFAULT_DB_NAME` to `tokeninspector.sqlite`.
   - Change `defaultStatePath()` from OpenCode paths to TokenInspector paths.
   - Replace `OC_TOKENINSPECTOR_DB_PATH` and `OC_TOKENINSPECTOR_RETENTION_DAYS` with `TOKENINSPECTOR_DB_PATH` and `TOKENINSPECTOR_RETENTION_DAYS`.
   - Do not keep fallback support for old env vars.
3. In `plugins/pi/index.ts`:
   - Change `DEFAULT_DB_NAME` to `tokeninspector.sqlite`.
   - Change `defaultStatePath()` from OpenCode paths to TokenInspector paths.
   - Replace `PI_TOKENINSPECTOR_DB_PATH`, `OC_TOKENINSPECTOR_DB_PATH`, and `PI_TOKENINSPECTOR_RETENTION_DAYS` with `TOKENINSPECTOR_DB_PATH` and `TOKENINSPECTOR_RETENTION_DAYS`.
   - Do not keep fallback support for old env vars.
4. In `plugins/opencode-tui/oc-tokeninspector.tsx`:
   - Change `DEFAULT_DB_NAME` to `tokeninspector.sqlite`.
   - Change default and relative configured `dbPath` values to use the neutral TokenInspector state directory instead of OpenCode TUI state.
   - Do not add env var support to the TUI plugin in this task.
5. In CLI tests:
   - Update temporary DB filenames in `cli/internal/db/db_test.go` and `cli/cmd/tokeninspector-cli/main_test.go` to `tokeninspector.sqlite`.
   - Do not add a CLI default DB path; the CLI still requires `--db-path`.
6. Update current docs:
   - `README.md`
   - `cli/README.md`
   - `docs/design.md`
   - `AGENTS.md`
   - Replace current path examples with `~/.local/state/tokeninspector/tokeninspector.sqlite`.
   - Document `TOKENINSPECTOR_DB_PATH` and `TOKENINSPECTOR_RETENTION_DAYS`.
   - Remove current references to old env vars from current docs/code.
   - Do not edit historical planning records under `.ai/plans/` except this new saved plan.

## Tests / verification

Plugin smoke builds:

```sh
bun build plugins/opencode-tui/oc-tokeninspector.tsx --target=bun --outfile=/tmp/oc-tokeninspector-check.js --external "solid-js" --external "@opentui/solid" --external "@opentui/solid/jsx-dev-runtime"
bun build plugins/shared/oc-tokeninspector-writer.ts --target=bun --outfile=/tmp/oc-tokeninspector-writer-check.js
bun build plugins/opencode-server/oc-tokeninspector-server.ts --target=bun --outfile=/tmp/oc-tokeninspector-server-check.js --external "@opencode-ai/plugin"
```

CLI verification:

```sh
cd cli
go test ./...
go build -o tokeninspector-cli ./cmd/tokeninspector-cli
```

Search checks:

```sh
grep -R "oc-tps.sqlite" README.md cli/README.md docs/design.md plugins cli AGENTS.md
grep -R ".local/state/opencode" README.md cli/README.md docs/design.md plugins cli AGENTS.md
grep -R "OC_TOKENINSPECTOR\|PI_TOKENINSPECTOR" README.md cli/README.md docs/design.md plugins cli AGENTS.md
```

Expected: no matches in current code/docs.

## Decisions made by user

- Rename DB filename to `tokeninspector.sqlite`.
- Move default DB state directory to `~/.local/state/tokeninspector/`.
- Do not use `opencode` in the default path for TokenInspector-owned data.
- Relative configured paths should resolve under the TokenInspector state dir.
- Replace old harness-scoped env vars with neutral ones: `TOKENINSPECTOR_DB_PATH`, `TOKENINSPECTOR_RETENTION_DAYS`.
- Remove support for old env vars completely.
- The user will manually rename/move the existing local DB file and sidecar files if present.
- No schema migration or data migration code is needed.
- Update docs wherever needed, including `AGENTS.md`.
- Do not edit historical `.ai/plans/*` records except this new saved plan.

## Tradeoffs and risks

- Old environment variables will stop working. This is intentional to avoid stale harness-scoped config influencing neutral TokenInspector behavior.
- The plugins will not auto-discover the old DB at `~/.local/state/opencode/oc-tps.sqlite`; they will use/create `~/.local/state/tokeninspector/tokeninspector.sqlite`.
- Missing one component could split reads/writes across DB files, so server, TUI, Pi, CLI tests, and docs must be updated together.
- SQLite WAL mode may create `oc-tps.sqlite-wal` and `oc-tps.sqlite-shm`; stop writers first and rename sidecars when manually moving the DB.
- Table names such as `oc_tps_samples` are schema names and must remain unchanged.

## Execution guidance

If execution deviates from this plan, update this saved plan to reflect the latest approved approach and surface the deviation to the user before continuing.
