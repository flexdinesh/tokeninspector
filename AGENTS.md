# tokeninsights — Agent Guide

Track local token usage for OpenCode and Pi. The OpenCode server plugin writes to SQLite via worker thread; the OpenCode TUI plugin reads for live display; the Pi extension writes its own table family; the Go CLI reads aggregate tables.

Full architecture, schema contract, event flow, and invariants are in [`docs/design.md`](docs/design.md). Read it before any non-trivial change.

## Agent Rules

- **Minimal, surgical changes**.
- **Never use `any`** or type assertions (`!`, `as Type`) in TypeScript.
- **Plugin and CLI are one project**. When changing storage, schema, events, SQL, aggregation, metric names, table columns, docs, or tests, update both sides in the same task.
- **Default DB path** is `~/.local/state/tokeninsights/tokeninsights.sqlite`; writer overrides use `TOKENINSIGHTS_DB_PATH` and `TOKENINSIGHTS_RETENTION_DAYS`.
- **Schema is the contract**. `schema/schema.sql` is the single source of truth. Plugin auto-migrates from it; CLI validates `PRAGMA user_version` against it.
- **Schema changes require explicit user approval**. Before modifying `schema/schema.sql`, table structures, column definitions, or any cross-language schema contract, clearly explain the reasons to the user and ask for explicit approval. Never make silent or implicit schema changes — even for non-breaking additions.
- **TPS is first-class**. Do not remove `oc_tps_samples`, `tps avg`, `tps mean`, or `tps median` when changing token schema.
- **`session_id` is required** for every durable row. Never allow token data without it.
- **Prefer real token data** over estimated stream deltas. `message.part.delta` is live UI only.
- **Missing provider/model → `unknown`**. Never drop rows for missing metadata.
- **Write for maintainability**. Do not use magic numbers in calculations for quick fixes that violate code discipline.
- **Propose refactoring**. When you see an opportunity to refactor to strongly adhere to guidelines and quality, suggest it to the user.

## Change Checklist

- Schema changed? Update `schema/schema.sql`, then run `npm run check-schema`.
- Schema changed? Update `cli/internal/db/schema.go` constants so Go tests pass.
- Plugin row shape or token semantics changed? Update CLI query structs, SQL, aggregation, rendering, tests, README, and `docs/design.md`.
- CLI query columns changed? Update `sample`, `querySamples`, scan order, aggregation, rendering, tests, README, and `docs/design.md`.
- Grouping changed? Update sorting and table alignment tests.
- Event source changed? Update plugin event handling, CLI expectations, and `docs/design.md`.
- Token semantics changed? Update tests with semantic examples.

## Commands

### Schema validation

```sh
npm run check-schema
```

### Plugin smoke build (TypeScript changes)

```sh
npm run smoke:plugins
```

### CLI verification (Go changes)

```sh
npm run test:go
npm run build:cli
```
