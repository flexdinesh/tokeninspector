/** @jsxImportSource @opentui/solid */
import { isAbsolute, join } from "node:path"
import type { TuiPlugin, TuiPluginModule } from "@opencode-ai/plugin/tui"
import { createMemo, createSignal } from "solid-js"
import type {
  MessageInfo,
  MessageInfoUpdate,
  MessageTiming,
  SessionAverage,
  StreamSample,
  TokenCounts,
  TokenEventRow,
  TokenEventSource,
  TokenStorage,
  TokenStorageConfig,
  TrackerState,
  TpsSampleRow,
  WriterResponse,
} from "./types.ts"

const STREAM_WINDOW_MS = 5_000
const LIVE_STALE_MS = 1_500
const SINGLE_SAMPLE_MS = 1_000
const DEFAULT_DB_NAME = "oc-tps.sqlite"
const DEFAULT_RETENTION_DAYS = 365
const UNKNOWN_VALUE = "unknown"

function readStringOption(options: Record<string, unknown> | undefined, key: string) {
  const value = options?.[key]
  if (typeof value !== "string") return undefined
  const trimmed = value.trim()
  return trimmed.length > 0 ? trimmed : undefined
}

function readNumberOption(options: Record<string, unknown> | undefined, key: string, fallback: number) {
  const value = options?.[key]
  return typeof value === "number" && Number.isFinite(value) ? value : fallback
}

function storageConfig(statePath: string, options: Record<string, unknown> | undefined): TokenStorageConfig {
  const configuredPath = readStringOption(options, "dbPath")
  const dbPath = configuredPath
    ? isAbsolute(configuredPath)
      ? configuredPath
      : join(statePath, configuredPath)
    : join(statePath, DEFAULT_DB_NAME)

  return {
    dbPath,
    retentionDays: readNumberOption(options, "retentionDays", DEFAULT_RETENTION_DAYS),
  }
}

function createTokenStorage(config: TokenStorageConfig, onError: () => void): TokenStorage {
  const worker = new Worker(new URL("./oc-tokeninspector-writer.ts", import.meta.url))
  worker.onmessage = (event: MessageEvent<WriterResponse>) => {
    if (event.data.type === "error") onError()
  }
  worker.onerror = () => {
    onError()
  }
  worker.postMessage({ type: "init", dbPath: config.dbPath, retentionDays: config.retentionDays })

  return {
    flush(tokenRows, tpsRows, infoUpdates) {
      if (tokenRows.length === 0 && tpsRows.length === 0 && infoUpdates.length === 0) return
      worker.postMessage({ type: "flush", tokenRows, tpsRows, infoUpdates })
    },
    close() {
      worker.postMessage({ type: "close" })
    },
  }
}

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

function estimateStreamTokens(delta: string) {
  return Math.max(1, Math.ceil(Buffer.byteLength(delta, "utf8") / 5))
}

function formatRate(value: number, label: "TPS" | "AVG") {
  if (!Number.isFinite(value) || value <= 0) return undefined
  if (value >= 100) return `${Math.round(value)}${label === "TPS" ? " TPS" : ""}`
  if (value >= 10) return `${value.toFixed(1)}${label === "TPS" ? " TPS" : ""}`
  return `${value.toFixed(2)}${label === "TPS" ? " TPS" : ""}`
}

function formatTtft(value: number) {
  if (!Number.isFinite(value) || value < 0) return undefined
  return `${value.toFixed(1)}s`
}

function activeDurationMs(samples: StreamSample[], tailAt?: number) {
  if (samples.length === 0) return 0
  if (samples.length === 1) {
    const tailDuration = tailAt ? Math.max(0, tailAt - samples[0].at) : SINGLE_SAMPLE_MS
    return Math.min(Math.max(tailDuration, 250), SINGLE_SAMPLE_MS)
  }

  let duration = 0
  for (let i = 1; i < samples.length; i++) {
    duration += Math.max(0, samples[i].at - samples[i - 1].at)
  }

  if (tailAt) {
    duration += Math.max(0, tailAt - samples[samples.length - 1].at)
  }

  return Math.max(duration, SINGLE_SAMPLE_MS)
}

