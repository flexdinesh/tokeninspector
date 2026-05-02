# Interactive provider/harness filters for CLI TUI

## Summary

Add filtering by provider and harness in the interactive CLI. Pressing `f` opens a filter popup. The popup first lets the user choose `provider` or `harness`, then shows available values. The user toggles values with `space` and applies with `enter`; the table reloads with the selected filters.

No schema changes are required.

## Decisions made

- `f` opens the filter popup.
- Filter dimensions:
  - `provider`
  - `harness`
- In the dimension step:
  - `space` or `enter` enters value selection.
- In value selection:
  - `space` toggles highlighted value.
  - `enter` applies the staged selection and reloads rows.
  - `esc` cancels/closes without applying staged changes.
- No dedicated clear shortcut.
  - To clear a filter, uncheck all selected values and press `enter`.
- CLI flags initialize the TUI filter state.
  - Example: `--provider openai` means provider popup opens with `openai` selected.
- After launch, popup controls the latest filter state.
  - User can remove filters originally supplied by CLI flags.
  - Popup always reflects the current effective table state.
- Same-dimension selected values use OR behavior.
  - `openai` + `anthropic` means `provider IN ('openai', 'anthropic')`.
- Provider discovery is global across the dataset/tabs, not active-tab-specific.
  - It should respect period/date/session/model/harness filters.
  - It should not depend on whether the user is on tokens/TPS/requests/tool tabs.
- Popup help text should be shown at the bottom:
  - Dimension step: `space/enter = select · esc = close without applying`
  - Value step: `space = select · enter = apply · esc = close without applying`

## Key implementation changes

1. Add harness filter support:
   - Add `Harnesses []string` to `db.Filter`.
   - Add `harnesses stringList` to CLI `filters`.
   - Add `--harness` flag with repeat/comma support.
   - Validate harness values as `oc` or `pi`.
   - In aggregation, query only the selected harness table families.
   - Pass current harness filters from `loadRows` into `db.Filter`.

2. Add available value discovery:
   - Add `AvailableProviders(ctx, database, filter)` and `AvailableHarnesses(ctx, database, filter)`.
   - Search globally across token events, TPS samples, LLM requests, and tool calls.
   - Respect selected period/start, day range, session/model filters, and the opposite dimension filter.
   - Return sorted unique values.

3. Refactor popup state in `interactiveModel`:
   - Keep existing grouping popup behavior.
   - Add filter dimension and filter value popup states.
   - Values should be displayed as checkboxes and preselected from current effective filters.
   - `esc` cancels without applying staged changes.
   - Applying values updates `m.options.filters.providers` or `m.options.filters.harnesses`, resets scroll, and reloads rows.

4. Update rendering/hints:
   - Main hint includes `f filter`.
   - Filter popup includes key tips at the bottom.
   - Show active filters compactly in the footer if present.

5. Update docs:
   - `README.md` and `docs/design.md` document `--harness`, interactive `f`, popup controls, and mutable TUI filters.

## Tests and verification

- Add/update Go tests for:
  - harness flag parsing and validation
  - harness filtering in aggregation
  - available provider/harness discovery
  - filter selection helpers where practical
- Run:

```sh
cd cli
go test ./...
go build -o tokeninsights-cli ./cmd/tokeninsights-cli
```

## Tradeoffs and risks

- Global provider discovery is stable across tabs, but a provider may appear even if the active tab has no rows for it.
- Replacing CLI filters after launch is intuitive for TUI state, but means CLI flags are not immutable constraints.
- Harness filtering via table-family selection avoids schema changes and is efficient, but requires consistent application across all aggregation paths.
- Popup state refactor should be kept small to avoid destabilizing existing grouping behavior.

## Remaining open questions

None.

## Execution guidance

If implementation deviates from this plan, update this saved plan to reflect the latest approved behavior and surface the deviation before continuing.
