/** @jsxImportSource @opentui/solid */
import { isAbsolute, join } from "node:path"
import type { TuiPlugin, TuiPluginModule } from "@opencode-ai/plugin/tui"
import { createMemo, createSignal } from "solid-js"
import type {
  StreamSample,
  TokenStorageConfig,
} from "../shared/types.ts"

const STREAM_WINDOW_MS = 5_000
const LIVE_STALE_MS = 1_500
const SINGLE_SAMPLE_MS = 1_000
const BANNER_REFRESH_MS = 2_000
const DEFAULT_DB_NAME = "oc-tps.sqlite"
const DEFAULT_RETENTION_DAYS = 365

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

type SessionAverages = {
  avgTps: number | undefined
  avgTtft: number | undefined
}

function querySessionAverages(dbPath: string, sessionID: string): SessionAverages | undefined {
  try {
    const { Database } = require("bun:sqlite")
    const db = new Database(dbPath, { readonly: true, create: false })
    try {
      const stmt = db.query(`
        SELECT SUM(total_tokens) as throughput_tokens, SUM(duration_ms) as duration_ms, AVG(ttft_ms) as avg_ttft_ms
        FROM oc_tps_samples
        WHERE session_id = ? AND duration_ms > 0
      `)
      const row = stmt.get(sessionID) as {
        throughput_tokens: number | null
        duration_ms: number | null
        avg_ttft_ms: number | null
      } | null
      stmt.finalize()
      if (!row) return undefined
      const throughput = row.throughput_tokens ?? 0
      const duration = row.duration_ms ?? 0
      const avgTtftMs = row.avg_ttft_ms ?? 0
      const avgTps = duration > 0 ? throughput / (duration / 1000) : undefined
      const avgTtft = avgTtftMs > 0 ? avgTtftMs / 1000 : undefined
      return { avgTps, avgTtft }
    } finally {
      db.close()
    }
  } catch {
    return undefined
  }
}

function SessionPromptRight(props: {
  api: Parameters<TuiPlugin>[0]
  sessionID: string
  streamSamplesBySession: Record<string, StreamSample[]>
  version: () => number
  clock: () => number
  dbPath: string
}) {
  const sessionAverages = createMemo((): SessionAverages | undefined => {
    props.version()
    return querySessionAverages(props.dbPath, props.sessionID)
  })

  const sessionAverage = createMemo(() => {
    const averages = sessionAverages()
    if (!averages?.avgTps) return undefined
    return formatRate(averages.avgTps, "AVG")
  })

  const sessionTtft = createMemo(() => {
    const averages = sessionAverages()
    if (!averages?.avgTtft) return undefined
    return formatTtft(averages.avgTtft)
  })

  const liveTps = createMemo(() => {
    props.version()
    props.clock()
    const status = props.api.state.session.status(props.sessionID)
    if (status?.type === "idle") return undefined
    const samples = props.streamSamplesBySession[props.sessionID] ?? []
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
  const streamSamplesBySession: Record<string, StreamSample[]> = {}
  const [version, setVersion] = createSignal(0)
  const [clock, setClock] = createSignal(Date.now())

  const bump = () => setVersion((value) => value + 1)

  const dbConfig = storageConfig(api.state.path.state, options)

  const pruneSamples = (now = Date.now()) => {
    let changed = false

    for (const [sessionID, samples] of Object.entries(streamSamplesBySession)) {
      const next = samples.filter((sample) => now - sample.at <= STREAM_WINDOW_MS)
      if (next.length !== samples.length) {
        changed = true
        if (next.length > 0) streamSamplesBySession[sessionID] = next
        else delete streamSamplesBySession[sessionID]
      }
    }

    if (changed) bump()
  }

  const clearLiveSamples = (sessionID: string) => {
    if (!streamSamplesBySession[sessionID]?.length) return
    delete streamSamplesBySession[sessionID]
    bump()
  }

  const appendSample = (sessionID: string, _messageID: string, sample: StreamSample) => {
    const now = sample.at
    streamSamplesBySession[sessionID] = [
      ...(streamSamplesBySession[sessionID] ?? []).filter((item) => now - item.at <= STREAM_WINDOW_MS),
      sample,
    ]
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

  const onPart = api.event.on("message.part.updated", (evt) => {
    if (evt.properties.part.type !== "tool") return
    if (
      evt.properties.part.state.status === "running" ||
      evt.properties.part.state.status === "completed" ||
      evt.properties.part.state.status === "error"
    ) {
      clearLiveSamples(evt.properties.sessionID)
    }
  })

  const timer = setInterval(() => {
    setClock(Date.now())
    pruneSamples()
  }, 1000)

  const refreshTimer = setInterval(() => {
    bump()
  }, BANNER_REFRESH_MS)

  api.lifecycle.onDispose(() => {
    onDelta()
    onPart()
    clearInterval(timer)
    clearInterval(refreshTimer)
  })

  api.slots.register({
    slots: {
      session_prompt_right(_ctx, value) {
        return (
          <SessionPromptRight
            api={api}
            sessionID={value.session_id}
            streamSamplesBySession={streamSamplesBySession}
            version={version}
            clock={clock}
            dbPath={dbConfig.dbPath}
          />
        )
      },
    },
  })
}

const plugin: TuiPluginModule & { id: string } = {
  id: "oc-tokeninspector",
  tui,
}

export default plugin
