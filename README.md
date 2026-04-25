# tokeninspector

Local OpenCode token usage tools. The server plugin writes token/TPS/request data to SQLite via a worker thread; the TUI plugin queries the DB for live display; the Go CLI reads aggregate tables.

See [`docs/design.md`](docs/design.md) for full architecture, schema contract, event flow, and invariants.

## Code Organization

- `plugins/` — TypeScript OpenCode plugins (TUI + server), shared types, and writer client
- `cli/` — Go CLI (`tokeninspector-cli`) that queries the SQLite DB
- `schema/schema.sql` — single source of truth for SQLite schema
- `scripts/check-schema.ts` — cross-language schema contract validator

## Development

### When to run what

| You changed | Run |
|-------------|-----|
| `schema/schema.sql` | `bun run scripts/check-schema.ts` |
| Any `.ts` in `plugins/` | Plugin smoke builds (see below) |
| Any `.go` in `cli/` | `cd cli && go test ./... && go build -o tokeninspector-cli .` |
| Storage, schema, events, SQL, aggregation, rendering, or tests | Update **both** plugin and CLI; run all of the above |

### Plugin smoke builds

```sh
bun build plugins/oc-tokeninspector.tsx --target=bun --outfile=/tmp/oc-tokeninspector-check.js --external "solid-js" --external "@opentui/solid" --external "@opentui/solid/jsx-dev-runtime"
bun build plugins/oc-tokeninspector-writer.ts --target=bun --outfile=/tmp/oc-tokeninspector-writer-check.js
bun build plugins/oc-tokeninspector-server.ts --target=bun --outfile=/tmp/oc-tokeninspector-server-check.js --external "@opencode-ai/plugin"
```

### CLI verification

```sh
cd cli
go test ./...
go build -o tokeninspector-cli .
```

### Smoke test against real DB

```sh
cd cli
./tokeninspector-cli --db-path ~/.local/state/opencode/oc-tps.sqlite --day
```

## Install OpenCode Plugins

### Server plugin (auto-discovered)

OpenCode auto-discovers server plugins in `~/.config/opencode/plugins/`. Symlink the server plugin there:

```sh
mkdir -p ~/.config/opencode/plugins
ln -s "$PWD/plugins/oc-tokeninspector-server.ts" ~/.config/opencode/plugins/
```

Then **remove** any `tokeninspector` entry from `opencode.jsonc`.

### TUI plugin (explicit config)

TUI plugins do **not** auto-discover. Add the TUI plugin to your per-OS `tui.json`:

**macOS** (`~/.config/opencode/tui.json`):
```json
{
  "plugin": [
    "/Users/dineshpandiyan/workspace/tokeninspector/plugins/oc-tokeninspector.tsx"
  ]
}
```

**Linux** (`~/.config/opencode/tui.json`):
```json
{
  "plugin": [
    "/home/dee/workspace/tokeninspector/plugins/oc-tokeninspector.tsx"
  ]
}
```

Do **not** add `plugins/oc-tokeninspector-writer.ts` or `plugins/writer-client.ts` to config. The writer is a worker module loaded internally by the server plugin.

Default DB path: `~/.local/state/opencode/oc-tps.sqlite`

## CLI Usage

```sh
tokeninspector-cli --db-path ~/.local/state/opencode/oc-tps.sqlite --day
tokeninspector-cli --db-path ~/.local/state/opencode/oc-tps.sqlite --week
tokeninspector-cli --db-path ~/.local/state/opencode/oc-tps.sqlite --month
tokeninspector-cli --db-path ~/.local/state/opencode/oc-tps.sqlite --week --group-by=hour
tokeninspector-cli --db-path ~/.local/state/opencode/oc-tps.sqlite --week --group-by=session
tokeninspector-cli --db-path ~/.local/state/opencode/oc-tps.sqlite --week --provider openai --model gpt-5.5
tokeninspector-cli --db-path ~/.local/state/opencode/oc-tps.sqlite --month --filter-day 2026-04-24,2026-04-23
```

Interactive keys:

- `tab` / `shift+tab` — switch tabs (tokens, tps, requests)
- `g` — open grouping popup
- `↑/↓` / `j/k` — scroll or move cursor in popup
- `space` / `enter` — select grouping mode
- `q` / `esc` / `ctrl+c` — quit
