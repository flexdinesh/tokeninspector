import { mkdirSync } from "node:fs"
import { dirname, isAbsolute, join } from "node:path"
import { Database } from "bun:sqlite"
import type { Plugin } from "@opencode-ai/plugin"
import { createTokenStorage } from "./writer-client.ts"
import { applySchema } from "./schema-migrate.ts"
import type {
  MessageInfo,
  MessageInfoUpdate,
  MessageTiming,
  RequestRow,
  RequestStorage,
  TokenCounts,
  TokenEventRow,
  TokenEventSource,
  TokenStorage,
  TpsSampleRow,
} from "./types.ts"

const DEFAULT_DB_NAME = "oc-tps.sqlite"
const DEFAULT_RETENTION_DAYS = 365
const DAY_MS = 24 * 60 * 60 * 1000
const UNKNOWN_VALUE = "unknown"

function knownValue(value: string | undefined) {
  const trimmed = value?.trim()
  return trimmed && trimmed.length > 0 ? trimmed : UNKNOWN_VALUE
}

function messageKey(sessionID: string, messageID: string) {
  return `${sessionID}:${messageID}`
}

function totalTokenCount(tokens: TokenCounts) {
  return tokens.total ?? tokens.input + tokens.output + tokens.reasoning + tokens.cache.read + tokens.cache.write
}

