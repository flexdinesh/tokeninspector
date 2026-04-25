# Plan: Add Pi Harness Support to tokeninspector

## Compatibility: OpenCode vs Pi

| Field | OpenCode source | Pi source | Status |
|---|---|---|---|
| `input_tokens` | `step-finish` / `message.tokens` | `AssistantMessage.usage.input` | ✅ |
| `output_tokens` | `step-finish` / `message.tokens` | `AssistantMessage.usage.output` | ✅ |
| `reasoning_tokens` | `message.tokens.reasoning` | **Not exposed** — store `0` | ⚠️ Gap |
| `cache_read_tokens` | `message.tokens.cache.read` | `AssistantMessage.usage.cacheRead` | ✅ |
| `cache_write_tokens` | `message.tokens.cache.write` | `AssistantMessage.usage.cacheWrite` | ✅ |
| `total_tokens` | `tokens.total` or sum | `AssistantMessage.usage.totalTokens` | ✅ |
| `provider` | `message.updated` | `AssistantMessage.provider` | ✅ |
| `model` | `message.updated` | `AssistantMessage.model` | ✅ |
| `session_id` | `session.id` | `SessionManager.getSessionId()` | ✅ |
| `message_id` | `message.id` | Synthetic: `turnIndex` string | ✅ |
| `duration_ms` | `message.updated` timing | `turn_start` → `message_end` | ✅ |
| `ttft_ms` | `message.part.updated` | `turn_start` → first `message_update` | ✅ |
| `attempt_index` | `chat.headers` | `before_provider_request` counter | ✅ |
| `thinking_level` | `chat.params` | Extract from provider payload | ✅ |

**Pi gaps:** `reasoning_tokens` is not exposed by Pi's `Usage` type. We store `0` and compute TPS from `output + reasoning = output` only. This is acceptable because Pi either bundles reasoning into output or doesn't track it separately.

---

## Implementation Tasks

### 1. Schema (`schema/schema.sql`)
- Add `pi_token_events` — same columns as `oc_token_events` minus `part_id` and `source` (Pi has no parts), `UNIQUE(session_id, message_id)`
- Add `pi_tps_samples` — identical schema to `oc_tps_samples`
- Add `pi_llm_requests` — identical schema to `oc_llm_requests`
- Keep `PRAGMA user_version = 1` (non-breaking)

### 2. TypeScript types (`plugins/types.ts`)
- Add `PiTokenEventRow`, `PiTpsSampleRow`, `PiRequestRow`
- Update `scripts/check-schema.ts` `TABLE_TO_TS_TYPE` mapping to include `pi_*` tables

### 3. Pi extension (`pi-extension/index.ts`, `pi-extension/package.json`)
- Default export factory receiving `ExtensionAPI`
- Lazy DB init with `better-sqlite3` (synchronous API, closest Node equivalent to `bun:sqlite`)
- Reads `../../schema/schema.sql` at runtime and ports `applySchema` logic for `better-sqlite3`
- `package.json` declares `better-sqlite3` dependency; user runs `npm install` in extension dir
- Event handlers:
  - `session_start`: init per-session turn state `Map`
  - `turn_start`: record `requestStartAt`, set `currentTurnIndex`, generate `messageId = String(turnIndex)`
  - `before_provider_request`: extract thinking level from payload, increment attempt count, write one `pi_llm_requests` row
  - `message_update` (assistant only): record `firstTokenAt` for TTFT
  - `message_end` (assistant only): read `usage`, write `pi_token_events`; compute `durationMs`, `ttftMs`, `tokensPerSecond`, write `pi_tps_samples`
  - `session_shutdown`: flush and cleanup per-session state
- All writes happen synchronously (no worker thread needed — `better-sqlite3` is fast enough for this volume)
- `reasoning_tokens = 0` everywhere
- Graceful degradation: if DB init fails, log to stderr and disable tracking; never crash Pi startup

### 4. Go CLI schema constants (`cli/internal/db/schema.go`)
- Add `TablePiTokenEvents`, `TablePiTpsSamples`, `TablePiLLMRequests`

### 5. Go CLI aggregation (`cli/internal/db/aggregate.go`)
- Add `Harness string` to `Row` and `rowKey`
- Update `groupAliases`, `groupByAliases`, `partitionBy`, `scanKey` to include harness literal
- Update sort order: `harness asc`, then existing order
- `aggregateTokenEvents`: query `oc_token_events` (always), then query `pi_token_events` if table exists, merge via `mergeRow`
- `aggregateTpsSamples`: query `oc_tps_samples` if exists, then `pi_tps_samples` if exists, merge
- `aggregateLLMRequests`: query `oc_llm_requests` if exists, then `pi_llm_requests` if exists, merge

### 6. Go CLI rendering (`cli/internal/cli/render.go`, `cli/internal/cli/table.go`)
- Add `harness` column after grouping columns (day/hour/session)
- Short values: `oc` for OpenCode, `pi` for Pi
- Update column widths and alignment (text-left, numeric-right)

### 7. Go CLI tests
- `cli/internal/db/db_test.go`: insert `pi_*` fixture rows, assert aggregation merges both families
- `cli/internal/db/schema_test.go`: verify new constants
- `cli/internal/cli/render_test.go`: update golden output for harness column

### 8. Docs
- `docs/design.md`: update architecture diagram, document Pi event flow, Pi gaps, and `pi_*` table family
- `README.md`: add Pi extension installation instructions (symlink or copy `pi-extension/` to `~/.pi/agent/extensions/pi-tokeninspector/`)

### 9. Verification
- `bun run scripts/check-schema.ts`
- `cd cli && go test ./...`
- Type-check Pi extension with `tsc --noEmit` inside `pi-extension/`

---

## Decisions made

| Decision | Choice |
|---|---|
| Schema approach | Separate `pi_*` tables (user chose) |
| `reasoning_tokens` for Pi | `0` (Pi gap, user accepted) |
| `part_id` / `source` for Pi | Omitted (no Pi equivalent) |
| SQLite library for Pi | `better-sqlite3` (synchronous, Node.js compatible) |
| DB file | Shared `oc-tps.sqlite`, Pi writes `pi_*` tables alongside `oc_*` |
| Harness identifier | `pi` (user specified) |
| `message_id` in Pi | Synthetic from `turnIndex` (Pi entries lack message IDs) |
| Extension directory | `pi-extension/` (confirmed by user) |
| Harness column in CLI | Shown unconditionally (confirmed by user) |

## Tradeoffs / Risks

- `better-sqlite3` requires native compilation on `npm install`. Most platforms have prebuilt binaries, but exotic architectures may fail. Alternative is `sqlite3` (async-only) which changes the extension code shape.
- The CLI `harness` column adds width for all users, even OpenCode-only. This is the cost of a unified view.
- Pi `turnIndex`-based `message_id` is not globally unique but is unique per session, satisfying our `UNIQUE(session_id, message_id)` constraint.
