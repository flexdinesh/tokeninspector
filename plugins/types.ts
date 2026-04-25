export type StreamSample = {
  at: number
  tokens: number
}

export type MessageTiming = {
  sessionID: string
  requestStartAt: number
  firstResponseAt?: number
  firstTokenAt?: number
  lastTokenAt?: number
  lastToolCallAt?: number
}

export type SessionAverage = {
  totalTokens: number
  totalDurationMs: number
  totalTtftMs: number
  messageCount: number
}

export type TrackerState = {
  streamSamplesBySession: Record<string, StreamSample[]>
  messageTimingByID: Record<string, MessageTiming>
  sessionAverageByID: Record<string, SessionAverage>
}

export type TokenCounts = {
  total?: number
  input: number
  output: number
  reasoning: number
  cache: {
    read: number
    write: number
  }
}

export type MessageInfo = {
  sessionID: string
  provider: string
  model: string
}

export type TokenEventSource = "step-finish" | "message-fallback"

export type TokenEventRow = {
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

export type TpsSampleRow = {
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

export type TokenStorageConfig = {
  dbPath: string
  retentionDays: number
}

export type MessageInfoUpdate = {
  messageID: string
  info: MessageInfo
}

export type TokenStorage = {
  flush: (tokenRows: TokenEventRow[], tpsRows: TpsSampleRow[], infoUpdates: MessageInfoUpdate[]) => void
  close: () => void
}

export type WriterResponse = {
  type: "ready" | "flushed" | "closed" | "error"
  message?: string
}

export type RequestRow = {
  recordedAt: string
  recordedAtMs: number
  sessionID: string
  messageID: string
  provider: string
  model: string
  attemptIndex: number
  thinkingLevel: string
}

export type RequestStorage = {
  insert: (row: RequestRow) => void
  close: () => void
}
