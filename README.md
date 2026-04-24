# tokeninspector

Local OpenCode token usage tools.

- `plugins/oc-tokeninspector.tsx`: OpenCode TUI plugin that records token and TPS data to SQLite.
- `cli/`: Go CLI that queries the SQLite DB.

## Install OpenCode Plugin

Add the plugin to OpenCode TUI config:

```json
{
  "plugin": [
    "/Users/dineshpandiyan/workspace/tokeninspector/plugins/oc-tokeninspector.tsx"
  ]
}
```

The dotfiles OpenCode config does not load this plugin by default after this move.

Default DB path:

```text
~/.local/state/opencode/oc-tps.sqlite
```

## Verify

From this repo:

```sh
bun build plugins/oc-tokeninspector.tsx --target=bun --outfile=/tmp/oc-tokeninspector-check.js --external "solid-js" --external "@opentui/solid" --external "@opentui/solid/jsx-dev-runtime"
```

From `cli/`:

```sh
go test ./...
go build -o tokeninspector-cli .
```
