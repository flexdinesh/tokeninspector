import { mkdirSync } from "node:fs"
import { dirname, isAbsolute, join } from "node:path"
import type { ExtensionAPI } from "@mariozechner/pi-coding-agent"
import Database from "better-sqlite3"

type DB = InstanceType<typeof Database>

// ── Schema (inline so the extension is self-contained) ────────────────────────

const PI_SCHEMA = `
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS pi_token_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  recorded_at TEXT NOT NULL,
  recorded_at_ms INTEGER NOT NULL,
  session_id TEXT NOT NULL,
  message_id TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT 'unknown',
  model TEXT NOT NULL DEFAULT 'unknown',
  input_tokens INTEGER NOT NULL,
  output_tokens INTEGER NOT NULL,
  reasoning_tokens INTEGER NOT NULL,
  cache_read_tokens INTEGER NOT NULL,
  cache_write_tokens INTEGER NOT NULL,
  total_tokens INTEGER NOT NULL,
  UNIQUE(session_id, message_id)
);

CREATE INDEX IF NOT EXISTS pi_token_events_recorded_at_ms_idx ON pi_token_events (recorded_at_ms);
CREATE INDEX IF NOT EXISTS pi_token_events_session_time_idx ON pi_token_events (session_id, recorded_at_ms);
CREATE INDEX IF NOT EXISTS pi_token_events_provider_model_time_idx ON pi_token_events (provider, model, recorded_at_ms);

CREATE TABLE IF NOT EXISTS pi_tps_samples (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  recorded_at TEXT NOT NULL,
  recorded_at_ms INTEGER NOT NULL,
  session_id TEXT NOT NULL,
  message_id TEXT NOT NULL UNIQUE,
  provider TEXT NOT NULL DEFAULT 'unknown',
  model TEXT NOT NULL DEFAULT 'unknown',
  output_tokens INTEGER NOT NULL,
  reasoning_tokens INTEGER NOT NULL,
  total_tokens INTEGER NOT NULL,
  duration_ms INTEGER NOT NULL,
  ttft_ms INTEGER NOT NULL,
  tokens_per_second REAL NOT NULL
);

CREATE INDEX IF NOT EXISTS pi_tps_samples_recorded_at_ms_idx ON pi_tps_samples (recorded_at_ms);
CREATE INDEX IF NOT EXISTS pi_tps_samples_session_time_idx ON pi_tps_samples (session_id, recorded_at_ms);
CREATE INDEX IF NOT EXISTS pi_tps_samples_provider_model_time_idx ON pi_tps_samples (provider, model, recorded_at_ms);

CREATE TABLE IF NOT EXISTS pi_llm_requests (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  recorded_at TEXT NOT NULL,
  recorded_at_ms INTEGER NOT NULL,
  session_id TEXT NOT NULL,
  message_id TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT 'unknown',
  model TEXT NOT NULL DEFAULT 'unknown',
  attempt_index INTEGER NOT NULL CHECK (attempt_index > 0),
  thinking_level TEXT NOT NULL DEFAULT 'unknown'
);

CREATE INDEX IF NOT EXISTS pi_llm_requests_recorded_at_ms_idx ON pi_llm_requests (recorded_at_ms);
CREATE INDEX IF NOT EXISTS pi_llm_requests_session_time_idx ON pi_llm_requests (session_id, recorded_at_ms);
CREATE INDEX IF NOT EXISTS pi_llm_requests_provider_model_time_idx ON pi_llm_requests (provider, model, recorded_at_ms);
`;

// ── Config ──────────────────────────────────────────────────────────────────

const DEFAULT_DB_NAME = "oc-tps.sqlite"
const DEFAULT_RETENTION_DAYS = 365
const DAY_MS = 24 * 60 * 60 * 1000
const UNKNOWN_VALUE = "unknown"

function defaultStatePath() {
  const xdgStateHome = process.env.XDG_STATE_HOME?.trim()
  if (xdgStateHome && xdgStateHome.length > 0) return join(xdgStateHome, "opencode")

  const home = process.env.HOME?.trim()
  if (home && home.length > 0) return join(home, ".local", "state", "opencode")

  return join(process.cwd(), ".opencode-state")
}

function dbPath() {
  const configured = process.env.PI_TOKENINSPECTOR_DB_PATH?.trim() || process.env.OC_TOKENINSPECTOR_DB_PATH?.trim()
  if (!configured) return join(defaultStatePath(), DEFAULT_DB_NAME)
  return isAbsolute(configured) ? configured : join(defaultStatePath(), configured)
}

function retentionDays() {
  const configured = process.env.PI_TOKENINSPECTOR_RETENTION_DAYS?.trim()
  if (!configured) return DEFAULT_RETENTION_DAYS

  const parsed = Number(configured)
  return Number.isFinite(parsed) ? parsed : DEFAULT_RETENTION_DAYS
}

// ── Utilities ───────────────────────────────────────────────────────────────