function SessionPromptRight(props: {
  api: Parameters<TuiPlugin>[0]
  sessionID: string
  tracker: TrackerState
  version: () => number
  clock: () => number
}) {
  const sessionAverage = createMemo(() => {
    props.version()
    const totals = props.tracker.sessionAverageByID[props.sessionID]
    if (!totals || totals.totalTokens <= 0 || totals.totalDurationMs <= 0) return undefined
    return formatRate(totals.totalTokens / (totals.totalDurationMs / 1000), "AVG")
  })

  const sessionTtft = createMemo(() => {
    props.version()
    const totals = props.tracker.sessionAverageByID[props.sessionID]
    if (!totals || totals.messageCount <= 0 || totals.totalTtftMs < 0) return undefined
    return formatTtft(totals.totalTtftMs / totals.messageCount / 1000)
  })

  const liveTps = createMemo(() => {
    props.version()
    props.clock()
    const status = props.api.state.session.status(props.sessionID)
    if (status?.type === "idle") return undefined
    const samples = props.tracker.streamSamplesBySession[props.sessionID] ?? []
    if (samples.length === 0) return undefined
    const now = Date.now()
    const relevant = samples.filter((sample) => now - sample.at <= STREAM_WINDOW_MS)
    if (relevant.length === 0) return undefined
    const lastSample = relevant[relevant.length - 1]
    if (!lastSample || now - lastSample.at > LIVE_STALE_MS) return undefined
    const total = relevant.reduce((sum, sample) => sum + sample.tokens, 0)
    const durationSeconds = activeDurationMs(relevant, now) / 1000
    if (durationSeconds <= 0) return undefined
    return formatRate(total / durationSeconds, "AVG")
  })

  const text = createMemo(() => {
    const live = liveTps() ?? "-"
    const avg = sessionAverage() ?? "-"
    const ttft = sessionTtft() ?? "-"
    return `TPS ${live} | AVG ${avg} | TTFT ${ttft}`
  })

  return <>{text() ? <text fg={props.api.theme.current.textMuted}>{text()}</text> : null}</>
}

