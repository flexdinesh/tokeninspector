# tokeninsights

Local token usage tools for OpenCode and Pi. The OpenCode server plugin and Pi extension write token/TPS/request/tool-call data to SQLite; the OpenCode TUI plugin queries the DB for live display; the Go CLI reads aggregate tables.

See [`docs/design.md`](docs/design.md) for full architecture, schema contract, event flow, and invariants.

## Code Organization

- `plugins/opencode-tui/` — OpenCode TUI plugin
- `plugins/opencode-server/` — OpenCode server plugin
- `plugins/shared/` — Shared types, schema migration, writer client, writer worker
- `plugins/pi/` — Pi extension
- `cli/` — Go CLI (`tokeninsights-cli`) that queries the SQLite DB
- `schema/schema.sql` — single source of truth for SQLite schema
- `scripts/check-schema.ts` — cross-language schema contract validator

## Development

### When to run what

| You changed | Run |
|-------------|-----|
| `schema/schema.sql` | `npm run check-schema` |
| Any `.ts` in `plugins/` | `npm run smoke:plugins` |
| Any `.go` in `cli/` | `npm run test:go && npm run build:cli` |
| Storage, schema, events, SQL, aggregation, rendering, or tests | Update **both** plugin and CLI; `npm run verify:all` |

> ⚠️ **Schema changes are user-approved only.** Never modify `schema/schema.sql` without explicit user approval, even for additive changes. Always explain the rationale and ask first.

### Plugin smoke builds

```sh
npm run smoke:plugins
```

### CLI verification

```sh
npm run test:go
npm run build:cli
```

### Smoke test against real DB

Build the CLI first, then run against your local database:

```sh
npm run build:cli
npm run smoke:db
```

## Install OpenCode Plugins

### Server plugin (auto-discovered)

OpenCode auto-discovers server plugins in `~/.config/opencode/plugins/`. Symlink the server plugin there:

```sh
mkdir -p ~/.config/opencode/plugins
ln -s "$PWD/plugins/opencode-server/oc-tokeninsights-server.ts" ~/.config/opencode/plugins/
```

Then **remove** any `tokeninsights` entry from `opencode.jsonc`.

### TUI plugin (explicit config)

TUI plugins do **not** auto-discover. Add the TUI plugin to your per-OS `tui.json`:

**macOS** (`~/.config/opencode/tui.json`):
```json
{
  "plugin": [
    "/Users/dineshpandiyan/workspace/tokeninsights/plugins/opencode-tui/oc-tokeninsights.tsx"
  ]
}
```

**Linux** (`~/.config/opencode/tui.json`):
```json
{
  "plugin": [
    "/home/dee/workspace/tokeninsights/plugins/opencode-tui/oc-tokeninsights.tsx"
  ]
}
```

Do **not** add `plugins/shared/oc-tokeninsights-writer.ts` or `plugins/shared/writer-client.ts` to config. The writer is a worker module loaded internally by the server plugin.

Default DB path: `~/.local/state/tokeninsights/tokeninsights.sqlite`

Environment overrides for writers:

- `TOKENINSIGHTS_DB_PATH` — absolute path, or relative to the TokenInsights state directory
- `TOKENINSIGHTS_RETENTION_DAYS` — retention window for pruning durable rows

## Install Pi Extension

Pi extensions auto-discover from `~/.pi/agent/extensions/`. Copy or symlink the extension directory there and install its dependency:

```sh
mkdir -p ~/.pi/agent/extensions
ln -s "$PWD/plugins/pi" ~/.pi/agent/extensions/pi-tokeninsights
cd ~/.pi/agent/extensions/pi-tokeninsights
npm install
```

The Pi extension writes to the same TokenInsights DB as the OpenCode plugins (`~/.local/state/tokeninsights/tokeninsights.sqlite`) but stores data in the `pi_*` table family. The CLI reads both `oc_*` and `pi_*` tables and shows a `harness` column (`oc` or `pi`) to distinguish sources.

Tool calls are tracked as lifecycle rows (`started`, `completed`, `error`). The CLI `tool calls` tab shows started-call counts plus error counts per normal group; `tool breakdown` adds per-tool grouping.

## CLI Usage

```sh
tokeninsights-cli --db-path ~/.local/state/tokeninsights/tokeninsights.sqlite --today
tokeninsights-cli --db-path ~/.local/state/tokeninsights/tokeninsights.sqlite --week
tokeninsights-cli --db-path ~/.local/state/tokeninsights/tokeninsights.sqlite --month
tokeninsights-cli --db-path ~/.local/state/tokeninsights/tokeninsights.sqlite --all-time
tokeninsights-cli --db-path ~/.local/state/tokeninsights/tokeninsights.sqlite --week --group-by=hour
tokeninsights-cli --db-path ~/.local/state/tokeninsights/tokeninsights.sqlite --week --group-by=session
tokeninsights-cli --db-path ~/.local/state/tokeninsights/tokeninsights.sqlite --week --provider openai --model gpt-5.5
tokeninsights-cli --db-path ~/.local/state/tokeninsights/tokeninsights.sqlite --week --harness pi
tokeninsights-cli --db-path ~/.local/state/tokeninsights/tokeninsights.sqlite --month
tokeninsights-cli --db-path ~/.local/state/tokeninsights/tokeninsights.sqlite --all-time --filter-day 2026-04-24,2026-04-23
```

Interactive keys:

- `tab` / `shift+tab` — switch tabs (tokens, tps, requests, tool calls, tool breakdown)
- `g` — open grouping popup
- `f` — open filter popup for provider or harness
- `↑/↓` / `j/k` — scroll vertically or move cursor in popup
- `←/→` / `h/l` — scroll the table horizontally
- `home` / `end` — jump to the start/end of the horizontal table viewport
- grouping popup: `space` / `enter` selects grouping mode
- filter popup: `space` / `enter` enters value selection; in value selection, `space` toggles values and `enter` applies
- `esc` in a popup closes without applying staged filter changes
- `q` / `esc` / `ctrl+c` — quit
