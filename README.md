# tokeninspector

Local token usage tools for OpenCode and Pi. The OpenCode server plugin and Pi extension write token/TPS/request/tool-call data to SQLite; the OpenCode TUI plugin queries the DB for live display; the Go CLI reads aggregate tables.

See [`docs/design.md`](docs/design.md) for full architecture, schema contract, event flow, and invariants.

## Code Organization

- `plugins/opencode-tui/` — OpenCode TUI plugin
- `plugins/opencode-server/` — OpenCode server plugin
- `plugins/shared/` — Shared types, schema migration, writer client, writer worker
- `plugins/pi/` — Pi extension
- `cli/` — Go CLI (`tokeninspector-cli`) that queries the SQLite DB
- `schema/schema.sql` — single source of truth for SQLite schema
- `scripts/check-schema.ts` — cross-language schema contract validator

## Development

### When to run what

| You changed | Run |
|-------------|-----|
| `schema/schema.sql` | `bun run scripts/check-schema.ts` |
| Any `.ts` in `plugins/` | Plugin smoke builds (see below) |
| Any `.go` in `cli/` | `cd cli && go test ./... && go build -o tokeninspector-cli ./cmd/tokeninspector-cli` |
| Storage, schema, events, SQL, aggregation, rendering, or tests | Update **both** plugin and CLI; run all of the above |

> ⚠️ **Schema changes are user-approved only.** Never modify `schema/schema.sql` without explicit user approval, even for additive changes. Always explain the rationale and ask first.

### Plugin smoke builds

```sh
bun build plugins/opencode-tui/oc-tokeninspector.tsx --target=bun --outfile=/tmp/oc-tokeninspector-check.js --external "solid-js" --external "@opentui/solid" --external "@opentui/solid/jsx-dev-runtime"
bun build plugins/shared/oc-tokeninspector-writer.ts --target=bun --outfile=/tmp/oc-tokeninspector-writer-check.js
bun build plugins/opencode-server/oc-tokeninspector-server.ts --target=bun --outfile=/tmp/oc-tokeninspector-server-check.js --external "@opencode-ai/plugin"
cd plugins/pi && npm run typecheck
```

### CLI verification

```sh
cd cli
go test ./...
go build -o tokeninspector-cli ./cmd/tokeninspector-cli
```

### Smoke test against real DB

```sh
cd cli
./tokeninspector-cli --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --today
```

## Install OpenCode Plugins

### Server plugin (auto-discovered)

OpenCode auto-discovers server plugins in `~/.config/opencode/plugins/`. Symlink the server plugin there:

```sh
mkdir -p ~/.config/opencode/plugins
ln -s "$PWD/plugins/opencode-server/oc-tokeninspector-server.ts" ~/.config/opencode/plugins/
```

Then **remove** any `tokeninspector` entry from `opencode.jsonc`.

### TUI plugin (explicit config)

TUI plugins do **not** auto-discover. Add the TUI plugin to your per-OS `tui.json`:

**macOS** (`~/.config/opencode/tui.json`):
```json
{
  "plugin": [
    "/Users/dineshpandiyan/workspace/tokeninspector/plugins/opencode-tui/oc-tokeninspector.tsx"
  ]
}
```

**Linux** (`~/.config/opencode/tui.json`):
```json
{
  "plugin": [
    "/home/dee/workspace/tokeninspector/plugins/opencode-tui/oc-tokeninspector.tsx"
  ]
}
```

Do **not** add `plugins/shared/oc-tokeninspector-writer.ts` or `plugins/shared/writer-client.ts` to config. The writer is a worker module loaded internally by the server plugin.

Default DB path: `~/.local/state/tokeninspector/tokeninspector.sqlite`

Environment overrides for writers:

- `TOKENINSPECTOR_DB_PATH` — absolute path, or relative to the TokenInspector state directory
- `TOKENINSPECTOR_RETENTION_DAYS` — retention window for pruning durable rows

## Install Pi Extension

Pi extensions auto-discover from `~/.pi/agent/extensions/`. Copy or symlink the extension directory there and install its dependency:

```sh
mkdir -p ~/.pi/agent/extensions
ln -s "$PWD/plugins/pi" ~/.pi/agent/extensions/pi-tokeninspector
cd ~/.pi/agent/extensions/pi-tokeninspector
npm install
```

The Pi extension writes to the same TokenInspector DB as the OpenCode plugins (`~/.local/state/tokeninspector/tokeninspector.sqlite`) but stores data in the `pi_*` table family. The CLI reads both `oc_*` and `pi_*` tables and shows a `harness` column (`oc` or `pi`) to distinguish sources.

Tool calls are tracked as lifecycle rows (`started`, `completed`, `error`). The CLI `tool calls` tab shows started-call counts plus error counts per normal group; `tool breakdown` adds per-tool grouping.

## CLI Usage

```sh
tokeninspector-cli --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --today
tokeninspector-cli --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --week
tokeninspector-cli --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --month
tokeninspector-cli --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --all-time
tokeninspector-cli --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --week --group-by=hour
tokeninspector-cli --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --week --group-by=session
tokeninspector-cli --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --week --provider openai --model gpt-5.5
tokeninspector-cli --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --week --harness pi
tokeninspector-cli --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --month
tokeninspector-cli --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --all-time --filter-day 2026-04-24,2026-04-23
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
