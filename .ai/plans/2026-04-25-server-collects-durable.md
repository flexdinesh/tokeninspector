# Plan: Move Durable Collection to Server Plugin

## Decisions

- **Server plugin collects all durable data**: token events, TPS samples, fallback rows, LLM requests.
- **TUI plugin only handles live display**: `message.part.delta` estimates for live TPS banner + DB queries for session averages/TTFT.
- **Server plugin writes via shared worker thread**: `oc-tokeninspector-writer.ts` is shared between TUI and server. Both post to the same Bun worker.
- **Session averages become persistent**: TUI queries `oc_tps_samples` instead of tracking in-memory averages that reset on restart.
- **DB query interval is configurable**: `const BANNER_REFRESH_MS = 2000` in TUI plugin.
- **No schema changes**: tables remain the same; only event handlers move.
- **TUI retains `message.part.delta` handler**: this is TUI-only; server cannot receive `message.part.delta`.

---

## Phase 1: Extract Shared Writer Logic

### Task A: Extract `createTokenStorage` from `oc-tokeninspector-writer.ts` into `plugins/writer-client.ts`

Create `plugins/writer-client.ts`:

```ts
import type { TokenEventRow, TpsSampleRow, MessageInfoUpdate, TokenStorage, WriterResponse } from "./types.ts"

export function createTokenStorage(
  importMetaUrl: string,
  config: { dbPath: string; retentionDays: number },
  onError: () => void,
): TokenStorage
```

Implementation: copy the existing `createTokenStorage` logic (Worker spawn, init/flush/close postMessage, ready/error tracking). Parameterize the worker URL via `importMetaUrl` so both TUI and server can construct the correct relative URL.

Keep `plugins/oc-tokeninspector-writer.ts` as the worker entry point (it registers `self.onmessage`). No changes to the worker code itself.

**Verification**: smoke build passes for both plugins importing `writer-client.ts`.

---

### Task B: Update `plugins/types.ts` — Add `RequestRow` and `RequestStorage` if missing

Ensure `RequestRow` and `RequestStorage` are in `plugins/types.ts`. Already present from previous refactor — verify.

Also add to `plugins/types.ts`:

```ts
export type WriterConfig = {
  dbPath: string
  retentionDays: number
}
```

**Verification**: `bun build` for all plugin files passes.

---

## Phase 2: Server Plugin — Add Durable Collection

### Task C: Move durable event handlers from TUI to server plugin

Update `plugins/oc-tokeninspector-server.ts`:

1. **Import `createTokenStorage` from `./writer-client.ts`**
2. **Add `event` hook** handling:
   - `message.part.updated` (step-finish parts)
   - `message.updated` (assistant messages: fallback rows, TPS samples, metadata)
   - `session.idle` (fallback scan + flush)
   - `session.deleted` (fallback scan + flush)

