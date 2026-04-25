# Plan: Reorganize plugins into per-harness directories

## Summary
Move all plugin code into three harness-specific directories under `plugins/`:
- `plugins/opencode-tui/` — TUI plugin
- `plugins/opencode-server/` — server plugin
- `plugins/pi/` — Pi extension
- `plugins/shared/` — shared types, schema migration, writer client, writer worker

## Detailed Steps

### 1. Create directories and move files
- `mkdir -p plugins/shared plugins/opencode-tui plugins/opencode-server plugins/pi`
- Move `plugins/types.ts` → `plugins/shared/types.ts`
- Move `plugins/schema-migrate.ts` → `plugins/shared/schema-migrate.ts`
- Move `plugins/writer-client.ts` → `plugins/shared/writer-client.ts`
- Move `plugins/oc-tokeninspector-writer.ts` → `plugins/shared/oc-tokeninspector-writer.ts`
- Move `plugins/oc-tokeninspector.tsx` → `plugins/opencode-tui/oc-tokeninspector.tsx`
- Move `plugins/oc-tokeninspector-server.ts` → `plugins/opencode-server/oc-tokeninspector-server.ts`
- Move `pi-extension/index.ts` → `plugins/pi/index.ts`
- Move `pi-extension/package.json` → `plugins/pi/package.json`
- Remove empty `pi-extension/` directory after move

### 2. Create `.gitignore`
- Add `node_modules/` to root `.gitignore`

### 3. Update import paths
- `plugins/shared/writer-client.ts`: no change (still `./types.ts`)
- `plugins/shared/oc-tokeninspector-writer.ts`: no change (still `./schema-migrate.ts` and `./types.ts`)
- `plugins/opencode-tui/oc-tokeninspector.tsx`: change `from "./types.ts"` → `from "../shared/types.ts"`
- `plugins/opencode-server/oc-tokeninspector-server.ts`:
  - `from "./writer-client.ts"` → `from "../shared/writer-client.ts"`
  - `from "./schema-migrate.ts"` → `from "../shared/schema-migrate.ts"`
  - `from "./types.ts"` → `from "../shared/types.ts"`
  - Worker URL: `new URL("./oc-tokeninspector-writer.ts", import.meta.url)` → `new URL("../shared/oc-tokeninspector-writer.ts", import.meta.url)`

### 4. Update schema checker path
- `scripts/check-schema.ts`: change `TS_TYPES_PATH` from `plugins/types.ts` to `plugins/shared/types.ts`

### 5. Update docs
- `docs/design.md`: Update all file organization tables and build commands to use new paths
- `README.md`: Update plugin paths, build commands, install symlinks, and code organization section
- `AGENTS.md`: Update build command paths

### 6. Verification
- Run `bun run scripts/check-schema.ts`
- Run plugin smoke builds with new paths
- Run `cd cli && go test ./... && go build -o tokeninspector-cli .`
