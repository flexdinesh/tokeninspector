import { mkdirSync } from "node:fs"
import { dirname, isAbsolute, join } from "node:path"
import { Database } from "bun:sqlite"
import type { Plugin } from "@opencode-ai/plugin"
import { applySchema } from "./schema-migrate.ts"
import type { RequestRow, RequestStorage } from "./types.ts"

const DEFAULT_DB_NAME = "oc-tps.sqlite"
const DEFAULT_RETENTION_DAYS = 365
const DAY_MS = 24 * 60 * 60 * 1000
const UNKNOWN_VALUE = "unknown"

function knownValue(value: string | undefined) {
  const trimmed = value?.trim()
  return trimmed && trimmed.length > 0 ? trimmed : UNKNOWN_VALUE
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

export const OcTokenInspectorServer: Plugin = async () => {
  let storage: RequestStorage | undefined
  let storageInitPromise: Promise<RequestStorage | undefined> | undefined
  let storageInitFailed = false
  const attemptsByKey: Record<string, number> = {}
  const thinkingLevelByKey: Record<string, string> = {}

  async function getStorage(): Promise<RequestStorage | undefined> {
    if (storage) return storage
    if (storageInitFailed) return undefined
    if (storageInitPromise) return storageInitPromise

    storageInitPromise = createRequestStorage(dbPath(), retentionDays())
      .then((s) => {
        storage = s
        return s
      })
      .catch((err) => {
        storageInitFailed = true
        console.error("oc-tokeninspector-server: db init failed, request tracking disabled:", err)
        return undefined
      })

    return storageInitPromise
  }

  return {
    "chat.params": async (chatInput, output) => {
      thinkingLevelByKey[attemptKey(chatInput)] = thinkingLevelFromOptions(output.options)
    },
    "chat.headers": async (chatInput) => {
      const s = await getStorage()
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
        console.error("oc-tokeninspector-server: insert failed:", err)
      }
    },
    event: async ({ event }) => {
      if (event.type !== "session.idle" && event.type !== "session.deleted") return
      for (const key of Object.keys(attemptsByKey)) {
        if (key.startsWith(`${event.properties.sessionID}\u0000`)) delete attemptsByKey[key]
      }
      for (const key of Object.keys(thinkingLevelByKey)) {
        if (key.startsWith(`${event.properties.sessionID}\u0000`)) delete thinkingLevelByKey[key]
      }
    },
  }
}

export default OcTokenInspectorServer