3. **Copy in-memory tracking structures** from TUI plugin:
   - `messageInfoByID: Record<string, MessageInfo>`
   - `messagesWithStepRows: Set<string>` (keyed by `sessionID:messageID`)
   - `seenSessionIDs: Set<string>`
   - `pendingRows: TokenEventRow[]`
   - `pendingTpsRows: TpsSampleRow[]`
   - `pendingInfoUpdates: MessageInfoUpdate[]`
   - In-memory `messageTimingByID` and `sessionAverageByID` **are NOT needed** in server (server doesn't display live TPS; only writes durable rows)
   - But `messageTimingByID` **IS needed** for TPS duration/TTFT computation

4. **Copy helper functions** from TUI plugin:
   - `knownValue`
   - `messageKey`
   - `totalTokenCount`
   - `tokenEventRow`
   - `estimateStreamTokens` **NOT needed** (server doesn't handle deltas)
   - `queueTokenEvent`, `queueFallbackEvent`, `queueTpsSample`
   - `updateMessageInfo`
   - `flushRows` (filters by sessionID or flushes all)
   - `queueSessionFallbacks` (uses `api.state.session.messages()` — **WAIT**: server plugin does NOT have `api.state`. Must find alternative.)

5. **Session fallback scan on server**:
   - Server `event` hook receives `session.idle` with `event.properties.sessionID`.
   - Server does NOT have `api.state.session.messages(id)`.
   - However, `message.updated` events for assistant messages already arrive with full message info.
   - We can track `message.updated` events in-memory and scan that on `session.idle`.
   - Add `serverMessageInfoBySessionID: Record<string, Array<{ id, role, completedAt, providerID, modelID, tokens }>>` populated by `message.updated`.
   - On `session.idle`, scan this per-session array for completed assistant messages without step rows.

6. **Timer-based flush**: `setInterval(flushRows, 1000)` in server plugin, plus cleanup on plugin unload.

7. **Lazy DB init**: reuse existing `getStorage()` pattern. Create `TokenStorage` on first `message.part.updated` (step-finish) or `message.updated` (assistant).

**Verification**:
```sh
bun build plugins/oc-tokeninspector-server.ts --target=bun --outfile=/tmp/oc-tokeninspector-server-check.js --external "@opencode-ai/plugin"
```

---

### Task D: Remove durable collection from TUI plugin

Update `plugins/oc-tokeninspector.tsx`:

1. **Remove all DB write plumbing**:
   - Remove `createTokenStorage` call and `storage` management
   - Remove `flushRows`, `updateMessageInfo`, `queueTokenEvent`, `queueFallbackEvent`, `queueTpsSample`
   - Remove `pendingRows`, `pendingTpsRows`, `pendingInfoUpdates`
   - Remove `messageInfoByID`, `messagesWithStepRows`, `seenSessionIDs`
   - Remove `messageTimingByID` and `sessionAverageByID` — replaced by DB queries

2. **Keep live TPS display**:
   - Keep `message.part.delta` handler for `text`/`reasoning` fields
   - Keep `streamSamplesBySession` and `appendSample`
   - Keep `activeDurationMs`, `estimateStreamTokens`, `formatRate`

3. **Add DB query for session averages**:
   - Import `Database` from `bun:sqlite` (or use read-only mode)
   - On `session_prompt_right` render or timer, query:
     ```sql
     SELECT SUM(total_tokens) as total_tokens, SUM(duration_ms) as total_duration_ms, AVG(ttft_ms) as avg_ttft_ms, COUNT(*) as message_count
     FROM oc_tps_samples
     WHERE session_id = ? AND recorded_at_ms > ?
     ```
   - Use a configurable lookback window (e.g., `SESSION_LOOKBACK_MS = 24 * 60 * 60 * 1000` for current session day)
   - Actually, for per-session display: `WHERE session_id = ?` with no time filter.

4. **Add periodic refresh**:
   - `const BANNER_REFRESH_MS = 2000` (configurable)
   - `setInterval(() => { queryDB(); bump(); }, BANNER_REFRESH_MS)`
   - Or query on each `message.updated` + `setInterval` when idle.

5. **Handle DB not ready gracefully**:
   - If DB path doesn't exist, show `-` for all averages.
   - If query fails, show `-` and optional toast.

**Key concern**: TUI plugin and server plugin may both write to DB. With server plugin writing and TUI plugin reading, there's no writer contention. BUT if server plugin also reads, or if both write — we have two writers. We must ensure:
   - TUI plugin no longer writes. It only reads.
   - Server plugin is the sole writer.
   - This eliminates the dual-writer problem entirely.

**Verification**:
```sh
bun build plugins/oc-tokeninspector.tsx --target=bun --outfile=/tmp/oc-tokeninspector-check.js --external "solid-js" --external "@opentui/solid" --external "@opentui/solid/jsx-dev-runtime"
```

---

## Phase 3: Server Plugin Message Tracking for Fallbacks

### Task E: Implement `ServerMessageTracker` in server plugin

Since server plugin lacks `api.state.session.messages()`, we need an in-memory message tracker:

```ts
type ServerMessageInfo = {
  id: string
  role: string
  completedAt?: number
  providerID?: string
  modelID?: string
  tokens?: TokenCounts
}

const serverMessagesBySession: Record<string, ServerMessageInfo[]> = {}
```

- On `message.updated`: append or update the message in `serverMessagesBySession[sessionID]`.
- On `session.idle` / `session.deleted`: scan session messages for completed assistant messages not in `messagesWithStepRows`, queue fallback rows, flush, then clear session data.
- On `session.deleted`: also clear `serverMessagesBySession[sessionID]`.

**This replaces `api.state.session.messages()` dependency.**

**Verification**: server plugin smoke build passes.

---

## Phase 4: TUI Plugin — DB Query for Session Averages

### Task F: Implement `querySessionAverages(dbPath, sessionID)` in TUI plugin

```ts
function querySessionAverages(dbPath: string, sessionID: string): {
  avgTps: number | undefined
  avgTtft: number | undefined
} | undefined
```

Implementation:
1. Check if `dbPath` file exists (`Bun.file(dbPath).exists()` or `statSync`)
2. Open `new Database(dbPath, { readonly: true })` or use `mode=ro`
3. Run:
   ```sql
   SELECT SUM(total_tokens) as throughput_tokens, SUM(duration_ms) as duration_ms, AVG(ttft_ms) as avg_ttft_ms
   FROM oc_tps_samples
   WHERE session_id = ? AND duration_ms > 0
   ```
4. Compute `avgTps = throughput_tokens / (duration_ms / 1000)`
5. Compute `avgTtft = avg_ttft_ms / 1000`
6. Close DB or keep connection open (SQLite `mode=ro` is safe for concurrent reads)

**Performance**: a `SELECT` on an indexed `session_id` column is <1ms. Acceptable for 1–2s refresh.

**Verification**: manual test with real DB file.

---

## Phase 5: Update Tests, Docs, and Schema Checks

### Task G: Update `cli/internal/db/schema.go` constants if needed

No new tables/columns — should be unchanged. Verify with:
```sh
cd cli && go test ./...
```

---

### Task H: Update `docs/design.md`

Update architecture diagram and plugin event flow:

1. **Architecture diagram**: Server plugin now writes all three table types. TUI plugin reads only.
2. **TUI Plugin event flow**: only `message.part.delta` for live display. Remove `message.part.updated`, `message.updated`, `session.idle`, `session.deleted` durable write descriptions.
3. **Server Plugin event flow**: add `message.part.updated` (step-finish), `message.updated` (fallback + TPS), `session.idle` (fallback scan + flush), `session.deleted` (cleanup + flush).
4. **Plugin / CLI Boundary**: clarify "Server plugin writes directly; TUI plugin writes nothing."

---

### Task I: Update `AGENTS.md`

Update agent rules if any plugin-specific guidance changed. Likely minimal changes.

---

### Task J: Update `plugins/README.md` or add inline comments

Document:
- `BANNER_REFRESH_MS` constant in TUI plugin
- Server plugin message tracker behavior
- Shared writer module usage

---

## Phase 6: Verification

### Full verification run

```sh
# Schema check (should pass — no schema changes)
bun run scripts/check-schema.ts

# Plugin smoke builds
bun build plugins/oc-tokeninspector.tsx --target=bun --outfile=/tmp/oc-tokeninspector-check.js --external "solid-js" --external "@opentui/solid" --external "@opentui/solid/jsx-dev-runtime"
bun build plugins/oc-tokeninspector-writer.ts --target=bun --outfile=/tmp/oc-tokeninspector-writer-check.js
bun build plugins/oc-tokeninspector-server.ts --target=bun --outfile=/tmp/oc-tokeninspector-server-check.js --external "@opencode-ai/plugin"

# Go tests
cd cli && go test ./...

# Full CLI build
cd cli && go build -o tokeninspector-cli .
```

---

## Parallelization Map

```
Phase 1:
  A: Extract writer-client.ts ───────┐
  B: Verify types.ts ────────────────┘
                                    │
  C: Server plugin durable handlers <─┘
                                    │
  E: Server message tracker <───────┘ (part of C)
                                    │
  D: TUI plugin slim-down <───────────┘
                                    │
  F: TUI DB query <─────────────────┘ (part of D)
                                    │
  G: Schema/Go tests <──────────────┘
                                    │
  H: docs/design.md <─────────────────┘
  I: AGENTS.md <──────────────────────┘
  J: README/comments <────────────────┘
```

**Parallel agent spawn points**:
- Spawn A+B simultaneously (both minimal)
- After A completes, spawn C+E simultaneously
- After C completes, spawn D+F simultaneously
- After D completes, spawn G, H, I, J simultaneously

---

## Invariants (Must Not Break)

1. `session_id` is required for every durable row — unchanged.
2. TPS tables/columns/metrics must remain — unchanged.
3. Plugin init must never block OpenCode — server plugin already lazy; TUI plugin now has no DB init blocking.
4. Missing provider/model → `unknown` — unchanged.
5. Schema contract — no schema changes in this refactor.
6. `message.part.delta` estimates are NOT persisted — unchanged (still TUI-only).

---

## Tradeoffs

| Decision | Chosen | Rejected | Rationale |
|---|---|---|---|
| Collection location | Server plugin | TUI plugin | TUI should be lightweight; server handles background work |
| Writer pattern | Shared worker thread | Direct writes in server | User preference; worker isolates DB blocking from server event loop |
| TUI averages source | DB query | In-memory | Persistent across TUI restarts; slight latency (1–2s) |
| Fallback scan on server | In-memory message tracker | `api.state.session.messages()` | Server plugin lacks state API; tracker replicates needed data |
| DB query interval | Configurable constant (2s default) | Event-driven only | Ensures banner updates even when idle; avoids per-event query storm |

---

## Rollout Notes

- If implementation deviates, update this plan file and notify user with reasons.
- After execution: list changed files, decisions, and commands run.