function knownValue(value: string | undefined): string {
  const trimmed = value?.trim()
  return trimmed && trimmed.length > 0 ? trimmed : UNKNOWN_VALUE
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}

function recordValue(value: unknown, key: string): unknown | undefined {
  if (!isRecord(value)) return undefined
  if (!Object.prototype.hasOwnProperty.call(value, key)) return undefined
  return value[key]
}

function normalizedThinkingLevel(value: unknown): string | undefined {
  if (typeof value === "string") {
    const normalized = value.trim().toLowerCase().replace(/[-_ ]/g, "")
    if (normalized === "low") return "low"
    if (normalized === "medium") return "medium"
    if (normalized === "high") return "high"
    if (normalized === "xhigh" || normalized === "extrahigh") return "xhigh"
    return undefined
  }
  if (!isRecord(value)) return undefined

  return (
    normalizedThinkingLevel(recordValue(value, "level")) ??
    normalizedThinkingLevel(recordValue(value, "effort")) ??
    normalizedThinkingLevel(recordValue(value, "reasoningEffort"))
  )
}

function thinkingLevelFromPayload(payload: unknown): string {
  if (!isRecord(payload)) return UNKNOWN_VALUE

  return (
    normalizedThinkingLevel(recordValue(payload, "thinking")) ??
    normalizedThinkingLevel(recordValue(payload, "thinkingLevel")) ??
    normalizedThinkingLevel(recordValue(payload, "reasoning")) ??
    normalizedThinkingLevel(recordValue(payload, "reasoningEffort")) ??
    UNKNOWN_VALUE
  )
}

// ── Session state ───────────────────────────────────────────────────────────

interface SessionState {
  turnSeq: number
  currentTurnId: string
  requestStartAt: number
  firstTokenAt?: number
  lastTokenAt?: number
  thinkingLevel: string
  provider: string
  model: string
}

const sessionStates = new Map<string, SessionState>()

function getOrCreateState(sessionId: string): SessionState {
  let state = sessionStates.get(sessionId)
  if (!state) {
    state = {
      turnSeq: 0,
      currentTurnId: "",
      requestStartAt: 0,
      firstTokenAt: undefined,
      lastTokenAt: undefined,
      thinkingLevel: UNKNOWN_VALUE,
      provider: UNKNOWN_VALUE,
      model: UNKNOWN_VALUE,
    }
    sessionStates.set(sessionId, state)
  }
  return state
}

// ── DB layer ────────────────────────────────────────────────────────────────

let db: DB | undefined
let dbInitFailed = false
let insertTokenEvent: ReturnType<DB["prepare"]> | undefined
let insertTpsSample: ReturnType<DB["prepare"]> | undefined
let insertRequest: ReturnType<DB["prepare"]> | undefined
let pruneTokenEvents: ReturnType<DB["prepare"]> | undefined
let pruneTpsSamples: ReturnType<DB["prepare"]> | undefined
let pruneRequests: ReturnType<DB["prepare"]> | undefined
let lastPruneKey = ""

function initDb(): boolean {
  if (db) return true
  if (dbInitFailed) return false

  try {
    const path = dbPath()
    mkdirSync(dirname(path), { recursive: true })

    db = new Database(path)
    db.exec(PI_SCHEMA)

    insertTokenEvent = db.prepare(`
      INSERT OR IGNORE INTO pi_token_events (
        recorded_at, recorded_at_ms, session_id, message_id,
        provider, model,
        input_tokens, output_tokens, reasoning_tokens,
        cache_read_tokens, cache_write_tokens, total_tokens
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `)

    insertTpsSample = db.prepare(`
      INSERT OR IGNORE INTO pi_tps_samples (
        recorded_at, recorded_at_ms, session_id, message_id,
        provider, model,
        output_tokens, reasoning_tokens, total_tokens,
        duration_ms, ttft_ms, tokens_per_second
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `)

    insertRequest = db.prepare(`
      INSERT INTO pi_llm_requests (
        recorded_at, recorded_at_ms, session_id, message_id,
        provider, model, attempt_index, thinking_level
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `)

    pruneTokenEvents = db.prepare("DELETE FROM pi_token_events WHERE recorded_at_ms < ?")
    pruneTpsSamples = db.prepare("DELETE FROM pi_tps_samples WHERE recorded_at_ms < ?")
    pruneRequests = db.prepare("DELETE FROM pi_llm_requests WHERE recorded_at_ms < ?")

    return true
  } catch (err) {
    dbInitFailed = true
    console.error("pi-tokeninspector: db init failed, tracking disabled:", err)
    return false
  }
}

function pruneDaily() {
  const retention = retentionDays()
  if (retention <= 0) return

  const key = new Date().toISOString().slice(0, 10)
  if (key === lastPruneKey) return
  lastPruneKey = key

  const cutoff = Date.now() - retention * DAY_MS
  pruneTokenEvents?.run(cutoff)
  pruneTpsSamples?.run(cutoff)
  pruneRequests?.run(cutoff)
}