function tokenEventRow(input: {
  recordedAtMs: number
  sessionID: string
  messageID: string
  partID: string
  source: TokenEventSource
  provider: string | undefined
  model: string | undefined
  tokens: TokenCounts
}): TokenEventRow {
  return {
    recordedAt: new Date(input.recordedAtMs).toISOString(),
    recordedAtMs: input.recordedAtMs,
    sessionID: input.sessionID,
    messageID: input.messageID,
    partID: input.partID,
    source: input.source,
    provider: knownValue(input.provider),
    model: knownValue(input.model),
    inputTokens: input.tokens.input,
    outputTokens: input.tokens.output,
    reasoningTokens: input.tokens.reasoning,
    cacheReadTokens: input.tokens.cache.read,
    cacheWriteTokens: input.tokens.cache.write,
    totalTokens: totalTokenCount(input.tokens),
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}

function recordValue(value: unknown, key: string) {
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

function thinkingLevelFromOptions(options: unknown) {
  return (
    normalizedThinkingLevel(recordValue(options, "thinking")) ??
    normalizedThinkingLevel(recordValue(options, "thinkingLevel")) ??
    normalizedThinkingLevel(recordValue(options, "reasoning")) ??
    normalizedThinkingLevel(recordValue(options, "reasoningEffort")) ??
    UNKNOWN_VALUE
  )
}

function defaultStatePath() {
  const xdgStateHome = process.env.XDG_STATE_HOME?.trim()
  if (xdgStateHome && xdgStateHome.length > 0) return join(xdgStateHome, "opencode")

  const home = process.env.HOME?.trim()
  if (home && home.length > 0) return join(home, ".local", "state", "opencode")

  return join(process.cwd(), ".opencode-state")
}

function dbPath() {
  const configured = process.env.OC_TOKENINSPECTOR_DB_PATH?.trim()
  if (!configured) return join(defaultStatePath(), DEFAULT_DB_NAME)
  return isAbsolute(configured) ? configured : join(defaultStatePath(), configured)
}

function retentionDays() {
  const configured = process.env.OC_TOKENINSPECTOR_RETENTION_DAYS?.trim()
  if (!configured) return DEFAULT_RETENTION_DAYS

  const parsed = Number(configured)
  return Number.isFinite(parsed) ? parsed : DEFAULT_RETENTION_DAYS
}

async function createRequestStorage(path: string, retention: number): Promise<RequestStorage> {
  mkdirSync(dirname(path), { recursive: true })

  const db = new Database(path)
  const schemaSql = await Bun.file(new URL("../schema/schema.sql", import.meta.url)).text()
  applySchema(db, schemaSql)

  const insertRequest = db.prepare(`
    INSERT INTO oc_llm_requests (
      recorded_at,
      recorded_at_ms,
      session_id,
      message_id,
      provider,
      model,
      attempt_index,
      thinking_level
    ) VALUES (
      $recordedAt,
      $recordedAtMs,
      $sessionID,
      $messageID,
      $provider,
      $model,
      $attemptIndex,
      $thinkingLevel
    )
  `)
  const pruneRequests = db.prepare("DELETE FROM oc_llm_requests WHERE recorded_at_ms < $cutoff")

  return {
    insert(row) {
      insertRequest.run({
        $recordedAt: row.recordedAt,
        $recordedAtMs: row.recordedAtMs,
        $sessionID: row.sessionID,
        $messageID: row.messageID,
        $provider: row.provider,
        $model: row.model,
        $attemptIndex: row.attemptIndex,
        $thinkingLevel: row.thinkingLevel,
      })
      if (retention > 0) {
        pruneRequests.run({ $cutoff: Date.now() - retention * DAY_MS })
      }
    },
    close() {
      db.close()
    },
  }
}

function attemptKey(input: { sessionID: string; message: { id?: string }; provider: { id?: string }; model: { id?: string } }) {
  return [input.sessionID, knownValue(input.message.id), knownValue(input.provider.id), knownValue(input.model.id)].join("\u0000")
}

type ServerMessageInfo = {
  id: string
  role: string
  completedAt?: number
  providerID?: string
  modelID?: string
  tokens?: TokenCounts
}

export const OcTokenInspectorServer: Plugin = async () => {
  // --- LLM request tracking (direct DB) ---
  let requestStorage: RequestStorage | undefined
  let requestInitPromise: Promise<RequestStorage | undefined> | undefined
  let requestInitFailed = false
  const attemptsByKey: Record<string, number> = {}
  const thinkingLevelByKey: Record<string, string> = {}

  async function getRequestStorage(): Promise<RequestStorage | undefined> {
    if (requestStorage) return requestStorage
    if (requestInitFailed) return undefined
    if (requestInitPromise) return requestInitPromise

    requestInitPromise = createRequestStorage(dbPath(), retentionDays())
      .then((s) => {
        requestStorage = s
        return s
      })
      .catch((err) => {
        requestInitFailed = true
        console.error("oc-tokeninspector-server: request db init failed:", err)
        return undefined
      })

    return requestInitPromise
  }

  // --- Token / TPS tracking (worker) ---
  let tokenStorage: TokenStorage | undefined
  let tokenInitFailed = false

  function getTokenStorage(): TokenStorage | undefined {
    if (tokenStorage) return tokenStorage
    if (tokenInitFailed) return undefined
    try {
      tokenStorage = createTokenStorage(
        new URL("./oc-tokeninspector-writer.ts", import.meta.url),
        { dbPath: dbPath(), retentionDays: retentionDays() },
        () => {
          console.error("oc-tokeninspector-server: token worker error")
        },
      )
      return tokenStorage
    } catch (err) {
      tokenInitFailed = true
      console.error("oc-tokeninspector-server: token worker init failed:", err)
      return undefined
    }
  }

  // --- In-memory state for durable collection ---
  let pendingRows: TokenEventRow[] = []
  let pendingTpsRows: TpsSampleRow[] = []
  let pendingInfoUpdates: MessageInfoUpdate[] = []
  const messageInfoByID: Record<string, MessageInfo> = {}
  const messagesWithStepRows = new Set<string>()
  const messageTimingByID: Record<string, MessageTiming> = {}
  const serverMessagesBySession: Record<string, ServerMessageInfo[]> = {}

  const flushRows = (sessionID?: string) => {
    const storage = getTokenStorage()
    if (!storage) return
    const rows = sessionID ? pendingRows.filter((row) => row.sessionID === sessionID) : pendingRows
    const tpsRows = sessionID ? pendingTpsRows.filter((row) => row.sessionID === sessionID) : pendingTpsRows
    const infoUpdates = sessionID
      ? pendingInfoUpdates.filter((update) => update.info.sessionID === sessionID)
      : pendingInfoUpdates
    if (rows.length === 0 && tpsRows.length === 0 && infoUpdates.length === 0) return

    try {
      storage.flush(rows, tpsRows, infoUpdates)
      pendingRows = sessionID ? pendingRows.filter((row) => row.sessionID !== sessionID) : []
      pendingTpsRows = sessionID ? pendingTpsRows.filter((row) => row.sessionID !== sessionID) : []
      pendingInfoUpdates = sessionID
        ? pendingInfoUpdates.filter((update) => update.info.sessionID !== sessionID)
        : []
    } catch (err) {
      console.error("oc-tokeninspector-server: flush failed:", err)
    }
  }

  const updateMessageInfo = (messageID: string, info: MessageInfo) => {
    messageInfoByID[messageID] = info
    pendingRows = pendingRows.map((row) =>
      row.sessionID === info.sessionID && row.messageID === messageID
        ? { ...row, provider: info.provider, model: info.model }
        : row,
    )
    pendingTpsRows = pendingTpsRows.map((row) =>
      row.sessionID === info.sessionID && row.messageID === messageID
        ? { ...row, provider: info.provider, model: info.model }
        : row,
    )
    pendingInfoUpdates = [
      ...pendingInfoUpdates.filter((update) => update.info.sessionID !== info.sessionID || update.messageID !== messageID),
      { messageID, info },
    ]
  }

  const queueTokenEvent = (row: TokenEventRow) => {
    if (row.totalTokens <= 0) return
    if (row.source === "step-finish") {
      messagesWithStepRows.add(messageKey(row.sessionID, row.messageID))
      pendingRows = pendingRows.filter(
        (item) =>
          item.sessionID !== row.sessionID ||
          item.messageID !== row.messageID ||
          (item.source !== "message-fallback" && item.partID !== row.partID),
      )
    }
    pendingRows = [...pendingRows, row]
  }

  const queueFallbackEvent = (row: TokenEventRow) => {
    if (messagesWithStepRows.has(messageKey(row.sessionID, row.messageID))) return
    pendingRows = [
      ...pendingRows.filter(
        (item) => item.sessionID !== row.sessionID || item.messageID !== row.messageID || item.source !== "message-fallback",
      ),
      row,
    ]
  }

  const queueTpsSample = (row: TpsSampleRow) => {
    if (row.totalTokens <= 0 || row.durationMs <= 0) return
    pendingTpsRows = [
      ...pendingTpsRows.filter((item) => item.sessionID !== row.sessionID || item.messageID !== row.messageID),
      row,
    ]
  }

  const queueSessionFallbacks = (sessionID: string) => {
    const messages = serverMessagesBySession[sessionID] ?? []
    for (const message of messages) {
      if (message.role !== "assistant") continue
      if (typeof message.completedAt !== "number") continue
      if (messagesWithStepRows.has(messageKey(sessionID, message.id))) continue
      const info = {
        sessionID,
        provider: knownValue(message.providerID),
        model: knownValue(message.modelID),
      }
      updateMessageInfo(message.id, info)
      queueFallbackEvent(
        tokenEventRow({
          recordedAtMs: message.completedAt,
          sessionID,
          messageID: message.id,
          partID: `message:${message.id}`,
          source: "message-fallback",
          provider: info.provider,
          model: info.model,
          tokens: message.tokens ?? { input: 0, output: 0, reasoning: 0, cache: { read: 0, write: 0 } },
        }),
      )
    }
  }

  const timer = setInterval(() => {
    flushRows()
  }, 1000)

  return {
    "chat.params": async (chatInput, output) => {
      thinkingLevelByKey[attemptKey(chatInput)] = thinkingLevelFromOptions(output.options)
    },
    "chat.headers": async (chatInput) => {
      const s = await getRequestStorage()
      if (!s) return

      const key = attemptKey(chatInput)
      const attemptIndex = (attemptsByKey[key] ?? 0) + 1
      attemptsByKey[key] = attemptIndex

      const recordedAtMs = Date.now()
      try {
        s.insert({
          recordedAt: new Date(recordedAtMs).toISOString(),
          recordedAtMs,
          sessionID: chatInput.sessionID,
          messageID: knownValue(chatInput.message.id),
          provider: knownValue(chatInput.provider.id),
          model: knownValue(chatInput.model.id),
          attemptIndex,
          thinkingLevel: thinkingLevelByKey[key] ?? UNKNOWN_VALUE,
        })
      } catch (err) {
        console.error("oc-tokeninspector-server: request insert failed:", err)
      }
    },
    event: async ({ event }) => {
      switch (event.type) {
        case "message.updated": {
          const info = event.properties.info
          const sessionMessages = serverMessagesBySession[event.properties.sessionID] ?? []
          const existingIndex = sessionMessages.findIndex((m) => m.id === info.id)
          const messageData: ServerMessageInfo = {
            id: info.id,
            role: info.role,
            completedAt: info.time.completed,
            providerID: info.providerID,
            modelID: info.modelID,
            tokens: info.tokens,
          }
          if (existingIndex >= 0) {
            sessionMessages[existingIndex] = messageData
          } else {
            sessionMessages.push(messageData)
          }
          serverMessagesBySession[event.properties.sessionID] = sessionMessages

          if (info.role !== "assistant") return

          const messageInfo = {
            sessionID: event.properties.sessionID,
            provider: knownValue(info.providerID),
            model: knownValue(info.modelID),
          }
          updateMessageInfo(info.id, messageInfo)

          const completedAt = info.time.completed
          if (typeof completedAt !== "number") {
            const existing = messageTimingByID[info.id]
            messageTimingByID[info.id] = {
              sessionID: event.properties.sessionID,
              requestStartAt: info.time.created,
              firstResponseAt: existing?.firstResponseAt,
              firstTokenAt: existing?.firstTokenAt,
              lastTokenAt: existing?.lastTokenAt,
              lastToolCallAt: existing?.lastToolCallAt,
            }
            return
          }

          const timing = messageTimingByID[info.id]
          if (timing?.sessionID === event.properties.sessionID && typeof timing.firstResponseAt === "number") {
            const totalTokens = (info.tokens?.output ?? 0) + (info.tokens?.reasoning ?? 0)
            const endAt =
              info.finish === "tool-calls"
                ? timing.lastToolCallAt
                : completedAt
            const durationMs = typeof endAt === "number" ? Math.max(endAt - timing.firstResponseAt, 1) : undefined
            const ttftMs = Math.max(timing.firstResponseAt - timing.requestStartAt, 0)
            if (totalTokens > 0 && durationMs) {
              queueTpsSample({
                recordedAt: new Date(completedAt).toISOString(),
                recordedAtMs: completedAt,
                sessionID: event.properties.sessionID,
                messageID: info.id,
                provider: messageInfo.provider,
                model: messageInfo.model,
                outputTokens: info.tokens?.output ?? 0,
                reasoningTokens: info.tokens?.reasoning ?? 0,
                totalTokens,
                durationMs,
                ttftMs,
                tokensPerSecond: totalTokens / (durationMs / 1000),
              })
            }
          }
          queueFallbackEvent(
            tokenEventRow({
              recordedAtMs: completedAt,
              sessionID: event.properties.sessionID,
              messageID: info.id,
              partID: `message:${info.id}`,
              source: "message-fallback",
              provider: messageInfo.provider,
              model: messageInfo.model,
              tokens: info.tokens ?? { input: 0, output: 0, reasoning: 0, cache: { read: 0, write: 0 } },
            }),
          )
          delete messageTimingByID[info.id]
          break
        }

        case "message.part.updated": {
          if (event.properties.part.type === "step-finish") {
            const partInfo = messageInfoByID[event.properties.part.messageID]
            queueTokenEvent(
              tokenEventRow({
                recordedAtMs: event.properties.time,
                sessionID: event.properties.sessionID,
                messageID: event.properties.part.messageID,
                partID: event.properties.part.id,
                source: "step-finish",
                provider: partInfo?.provider,
                model: partInfo?.model,
                tokens: event.properties.part.tokens,
              }),
            )
            return
          }
          if (event.properties.part.type !== "tool") return
          const timing = messageTimingByID[event.properties.part.messageID]
          if (!timing) return
          if (event.properties.part.state.status === "pending") {
            messageTimingByID[event.properties.part.messageID] = {
              ...timing,
              firstResponseAt: timing.firstResponseAt ?? event.properties.time,
            }
            return
          }
          if (event.properties.part.state.status !== "running") return
          messageTimingByID[event.properties.part.messageID] = {
            ...timing,
            lastToolCallAt: event.properties.part.state.time.start,
          }
          break
        }

        case "session.idle": {
          queueSessionFallbacks(event.properties.sessionID)
          flushRows(event.properties.sessionID)
          break
        }

        case "session.deleted": {
          queueSessionFallbacks(event.properties.sessionID)
          flushRows(event.properties.sessionID)
          delete serverMessagesBySession[event.properties.sessionID]
          for (const key of Object.keys(attemptsByKey)) {
            if (key.startsWith(`${event.properties.sessionID}\u0000`)) delete attemptsByKey[key]
          }
          for (const key of Object.keys(thinkingLevelByKey)) {
            if (key.startsWith(`${event.properties.sessionID}\u0000`)) delete thinkingLevelByKey[key]
          }
          break
        }
      }
    },
  }
}

export default OcTokenInspectorServer
