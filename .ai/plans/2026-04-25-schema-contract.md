# Plan: Schema Contract + Plugin Type Extraction

## Decisions

- **Schema contract**: SQL-file-as-contract (`schema/schema.sql`). Cross-language standard. Both TS writers and Go reader reference it.
- **Schema directory**: `/schema/` at repo root, shared by both plugin and CLI.
- **Validation scope**: Deep check — validates `schema.sql` against Go `schema.go` constants, TS `types.ts` field names, and SQL column names.
- **Plugin changes**: Minimal type extraction + self-sufficient migration capability.
- **CLI changes**: One new `schema_test.go` that parses `schema/schema.sql` and validates Go constants. No other CLI changes.
- **Plugin migration**: Plugin must diff existing DB schema against `schema.sql` and apply changes independently (users may not have CLI installed).
- **Test strategy**: No `//go:embed` — Go test reads `schema/schema.sql` from source at test time.
- **Safety**: Each step verified independently before proceeding.

---

## Phase 1: Schema Contract

### Task A: Create `schema/schema.sql` (INDEPENDENT)

Create `schema/schema.sql` containing all `CREATE TABLE` and `CREATE INDEX` statements from:
- `plugins/oc-tokeninspector-writer.ts` (oc_token_events, oc_tps_samples)
- `plugins/oc-tokeninspector-server.ts` (oc_llm_requests)

Order: pragmas, tables, indexes. No data.

**Verification**: File exists and contains valid SQLite DDL.

---

### Task B: Write `scripts/check-schema.ts` — Deep Validation (DEPENDS ON A)

Bun script that performs deep validation:

1. Parse `schema/schema.sql` → extract table names, column names, index names
2. Read `cli/internal/db/schema.go` → extract all `const` declarations
3. Verify every Go column/table constant matches a name from the SQL
4. Read `plugins/types.ts` (after Task E) → extract interface/type field names
5. Verify TS field names follow naming convention that maps to SQL columns (e.g., `inputTokens` ↔ `input_tokens`)
6. Non-zero exit on any mismatch

**Verification**: `bun run scripts/check-schema.ts` exits 0 on current codebase.

---

### Task C: Write Plugin Migration Helper (DEPENDS ON A)

Create `plugins/schema-migrate.ts`:

1. Read `schema/schema.sql` via `Bun.file()`
2. Parse CREATE TABLE / CREATE INDEX statements
3. For each table: query `PRAGMA table_info()` to get existing columns
4. Compare with desired columns from schema.sql
5. Apply `ALTER TABLE ... ADD COLUMN ...` for any missing columns
6. Apply `CREATE INDEX IF NOT EXISTS` for all indexes
7. Apply `CREATE TABLE IF NOT EXISTS` for all tables
8. Set `PRAGMA user_version` to match schema version

This makes the plugin self-sufficient: it can upgrade an old DB to the latest schema without the CLI.

**Verification**: Test with an old DB file (missing `thinking_level` column) — helper adds it.

---

### Task D: Update TS Writers to Use `schema.sql` + Migration Helper (DEPENDS ON A, C)