// ── Event handlers ───────────────────────────────────────────────────────────

export default function (pi: ExtensionAPI) {
  pi.on("session_start", async (event, ctx) => {
    const sessionId = ctx.sessionManager.getSessionId()
    if (sessionId) {
      getOrCreateState(sessionId)
    }
  })

  pi.on("turn_start", async (event, _ctx) => {
    if (!initDb()) return
    pruneDaily()

    // We don't have sessionId on turn_start event directly, so we skip here.
    // Timing is captured on before_provider_request where ctx is available.
  })

  pi.on("before_provider_request", async (event, ctx) => {
    if (!initDb()) return

    const sessionId = ctx.sessionManager.getSessionId()
    if (!sessionId) return

    const state = getOrCreateState(sessionId)
    state.requestStartAt = Date.now()
    state.firstTokenAt = undefined
    state.lastTokenAt = undefined

    // Extract thinking level from payload
    state.thinkingLevel = thinkingLevelFromPayload(event.payload)

    // Provider / model from ctx.model
    const currentModel = ctx.model
    state.provider = knownValue(currentModel?.provider)
    state.model = knownValue(currentModel?.id)

    // Record request attempt
    state.turnSeq++
    state.currentTurnId = String(state.turnSeq)
    const recordedAtMs = Date.now()

    try {
      insertRequest?.run([
        new Date(recordedAtMs).toISOString(),
        recordedAtMs,
        sessionId,
        state.currentTurnId,
        state.provider,
        state.model,
        1, // Pi doesn't expose retry detection; all attempts are 1
        state.thinkingLevel,
      ])
    } catch (err) {
      console.error("pi-tokeninspector: request insert failed:", err)
    }
  })

  pi.on("message_update", async (event, ctx) => {
    if (!initDb()) return

    const msg = event.message
    if (msg.role !== "assistant") return

    const sessionId = ctx.sessionManager.getSessionId()
    if (!sessionId) return

    const state = getOrCreateState(sessionId)
    const now = Date.now()

    if (state.firstTokenAt === undefined) {
      state.firstTokenAt = now
    }
    state.lastTokenAt = now
  })

  pi.on("message_end", async (event, ctx) => {
    if (!initDb()) return

    const msg = event.message
    if (msg.role !== "assistant") return

    const sessionId = ctx.sessionManager.getSessionId()
    if (!sessionId) return

    const state = getOrCreateState(sessionId)
    const messageId = state.currentTurnId || String(state.turnSeq)

    // Prefer message-level provider/model if available
    const provider = knownValue(msg.provider) || state.provider
    const model = knownValue(msg.model) || state.model
    state.provider = provider
    state.model = model

    const usage = (msg as any).usage
    if (!usage) return

    const recordedAtMs = Date.now()
    const recordedAt = new Date(recordedAtMs).toISOString()

    const inputTokens = Number(usage.input) || 0
    const outputTokens = Number(usage.output) || 0
    const cacheRead = Number(usage.cacheRead) || 0
    const cacheWrite = Number(usage.cacheWrite) || 0
    const totalTokens = Number(usage.totalTokens) || (inputTokens + outputTokens + cacheRead + cacheWrite)
    const reasoningTokens = 0 // Pi does not expose reasoning tokens separately

    // Write token event
    try {
      insertTokenEvent?.run([
        recordedAt, recordedAtMs, sessionId, messageId,
        provider, model,
        inputTokens, outputTokens, reasoningTokens,
        cacheRead, cacheWrite, totalTokens,
      ])
    } catch (err) {
      console.error("pi-tokeninspector: token event insert failed:", err)
    }

    // Compute TPS
    const throughputTokens = outputTokens + reasoningTokens
    const ttftMs = state.firstTokenAt !== undefined
      ? Math.max(state.firstTokenAt - state.requestStartAt, 0)
      : 0
    const endAt = state.lastTokenAt ?? recordedAtMs
    const durationMs = Math.max(endAt - state.requestStartAt, 1)
    const tps = throughputTokens / (durationMs / 1000)

    // Write TPS sample
    try {
      insertTpsSample?.run([
        recordedAt, recordedAtMs, sessionId, messageId,
        provider, model,
        outputTokens, reasoningTokens, throughputTokens,
        durationMs, ttftMs, tps,
      ])
    } catch (err) {
      console.error("pi-tokeninspector: tps sample insert failed:", err)
    }

    pruneDaily()
  })

  pi.on("session_shutdown", async (event, _ctx) => {
    // Note: we don't have sessionId here directly.
    // Pi rebinds extensions on session switch, so state is naturally reset.
    // If we ever need explicit cleanup, we can iterate sessionStates.
    db?.close()
    db = undefined
    insertTokenEvent = undefined
    insertTpsSample = undefined
    insertRequest = undefined
    pruneTokenEvents = undefined
    pruneTpsSamples = undefined
    pruneRequests = undefined
    dbInitFailed = false
    lastPruneKey = ""
    sessionStates.clear()
  })
}
