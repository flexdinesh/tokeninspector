# tokeninspector-cli

Query token usage data written by the TokenInspector OpenCode plugin and Pi extension.

The CLI reads the SQLite database directly, aggregates rows from the OpenCode `oc_*` table family and Pi `pi_*` table family, and opens a styled terminal table grouped by day, provider, and model. With `--group-by=hour`, it expands each day into hourly buckets. With `--group-by=session`, it expands each day into session buckets.

## Usage

```sh
~/workspace/tokeninspector/cli/tokeninspector-cli --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite
```

The default interactive view shows the current week. Press `q` to quit.

More examples:

```sh
tokeninspector-cli table --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --today
tokeninspector-cli table --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --week
tokeninspector-cli table --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --month
tokeninspector-cli table --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --all-time
tokeninspector-cli table --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --today --group-by=hour
tokeninspector-cli table --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --week --group-by=hour
tokeninspector-cli table --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --week --group-by=session
tokeninspector-cli table --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --week --provider openai --model gpt-5.5
tokeninspector-cli table --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --month --filter-day-from 2026-04-20 --filter-day-to 2026-04-25 --session-id ses_abc,ses_xyz
```

## Commands

Interactive mode

Open the styled terminal UI. Defaults to the current week when no period flag is passed.

```sh
tokeninspector-cli --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite
tokeninspector-cli --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --today
tokeninspector-cli --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --month --group-by=session
```

`table`

Legacy alias for interactive mode.

## Arguments

`--db-path PATH`

Required. Path to the SQLite database created by TokenInspector.

Default TokenInspector DB path:

```text
~/.local/state/tokeninspector/tokeninspector.sqlite
```

`--today`

Show data from today, grouped by day unless `--group-by` is present.

`--week`

Show data from the current calendar week (Monday 00:00 to Sunday 23:59), grouped by day unless `--group-by` is present.

`--month`

Show data from the current calendar month, grouped by day unless `--group-by` is present.

`--all-time`

Show all data with no time filter. This can be slow on large databases.

`--group-by=hour|session`

Optional. Split the selected period by hour or session. Only one `--group-by` can be passed.

`--group-by=hour` adds an `hour` column after `day`. Hours with no matching data are not printed.

`--group-by=session` adds a `session id` column after `day`.

`--session-id ID`

Optional. Filter by OpenCode session ID. Can be repeated or comma-separated.

```sh
tokeninspector-cli table --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --week --session-id ses_abc --session-id ses_xyz
tokeninspector-cli table --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --week --session-id ses_abc,ses_xyz
```

`--provider ID`

Optional. Filter by provider ID. Can be repeated or comma-separated.

```sh
tokeninspector-cli table --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --week --provider openai --provider github-copilot
```

`--model ID`

Optional. Filter by model ID. Can be repeated or comma-separated.

```sh
tokeninspector-cli table --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --week --model gpt-5.5 --model claude-opus-4.7
```

`--filter-day-from YYYY-MM-DD`

Optional. Filter from this local day (inclusive). Must be a valid `YYYY-MM-DD` date.

`--filter-day-to YYYY-MM-DD`

Optional. Filter to this local day (inclusive). Must be a valid `YYYY-MM-DD` date. `--filter-day-from` must not be after `--filter-day-to`.

These range filters apply in addition to any selected period (`--today`, `--week`, `--month`, `--all-time`).

```sh
tokeninspector-cli table --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --month --filter-day-from 2026-04-20 --filter-day-to 2026-04-25
tokeninspector-cli table --db-path ~/.local/state/tokeninspector/tokeninspector.sqlite --all-time --filter-day-from 2026-04-20 --filter-day-to 2026-04-25
```

Interactive mode defaults to `--week` when no period flag is passed.

## Display

Session IDs are shortened to the last 8 characters in table output.

Model names with `/` are shortened to the last path segment. For example, `openai/gpt-5.5` is shown as `gpt-5.5`.

## Metrics

Token columns are summed from `oc_token_events`:

```text
input
output
reasoning
cache read
cache write
total
```

`total` means OpenCode `tokens.total` when present in the plugin, otherwise input + output + reasoning + cache read + cache write.

TPS columns are read from `oc_tps_samples` and are part of the core project output:

```text
tps avg
tps mean
tps median
```

Request columns are read from `oc_llm_requests` when the server plugin is installed:

```text
requests
retries
```

`requests` counts initial LLM provider attempts. `retries` counts later attempts for the same session/message/provider/model.

With `--group-by=session`, the table also shows `thinking` after `session id`. It is a comma-separated list of non-unknown thinking levels seen for that session/provider/model row.

## Example Output

Daily output:

```text
╭────────────┬────────────────┬─────────────────┬─────────┬──────────┬────────────┬───────┬────────┬───────────┬────────────┬─────────────┬───────╮
│ day        │ provider       │ model           │ tps avg │ tps mean │ tps median │ input │ output │ reasoning │ cache read │ cache write │ total │ requests │ retries │
├────────────┼────────────────┼─────────────────┼─────────┼──────────┼────────────┼───────┼────────┼───────────┼────────────┼─────────────┼───────┼──────────┼─────────┤
│ 2026-04-24 │ github-copilot │ claude-opus-4.7 │   68.30 │   553.52 │     127.86 │  112K │     3K │        1K │        75K │         600 │  192K │       12 │       1 │
│ 2026-04-24 │ openai         │ gpt-5.5         │   45.24 │  2230.81 │      66.91 │   82K │     1K │       900 │        41K │         300 │  126K │        9 │       0 │
╰────────────┴────────────────┴─────────────────┴─────────┴──────────┴────────────┴───────┴────────┴───────────┴────────────┴─────────────┴───────╯
```

Hourly output:

```text
day        | hour  | provider       | model           | tps avg | tps mean | tps median | input | output | reasoning | cache read | cache write | total | requests | retries
2026-04-24 | 16:00 | github-copilot | claude-opus-4.7 |   68.30 |   553.52 │     127.86 │   32K │    900 │       300 │        21K │         200 │   54K │        4 │       1
2026-04-24 | 16:00 | openai         | gpt-5.5         │   45.24 │  2230.81 │      66.91 │   21K │    500 │       250 │        12K │         100 │   33K │        3 │       0
```

Session output:

```text
day        | session id | thinking | provider       │ model           │ tps avg │ tps mean │ tps median │ input │ output │ reasoning │ cache read │ cache write │ total │ requests │ retries
2026-04-24 │ ses_abc    │ high     │ github-copilot │ claude-opus-4.7 │   68.30 │   553.52 │     127.86 │   32K │    900 │       300 │        21K │         200 │   54K │        4 │       1
2026-04-24 │ ses_xyz    │ low,high │ openai         │ gpt-5.5         │   45.24 │  2230.81 │      66.91 │   21K │    500 │       250 │        12K │         100 │   33K │        3 │       0
```

## Notes

The default database filename is `tokeninspector.sqlite`. Current primary table families are `oc_*` for OpenCode data and `pi_*` for Pi data.