- `plugins/oc-tokeninspector-writer.ts`:
  - Remove inline DDL strings for `oc_token_events` and `oc_tps_samples`
  - Call migration helper with `schema/schema.sql` at init time
  - Keep prepared statements (they can stay inline since they're DML, not DDL)

- `plugins/oc-tokeninspector-server.ts`:
  - Remove inline DDL strings for `oc_llm_requests`
  - Call migration helper with `schema/schema.sql` at init time
  - Remove the existing `ALTER TABLE ... ADD COLUMN thinking_level` (now handled by migration helper)

**Verification**:
```sh
bun build plugins/oc-tokeninspector.tsx --target=bun --outfile=/tmp/oc-tokeninspector-check.js --external "solid-js" --external "@opentui/solid" --external "@opentui/solid/jsx-dev-runtime"
bun build plugins/oc-tokeninspector-writer.ts --target=bun --outfile=/tmp/oc-tokeninspector-writer-check.js
bun build plugins/oc-tokeninspector-server.ts --target=bun --outfile=/tmp/oc-tokeninspector-server-check.js --external "@opencode-ai/plugin"
```

---

### Task E: Extract `plugins/types.ts` (INDEPENDENT, CAN PARALLEL WITH A)

Move shared types from three files into one:

From `plugins/oc-tokeninspector.tsx`:
- `StreamSample`, `MessageTiming`, `SessionAverage`, `TrackerState`
- `TokenCounts`, `MessageInfo`, `TokenEventSource`, `TokenEventRow`, `TpsSampleRow`
- `TokenStorageConfig`, `TokenStorage`, `MessageInfoUpdate`, `WriterResponse`

From `plugins/oc-tokeninspector-writer.ts`:
- `TokenEventRow`, `TpsSampleRow`, `MessageInfo`, `MessageInfoUpdate`
- `TokenStorage` (same name, same shape — deduplicate)

From `plugins/oc-tokeninspector-server.ts`:
- `RequestRow`, `RequestStorage`

Keep types that are file-specific (e.g., `InitMessage`, `FlushMessage`, `CloseMessage` in writer.ts) in their original files.

**Verification**: Plugin smoke builds pass after import updates.

---

### Task F: Write Go `schema_test.go` (DEPENDS ON A)

`cli/internal/db/schema_test.go`:

1. Read `schema/schema.sql` from source (no `//go:embed` — reads file at test time)
2. Parse SQL to extract table names and column names
3. Verify all `schema.go` constants exist in the SQL
4. Verify all SQL table names have matching Go constants
5. Verify column count matches (catches missing column constants)
6. Fails test on mismatch with clear error message

**Verification**: `go test ./...` passes.

---

### Task G: Update `scripts/check-schema.ts` for Deep Type Validation (DEPENDS ON B, E)

After Task E extracts `plugins/types.ts`, extend the script from Task B:

1. Parse `plugins/types.ts` to extract interface field names
2. Map field naming convention (camelCase → snake_case)
3. Verify mapped names exist as columns in `schema/schema.sql`
4. Flag any TS field that doesn't map to a known SQL column
5. Flag any SQL column that doesn't have a corresponding TS field (for tables that should have one)

**Verification**: `bun run scripts/check-schema.ts` exits 0.

---

### Task H: Update `AGENTS.md` (DEPENDS ON ALL ABOVE, SERIAL)

Replace manual schema checklist with:
- `schema/schema.sql` is the single source of truth for table/column definitions
- Plugin writers auto-migrate DB using `plugins/schema-migrate.ts`
- Command: `bun run scripts/check-schema.ts` validates cross-language contract
- Build verification now includes schema check

**Verification**: Documentation accurately describes the new workflow.

---

## Phase 2: Plugin Type Extraction (Minimal)

### Task I: Update Imports Across Plugin Files (DEPENDS ON E)

- `plugins/oc-tokeninspector.tsx`: import shared types from `./types.ts`
- `plugins/oc-tokeninspector-writer.ts`: import shared types from `./types.ts`
- `plugins/oc-tokeninspector-server.ts`: import shared types from `./types.ts`

Remove duplicate type definitions. ~60 lines of duplication eliminated.

**Verification**: All plugin smoke builds pass.

---

## Parallelization Map

```
Phase 1:
  A: Create schema/schema.sql ──┬──┬──┬──┐
  E: Extract plugins/types.ts ──┘  │  │  │
                                    │  │  │
  B: check-schema.ts (shallow) <───┘  │  │
  C: Plugin migration helper <────────┘  │
  F: Go schema_test.go <─────────────────┘
                                    │
  D: Update TS writers <────────────┘
                                    │
  G: check-schema.ts (deep) <───────┘
                                    │
  H: Update AGENTS.md <─────────────┘

Phase 2:
  I: Update imports <─── depends on E
```

**Parallel agent spawn points**:
- Spawn agents for **A** and **E** simultaneously (both independent)
- After A completes, spawn agents for **B**, **C**, **F** simultaneously
- After B+C complete, spawn agent for **D**
- After B+E complete, spawn agent for **G**
- After all above complete, run **H** and **I** serially

---

## Migration Strategy Detail

Since the plugin must work without the CLI, `plugins/schema-migrate.ts` implements a minimal forward-only migration:

1. **Tables**: `CREATE TABLE IF NOT EXISTS` — idempotent, handles new tables
2. **Indexes**: `CREATE INDEX IF NOT EXISTS` — idempotent, handles new indexes
3. **Columns**: `PRAGMA table_info(table_name)` → compare with `schema.sql` → `ALTER TABLE ... ADD COLUMN ...` for missing ones
4. **Version**: `PRAGMA user_version = N` after successful migration

This handles the common case (adding tables, columns, indexes) without a full migration framework. Destructive changes (renames, drops) require manual intervention and a version bump.

**Rationale**: `oc-tokeninspector-server.ts` already demonstrates this pattern with its inline `ALTER TABLE oc_llm_requests ADD COLUMN thinking_level`. The migration helper generalizes this pattern and makes it data-driven from `schema.sql`.

---

## Scope: What's NOT Included

- No Makefile (AGENTS.md commands sufficient per user)
- No CLI code changes beyond `schema_test.go`
- No plugin modularization (no splitting `.tsx` into smaller files)
- No `tsconfig.json` or linting
- No ORM/sqlc
- No destructive schema change handling (renames/drops)

---

## Tradeoffs

| Decision | Chosen | Rejected | Rationale |
|---|---|---|---|
| SQL-file-as-contract | `schema/schema.sql` | JSON schema, inline DDL | Cross-language standard; readable; zero parsing complexity |
| Schema dir location | `/schema/` at repo root | `cli/schema/`, `plugins/schema/` | Shared by both sides |
| Plugin migration | Diff-based from schema.sql | Numbered migration files | Simpler for low-churn project; handles 90% of cases |
| Validation script language | Bun/TypeScript | Go | Can parse both TS and Go source; lives with schema |
| Go schema test | Read file at test time | `//go:embed` | Flexible; developer-side only |
| TS type extraction | Single `types.ts` | Multiple type files | Minimal churn; clear boundary |
| Deep type validation | Convention check (camel→snake) | Full AST parsing | Simpler; catches common drift |

---

## Verification Commands

After each phase:
```sh
# Schema validation
bun run scripts/check-schema.ts

# Plugin smoke builds
bun build plugins/oc-tokeninspector.tsx --target=bun --outfile=/tmp/oc-tokeninspector-check.js --external "solid-js" --external "@opentui/solid" --external "@opentui/solid/jsx-dev-runtime"
bun build plugins/oc-tokeninspector-writer.ts --target=bun --outfile=/tmp/oc-tokeninspector-writer-check.js
bun build plugins/oc-tokeninspector-server.ts --target=bun --outfile=/tmp/oc-tokeninspector-server-check.js --external "@opencode-ai/plugin"

# Go tests
cd cli && go test ./...

# Full build
cd cli && go build -o tokeninspector-cli .
```

---

## Rollout Notes

- If implementation deviates, update this plan file and notify user with reasons.
- After execution: list changed files, decisions, and commands run.
