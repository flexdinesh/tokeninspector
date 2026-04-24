# tokeninspector

Local OpenCode token usage tools.

- `plugins/oc-tokeninspector.tsx`: OpenCode TUI plugin that records token and TPS data to SQLite.
- `plugins/oc-tokeninspector-server.ts`: OpenCode server plugin that records LLM request attempts to SQLite.
- `cli/`: Go CLI that queries the SQLite DB.

## Install OpenCode Plugins

Add the server plugin to `opencode.jsonc`:

```jsonc
{
  "plugin": [
    "/Users/dineshpandiyan/workspace/tokeninspector/plugins/oc-tokeninspector-server.ts"
  ]
}
```

Add the TUI plugin to `tui.json`:

```jsonc
{
  "plugin": [
    "/Users/dineshpandiyan/workspace/tokeninspector/plugins/oc-tokeninspector.tsx"
  ]
}
```

Do not add `plugins/oc-tokeninspector-writer.ts` to config. It is a worker loaded by the TUI plugin.

The dotfiles OpenCode config does not load this plugin by default after this move.

Default DB path:

```text
~/.local/state/opencode/oc-tps.sqlite
```

## Verify

From this repo:

```sh
bun build plugins/oc-tokeninspector.tsx --target=bun --outfile=/tmp/oc-tokeninspector-check.js --external "solid-js" --external "@opentui/solid" --external "@opentui/solid/jsx-dev-runtime"
bun build plugins/oc-tokeninspector-writer.ts --target=bun --outfile=/tmp/oc-tokeninspector-writer-check.js
bun build plugins/oc-tokeninspector-server.ts --target=bun --outfile=/tmp/oc-tokeninspector-server-check.js --external "@opencode-ai/plugin"
```

From `cli/`:

```sh
go test ./...
go build -o tokeninspector-cli .
```
