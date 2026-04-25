PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;

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
);

CREATE INDEX IF NOT EXISTS oc_token_events_recorded_at_ms_idx ON oc_token_events (recorded_at_ms);
CREATE INDEX IF NOT EXISTS oc_token_events_session_time_idx ON oc_token_events (session_id, recorded_at_ms);
CREATE INDEX IF NOT EXISTS oc_token_events_provider_model_time_idx ON oc_token_events (provider, model, recorded_at_ms);

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
);

CREATE INDEX IF NOT EXISTS oc_tps_samples_recorded_at_ms_idx ON oc_tps_samples (recorded_at_ms);
CREATE INDEX IF NOT EXISTS oc_tps_samples_session_time_idx ON oc_tps_samples (session_id, recorded_at_ms);
CREATE INDEX IF NOT EXISTS oc_tps_samples_provider_model_time_idx ON oc_tps_samples (provider, model, recorded_at_ms);

CREATE TABLE IF NOT EXISTS oc_llm_requests (
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

CREATE INDEX IF NOT EXISTS oc_llm_requests_recorded_at_ms_idx ON oc_llm_requests (recorded_at_ms);
CREATE INDEX IF NOT EXISTS oc_llm_requests_session_time_idx ON oc_llm_requests (session_id, recorded_at_ms);
CREATE INDEX IF NOT EXISTS oc_llm_requests_provider_model_time_idx ON oc_llm_requests (provider, model, recorded_at_ms);

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

PRAGMA user_version = 1;