const tui: TuiPlugin = async (api, options) => {
  const tracker: TrackerState = {
    streamSamplesBySession: {},
    messageTimingByID: {},
    sessionAverageByID: {},
  }
  let pendingRows: TokenEventRow[] = []
  let pendingTpsRows: TpsSampleRow[] = []
  let pendingInfoUpdates: MessageInfoUpdate[] = []
  const messageInfoByID: Record<string, MessageInfo> = {}
  const seenSessionIDs = new Set<string>()
  const messagesWithStepRows = new Set<string>()
  let storageErrorShown = false
  let storage: TokenStorage | undefined
  const [version, setVersion] = createSignal(0)
  const [clock, setClock] = createSignal(Date.now())

  const bump = () => setVersion((value) => value + 1)

  const showStorageError = () => {
    if (storageErrorShown) return
    storageErrorShown = true
    api.ui.toast({ variant: "error", message: "oc-tokeninspector sqlite write failed" })
  }

  try {
    storage = createTokenStorage(storageConfig(api.state.path.state, options), showStorageError)
  } catch {
    api.ui.toast({ variant: "error", message: "oc-tokeninspector sqlite unavailable" })
  }

  const flushRows = (sessionID?: string) => {
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
    } catch {
      showStorageError()
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
    seenSessionIDs.add(sessionID)
    try {
      for (const message of api.state.session.messages(sessionID)) {
        if (message.role !== "assistant") continue
        const completedAt = message.time.completed
        if (typeof completedAt !== "number") continue
        const info = {
          sessionID,
          provider: knownValue(message.providerID),
          model: knownValue(message.modelID),
        }
        updateMessageInfo(message.id, info)
        queueFallbackEvent(
          tokenEventRow({
            recordedAtMs: completedAt,
            sessionID,
            messageID: message.id,
            partID: `message:${message.id}`,
            source: "message-fallback",
            provider: info.provider,
            model: info.model,
            tokens: message.tokens,
          }),
        )
      }
    } catch {
      return
    }
  }

  const pruneSamples = (now = Date.now()) => {
    let changed = false

    for (const [sessionID, samples] of Object.entries(tracker.streamSamplesBySession)) {
      const next = samples.filter((sample) => now - sample.at <= STREAM_WINDOW_MS)
      if (next.length !== samples.length) {
        changed = true
        if (next.length > 0) tracker.streamSamplesBySession[sessionID] = next
        else delete tracker.streamSamplesBySession[sessionID]
      }
    }

    if (changed) bump()
  }

  const clearLiveSamples = (sessionID: string) => {
    if (!tracker.streamSamplesBySession[sessionID]?.length) return
    delete tracker.streamSamplesBySession[sessionID]
    bump()
  }

  const appendSample = (sessionID: string, messageID: string, sample: StreamSample) => {
    seenSessionIDs.add(sessionID)
    const now = sample.at
    tracker.streamSamplesBySession[sessionID] = [
      ...(tracker.streamSamplesBySession[sessionID] ?? []).filter((item) => now - item.at <= STREAM_WINDOW_MS),
      sample,
    ]
    const timing = tracker.messageTimingByID[messageID]
    if (timing) {
      tracker.messageTimingByID[messageID] = timing.firstTokenAt
        ? { ...timing, lastTokenAt: now }
        : {
            ...timing,
            firstResponseAt: timing.firstResponseAt ?? now,
            firstTokenAt: now,
            lastTokenAt: now,
          }
    }
    bump()
  }

  const onDelta = api.event.on("message.part.delta", (evt) => {
    if (evt.properties.field !== "text") return
    const parts = api.state.part(evt.properties.messageID)
    const part = parts.find((item) => item.id === evt.properties.partID)
    if (!part) return
    if (part.type !== "text" && part.type !== "reasoning") return
    appendSample(evt.properties.sessionID, evt.properties.messageID, {
      at: Date.now(),
      tokens: estimateStreamTokens(evt.properties.delta),
    })
  })

  const onMessage = api.event.on("message.updated", (evt) => {
    if (evt.properties.info.role !== "assistant") return
    seenSessionIDs.add(evt.properties.sessionID)
    const messageInfo = {
      sessionID: evt.properties.sessionID,
      provider: knownValue(evt.properties.info.providerID),
      model: knownValue(evt.properties.info.modelID),
    }
    updateMessageInfo(evt.properties.info.id, messageInfo)

    const completedAt = evt.properties.info.time.completed
    if (typeof completedAt !== "number") {
      const existing = tracker.messageTimingByID[evt.properties.info.id]
      tracker.messageTimingByID[evt.properties.info.id] = {
        sessionID: evt.properties.sessionID,
        requestStartAt: evt.properties.info.time.created,
        firstResponseAt: existing?.firstResponseAt,
        firstTokenAt: existing?.firstTokenAt,
        lastTokenAt: existing?.lastTokenAt,
        lastToolCallAt: existing?.lastToolCallAt,
      }
      bump()
      return
    }

    const timing = tracker.messageTimingByID[evt.properties.info.id]
    if (timing?.sessionID === evt.properties.sessionID && typeof timing.firstResponseAt === "number") {
      const totalTokens = evt.properties.info.tokens.output + evt.properties.info.tokens.reasoning
      const endAt =
        evt.properties.info.finish === "tool-calls"
          ? timing.lastToolCallAt
          : completedAt
      const durationMs = typeof endAt === "number" ? Math.max(endAt - timing.firstResponseAt, 1) : undefined
      const ttftMs = Math.max(timing.firstResponseAt - timing.requestStartAt, 0)
      if (totalTokens > 0 && durationMs) {
        const totals = tracker.sessionAverageByID[evt.properties.sessionID] ?? {
          totalTokens: 0,
          totalDurationMs: 0,
          totalTtftMs: 0,
          messageCount: 0,
        }
        tracker.sessionAverageByID[evt.properties.sessionID] = {
          totalTokens: totals.totalTokens + totalTokens,
          totalDurationMs: totals.totalDurationMs + durationMs,
          totalTtftMs: totals.totalTtftMs + ttftMs,
          messageCount: totals.messageCount + 1,
        }
        queueTpsSample({
          recordedAt: new Date(completedAt).toISOString(),
          recordedAtMs: completedAt,
          sessionID: evt.properties.sessionID,
          messageID: evt.properties.info.id,
          provider: messageInfo.provider,
          model: messageInfo.model,
          outputTokens: evt.properties.info.tokens.output,
          reasoningTokens: evt.properties.info.tokens.reasoning,
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
        sessionID: evt.properties.sessionID,
        messageID: evt.properties.info.id,
        partID: `message:${evt.properties.info.id}`,
        source: "message-fallback",
        provider: messageInfo.provider,
        model: messageInfo.model,
        tokens: evt.properties.info.tokens,
      }),
    )
    delete tracker.messageTimingByID[evt.properties.info.id]
    pruneSamples(completedAt)
    bump()
  })

  const onPart = api.event.on("message.part.updated", (evt) => {
    seenSessionIDs.add(evt.properties.sessionID)
    if (evt.properties.part.type === "step-finish") {
      const info = messageInfoByID[evt.properties.part.messageID]
      queueTokenEvent(
        tokenEventRow({
          recordedAtMs: evt.properties.time,
          sessionID: evt.properties.sessionID,
          messageID: evt.properties.part.messageID,
          partID: evt.properties.part.id,
          source: "step-finish",
          provider: info?.provider,
          model: info?.model,
          tokens: evt.properties.part.tokens,
        }),
      )
      return
    }
    if (evt.properties.part.type !== "tool") return
    if (
      evt.properties.part.state.status === "running" ||
      evt.properties.part.state.status === "completed" ||
      evt.properties.part.state.status === "error"
    ) {
      clearLiveSamples(evt.properties.sessionID)
    }
    const timing = tracker.messageTimingByID[evt.properties.part.messageID]
    if (!timing) return
    if (evt.properties.part.state.status === "pending") {
      tracker.messageTimingByID[evt.properties.part.messageID] = {
        ...timing,
        firstResponseAt: timing.firstResponseAt ?? evt.properties.time,
      }
      bump()
      return
    }
    if (evt.properties.part.state.status !== "running") return
    tracker.messageTimingByID[evt.properties.part.messageID] = {
      ...timing,
      lastToolCallAt: evt.properties.part.state.time.start,
    }
    bump()
  })

  const onSessionIdle = api.event.on("session.idle", (evt) => {
    queueSessionFallbacks(evt.properties.sessionID)
    flushRows(evt.properties.sessionID)
  })

  const onSessionDeleted = api.event.on("session.deleted", (evt) => {
    queueSessionFallbacks(evt.properties.sessionID)
    flushRows(evt.properties.sessionID)
  })

  const timer = setInterval(() => {
    setClock(Date.now())
    pruneSamples()
    flushRows()
  }, 1000)

  api.lifecycle.onDispose(() => {
    onDelta()
    onMessage()
    onPart()
    onSessionIdle()
    onSessionDeleted()
    clearInterval(timer)
    for (const sessionID of seenSessionIDs) {
      queueSessionFallbacks(sessionID)
    }
    flushRows()
    storage?.close()
  })

  api.slots.register({
    slots: {
      session_prompt_right(_ctx, value) {
        return <SessionPromptRight api={api} sessionID={value.session_id} tracker={tracker} version={version} clock={clock} />
      },
    },
  })
}

const plugin: TuiPluginModule & { id: string } = {
  id: "oc-tokeninspector",
  tui,
}

export default plugin
