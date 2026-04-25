package db

const (
	TableTokenEvents = "oc_token_events"
	TableTpsSamples  = "oc_tps_samples"
	TableLLMRequests = "oc_llm_requests"
)

const (
	ColRecordedAtMs     = "recorded_at_ms"
	ColSessionID        = "session_id"
	ColMessageID        = "message_id"
	ColPartID           = "part_id"
	ColSource           = "source"
	ColProvider         = "provider"
	ColModel            = "model"
	ColInputTokens      = "input_tokens"
	ColOutputTokens     = "output_tokens"
	ColReasoningTokens  = "reasoning_tokens"
	ColCacheReadTokens  = "cache_read_tokens"
	ColCacheWriteTokens = "cache_write_tokens"
	ColTotalTokens      = "total_tokens"
	ColDurationMs       = "duration_ms"
	ColTokensPerSecond  = "tokens_per_second"
	ColTpsTotalTokens   = "total_tokens"
	ColAttemptIndex     = "attempt_index"
	ColThinkingLevel    = "thinking_level"
)

const SupportedSchemaVersion = 1
