# LLM Request Metrics

Summary: Add request-attempt tracking via a separate OpenCode server plugin, persist to SQLite, and surface `requests` + `retries` in all CLI groupings.

## Implementation Changes

- Add `plugins/oc-tokeninspector-server.ts`.
- Use server hook `chat.headers`.
- On each hook call, write one row to new SQLite table `oc_llm_requests`.
- Store `recorded_at`, `recorded_at_ms`, `session_id`, `message_id`, `provider`, `model`, `attempt_index`.
- `requests` = count where `attempt_index == 1`.
- `retries` = count where `attempt_index > 1`.
- Track attempts in-memory by `session_id + message_id + provider + model`.
- Server plugin uses the same default DB location as OpenCode state on this machine via `XDG_STATE_HOME`/`~/.local/state/opencode`, with `OC_TOKENINSPECTOR_DB_PATH` and `OC_TOKENINSPECTOR_RETENTION_DAYS` overrides. TUI `api.state.path.state` is not available to server plugins.
- Add indexes for `recorded_at_ms`, `(session_id, recorded_at_ms)`, `(provider, model, recorded_at_ms)`.
- Add retention cleanup for request rows.
- Update CLI to query `oc_llm_requests` if table exists.
- Add request counts to `sample`, `aggregate`, `tableRow`.
- Render `requests` and `retries` columns in day/hour/session tables.
- Keep day/session/provider/model filters applying to request rows.
- Update README, `docs/how-it-works.md`, and `cli/README.md`.

## Tests / Verification

- Go tests for aggregation summing `requests` + `retries`.
- Go tests for all group modes rendering columns.
- Go DB-query tests for request rows when table exists.
- Go DB-query tests for CLI compatibility when `oc_llm_requests` is absent.
- Plugin smoke builds for TUI and server plugin.
- CLI verification with `go test ./...` and `go build -o tokeninspector-cli .`.

## Decisions Made

- Scope: LLM provider requests only.
- Retries: counted separately from initial requests.
- Storage: SQLite.
- CLI: visible in all groupings.
- Plugin shape: separate server companion plugin.

## Tradeoffs / Risks

- `chat.headers` counts request attempts before provider call, not confirmed HTTP success.
- Retry detection depends on OpenCode invoking `chat.headers` for each retry.
- No HTTP status/latency unless OpenCode exposes a later response hook.
- TUI and server plugins must share DB schema safely.
- Server plugin config differs from TUI plugin options because server plugin API does not expose TUI state paths/options in the local reference.

## Other Requests

Non-LLM requests can include tool network calls, MCP traffic, auth/OAuth, provider metadata lookups, plugin-owned fetches, install/update checks, and local TUI/server API calls.

## Execution Guidance

If execution deviates, update this plan to match the latest approved plan and surface the deviation before continuing.

## Open Questions

None.
