import { mkdirSync } from "node:fs"
import { dirname } from "node:path"
import { Database } from "bun:sqlite"
import { applySchema } from "./schema-migrate.ts"
import type {
  MessageInfo,
  MessageInfoUpdate,
  TokenEventRow,
  TokenEventSource,
  TokenStorage,
  ToolCallRow,
  TpsSampleRow,
  WriterResponse,
} from "./types.ts"

const DAY_MS = 24 * 60 * 60 * 1000

type InitMessage = {
  type: "init"
  dbPath: string
  retentionDays: number
}

type FlushMessage = {
  type: "flush"
  tokenRows: TokenEventRow[]
  tpsRows: TpsSampleRow[]
  infoUpdates: MessageInfoUpdate[]
  toolRows: ToolCallRow[]
}

type CloseMessage = {
  type: "close"
}

type WorkerMessage = InitMessage | FlushMessage | CloseMessage

function post(response: WriterResponse) {
  self.postMessage(response)
}

function pruneKey(now = Date.now()) {
  return new Date(now).toISOString().slice(0, 10)
}

async function createTokenStorage(dbPath: string, retentionDays: number): Promise<TokenStorage> {
  mkdirSync(dirname(dbPath), { recursive: true })

  const db = new Database(dbPath)
  db.exec("PRAGMA busy_timeout = 5000")
  const schemaSql = await Bun.file(new URL("../../schema/schema.sql", import.meta.url)).text()
  applySchema(db, schemaSql)
  try {
    db.exec("PRAGMA wal_checkpoint(PASSIVE)")
  } catch {
    // PASSIVE can fail if another writer holds the lock; safe to ignore
  }

  const insertEvent = db.prepare(`
    INSERT OR IGNORE INTO oc_token_events (
      recorded_at,
      recorded_at_ms,
      session_id,
      message_id,
      part_id,
      source,
      provider,
      model,
      input_tokens,
      output_tokens,
      reasoning_tokens,
      cache_read_tokens,
      cache_write_tokens,
      total_tokens
    ) VALUES (
      $recordedAt,
      $recordedAtMs,
      $sessionID,
      $messageID,
      $partID,
      $source,
      $provider,
      $model,
      $inputTokens,
      $outputTokens,
      $reasoningTokens,
      $cacheReadTokens,
      $cacheWriteTokens,
      $totalTokens
    )
  `)
  const deleteFallback = db.prepare(`
    DELETE FROM oc_token_events
    WHERE session_id = $sessionID
      AND message_id = $messageID
      AND source = 'message-fallback'
  `)
  const existingStepRow = db.prepare(`
    SELECT 1
    FROM oc_token_events
    WHERE session_id = $sessionID
      AND message_id = $messageID
      AND source = 'step-finish'
    LIMIT 1
  `)
  const updateEventInfo = db.prepare(`
    UPDATE oc_token_events
    SET provider = $provider,
        model = $model
    WHERE session_id = $sessionID
      AND message_id = $messageID
  `)
  const updateTpsInfo = db.prepare(`
    UPDATE oc_tps_samples
    SET provider = $provider,
        model = $model
    WHERE session_id = $sessionID
      AND message_id = $messageID
  `)
  const insertTpsSample = db.prepare(`
    INSERT OR IGNORE INTO oc_tps_samples (
      recorded_at,
      recorded_at_ms,
      session_id,
      message_id,
      provider,
      model,
      output_tokens,
      reasoning_tokens,
      total_tokens,
      duration_ms,
      ttft_ms,
      tokens_per_second
    ) VALUES (
      $recordedAt,
      $recordedAtMs,
      $sessionID,
      $messageID,
      $provider,
      $model,
      $outputTokens,
      $reasoningTokens,
      $totalTokens,
      $durationMs,
      $ttftMs,
      $tokensPerSecond
    )
  `)
  const insertToolCall = db.prepare(`
    INSERT OR IGNORE INTO oc_tool_calls (
      recorded_at,
      recorded_at_ms,
      session_id,
      message_id,
      tool_call_id,
      tool_name,
      provider,
      model,
      status
    ) VALUES (
      $recordedAt,
      $recordedAtMs,
      $sessionID,
      $messageID,
      $toolCallID,
      $toolName,
      $provider,
      $model,
      $status
    )
  `)
  const pruneEvents = db.prepare("DELETE FROM oc_token_events WHERE recorded_at_ms < $cutoff")
  const pruneTpsSamples = db.prepare("DELETE FROM oc_tps_samples WHERE recorded_at_ms < $cutoff")
  const pruneToolCalls = db.prepare("DELETE FROM oc_tool_calls WHERE recorded_at_ms < $cutoff")
  const insertRows = db.transaction((rows: TokenEventRow[]) => {
    for (const row of rows) {
      if (row.source === "step-finish") {
        deleteFallback.run({ $sessionID: row.sessionID, $messageID: row.messageID })
      } else if (existingStepRow.get({ $sessionID: row.sessionID, $messageID: row.messageID })) {
        continue
      }
      insertEvent.run({
        $recordedAt: row.recordedAt,
        $recordedAtMs: row.recordedAtMs,
        $sessionID: row.sessionID,
        $messageID: row.messageID,
        $partID: row.partID,
        $source: row.source,
        $provider: row.provider,
        $model: row.model,
        $inputTokens: row.inputTokens,
        $outputTokens: row.outputTokens,
        $reasoningTokens: row.reasoningTokens,
        $cacheReadTokens: row.cacheReadTokens,
        $cacheWriteTokens: row.cacheWriteTokens,
        $totalTokens: row.totalTokens,
      })
    }
  })
  const insertTpsRows = db.transaction((rows: TpsSampleRow[]) => {
    for (const row of rows) {
      insertTpsSample.run({
        $recordedAt: row.recordedAt,
        $recordedAtMs: row.recordedAtMs,
        $sessionID: row.sessionID,
        $messageID: row.messageID,
        $provider: row.provider,
        $model: row.model,
        $outputTokens: row.outputTokens,
        $reasoningTokens: row.reasoningTokens,
        $totalTokens: row.totalTokens,
        $durationMs: row.durationMs,
        $ttftMs: row.ttftMs,
        $tokensPerSecond: row.tokensPerSecond,
      })
    }
  })
  const insertToolRows = db.transaction((rows: ToolCallRow[]) => {
    for (const row of rows) {
      insertToolCall.run({
        $recordedAt: row.recordedAt,
        $recordedAtMs: row.recordedAtMs,
        $sessionID: row.sessionID,
        $messageID: row.messageID,
        $toolCallID: row.toolCallID,
        $toolName: row.toolName,
        $provider: row.provider,
        $model: row.model,
        $status: row.status,
      })
    }
  })
  const updateInfo = db.transaction((messageID: string, info: MessageInfo) => {
    updateEventInfo.run({
      $messageID: messageID,
      $sessionID: info.sessionID,
      $provider: info.provider,
      $model: info.model,
    })
    updateTpsInfo.run({
      $messageID: messageID,
      $sessionID: info.sessionID,
      $provider: info.provider,
      $model: info.model,
    })
  })

  let lastPruneKey = ""

  const pruneDaily = () => {
    if (retentionDays <= 0) return
    const key = pruneKey()
    if (key === lastPruneKey) return
    lastPruneKey = key
    const cutoff = Date.now() - retentionDays * DAY_MS
    pruneEvents.run({ $cutoff: cutoff })
    pruneTpsSamples.run({ $cutoff: cutoff })
    pruneToolCalls.run({ $cutoff: cutoff })
  }

  pruneDaily()

  return {
    flush(tokenRows, tpsRows, infoUpdates, toolRows) {
      if (tokenRows.length > 0) insertRows(tokenRows)
      if (tpsRows.length > 0) insertTpsRows(tpsRows)
      if (toolRows.length > 0) insertToolRows(toolRows)
      for (const update of infoUpdates) {
        updateInfo(update.messageID, update.info)
      }
      pruneDaily()
    },
    close() {
      db.close()
    },
  }
}

const WORKER_INIT_TIMEOUT_MS = 10_000

let storage: TokenStorage | undefined

function withTimeout<T>(promise: Promise<T>, ms: number): Promise<T> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`timed out after ${ms}ms`)), ms)
    promise.then(
      (val) => { clearTimeout(timer); resolve(val) },
      (err) => { clearTimeout(timer); reject(err) },
    )
  })
}

self.onmessage = async (event: MessageEvent<WorkerMessage>) => {
  try {
    const message = event.data
    if (message.type === "init") {
      storage = await withTimeout(createTokenStorage(message.dbPath, message.retentionDays), WORKER_INIT_TIMEOUT_MS)
      post({ type: "ready" })
      return
    }
    if (message.type === "flush") {
      storage?.flush(message.tokenRows, message.tpsRows, message.infoUpdates, message.toolRows)
      post({ type: "flushed" })
      return
    }
    storage?.close()
    post({ type: "closed" })
  } catch (error) {
    post({ type: "error", message: error instanceof Error ? error.message : "sqlite write failed" })
  }
}
