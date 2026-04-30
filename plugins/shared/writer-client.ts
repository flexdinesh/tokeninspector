import type { TokenEventRow, TpsSampleRow, MessageInfoUpdate, TokenStorage, WriterResponse, WriterConfig } from "./types.ts"

export function createTokenStorage(
  workerScriptUrl: URL,
  config: WriterConfig,
  onError: () => void,
): TokenStorage {
  let workerReady = false
  const worker = new Worker(workerScriptUrl)
  worker.onmessage = (event: MessageEvent<WriterResponse>) => {
    if (event.data.type === "ready") {
      workerReady = true
      return
    }
    if (event.data.type === "error") {
      workerReady = false
      onError()
    }
  }
  worker.onerror = () => {
    workerReady = false
    onError()
  }
  worker.postMessage({ type: "init", dbPath: config.dbPath, retentionDays: config.retentionDays })

  return {
    flush(tokenRows, tpsRows, infoUpdates, toolRows) {
      if (!workerReady) return
      if (tokenRows.length === 0 && tpsRows.length === 0 && infoUpdates.length === 0 && toolRows.length === 0) return
      worker.postMessage({ type: "flush", tokenRows, tpsRows, infoUpdates, toolRows })
    },
    close() {
      worker.postMessage({ type: "close" })
    },
  }
}
