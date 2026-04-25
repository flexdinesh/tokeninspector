import { mkdirSync } from "node:fs"
import { dirname } from "node:path"
import { Database } from "bun:sqlite"

const DAY_MS = 24 * 60 * 60 * 1000

type TokenEventSource = "step-finish" | "message-fallback"

type MessageInfo = {
  sessionID: string
  provider: string
  model: string
}

type TokenEventRow = {
  recordedAt: string
  recordedAtMs: number
  sessionID: string
  messageID: string
  partID: string
  source: TokenEventSource
  provider: string
  model: string
  inputTokens: number
  outputTokens: number
  reasoningTokens: number
  cacheReadTokens: number
  cacheWriteTokens: number
  totalTokens: number
}

type TpsSampleRow = {
  recordedAt: string
  recordedAtMs: number
  sessionID: string
  messageID: string
  provider: string
  model: string
  outputTokens: number
  reasoningTokens: number
  totalTokens: number
  durationMs: number
  ttftMs: number
  tokensPerSecond: number
}

type MessageInfoUpdate = {
  messageID: string
  info: MessageInfo
}

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
}

type CloseMessage = {
  type: "close"
}

type WorkerMessage = InitMessage | FlushMessage | CloseMessage

type WriterResponse = {
  type: "ready" | "flushed" | "closed" | "error"
  message?: string
}

type TokenStorage = {
  flush: (tokenRows: TokenEventRow[], tpsRows: TpsSampleRow[], infoUpdates: MessageInfoUpdate[]) => void
  close: () => void
}

function post(response: WriterResponse) {
  self.postMessage(response)
}

function pruneKey(now = Date.now()) {
  return new Date(now).toISOString().slice(0, 10)
}

function createTokenStorage(dbPath: string, retentionDays: number): TokenStorage {
  mkdirSync(dirname(dbPath), { recursive: true })

  const db = new Database(dbPath)
  db.exec("PRAGMA journal_mode = WAL")
  db.exec("PRAGMA busy_timeout = 5000")
  db.exec(`
    CREATE TABLE IF NOT EXISTS oc_token_events (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      recorded_at TEXT NOT NULL,
      recorded_at_ms INTEGER NOT NULL,
      session_id TEXT NOT NULL,
      message_id TEXT NOT NULL,
      part_id TEXT NOT NULL,
      source TEXT NOT NULL CHECK (source IN ('step-finish', 'message-fallback')),
      provider TEXT NOT NULL DEFAULT 'unknown',
      model TEXT NOT NULL DEFAULT 'unknown',
      input_tokens INTEGER NOT NULL,
      output_tokens INTEGER NOT NULL,
      reasoning_tokens INTEGER NOT NULL,
      cache_read_tokens INTEGER NOT NULL,
      cache_write_tokens INTEGER NOT NULL,
      total_tokens INTEGER NOT NULL,
      UNIQUE(session_id, message_id, part_id)
    )
  `)
  db.exec("CREATE INDEX IF NOT EXISTS oc_token_events_recorded_at_ms_idx ON oc_token_events (recorded_at_ms)")
  db.exec("CREATE INDEX IF NOT EXISTS oc_token_events_session_time_idx ON oc_token_events (session_id, recorded_at_ms)")
  db.exec(
    "CREATE INDEX IF NOT EXISTS oc_token_events_provider_model_time_idx ON oc_token_events (provider, model, recorded_at_ms)",
  )
  db.exec("PRAGMA user_version = 1")
  db.exec(`
    CREATE TABLE IF NOT EXISTS oc_tps_samples (
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
    )
  `)
  db.exec("CREATE INDEX IF NOT EXISTS oc_tps_samples_recorded_at_ms_idx ON oc_tps_samples (recorded_at_ms)")
  db.exec("CREATE INDEX IF NOT EXISTS oc_tps_samples_session_time_idx ON oc_tps_samples (session_id, recorded_at_ms)")
  db.exec(
    "CREATE INDEX IF NOT EXISTS oc_tps_samples_provider_model_time_idx ON oc_tps_samples (provider, model, recorded_at_ms)",
  )

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
  const pruneEvents = db.prepare("DELETE FROM oc_token_events WHERE recorded_at_ms < $cutoff")
  const pruneTpsSamples = db.prepare("DELETE FROM oc_tps_samples WHERE recorded_at_ms < $cutoff")
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
  }

  pruneDaily()

  return {
    flush(tokenRows, tpsRows, infoUpdates) {
      if (tokenRows.length > 0) insertRows(tokenRows)
      if (tpsRows.length > 0) insertTpsRows(tpsRows)
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

let storage: TokenStorage | undefined

self.onmessage = (event: MessageEvent<WorkerMessage>) => {
  try {
    const message = event.data
    if (message.type === "init") {
      storage = createTokenStorage(message.dbPath, message.retentionDays)
      post({ type: "ready" })
      return
    }
    if (message.type === "flush") {
      storage?.flush(message.tokenRows, message.tpsRows, message.infoUpdates)
      post({ type: "flushed" })
      return
    }
    storage?.close()
    post({ type: "closed" })
  } catch (error) {
    post({ type: "error", message: error instanceof Error ? error.message : "sqlite write failed" })
  }
}
