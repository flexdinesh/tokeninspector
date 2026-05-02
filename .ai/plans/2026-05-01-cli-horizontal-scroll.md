# CLI horizontal table scrolling

## Summary

Add horizontal scrolling to the interactive Go CLI table. The table will scroll as one whole surface, not with frozen columns. Horizontal position resets when switching tabs, changing grouping, or applying filters.

## Key implementation changes

- Add a horizontal scroll offset to the Bubble Tea interactive model.
- Render the table at natural width with wrapping disabled for the interactive viewport.
- Slice the rendered table through an ANSI-aware horizontal viewport so styled borders and cells remain valid.
- Add key handling for `←`/`→`, `h`/`l`, and `home`/`end`.
- Clamp horizontal offset on resize, data reload, and vertical scroll.
- Update CLI hints and docs to mention horizontal controls.

## Tests and verification

- Add/update unit tests for horizontal scroll clamping, viewport slicing, and key handling.
- Run `cd cli && go test ./...`.
- Run `cd cli && go build -o tokeninsights-cli ./cmd/tokeninsights-cli`.

## Decisions made by user

- Horizontal scrolling should move the entire table.
- Use suggested bindings: arrows plus `h`/`l`; include `home`/`end` jumps.
- Reset horizontal offset on tab, grouping, and filter changes.

## Tradeoffs and risks

- Whole-table scrolling is simpler and lower risk than frozen columns, but row identity columns can scroll off-screen.
- ANSI-aware viewport slicing avoids broken styling, but terminal rendering should still be smoke-tested manually if possible.
- Rendering natural-width tables avoids wrapping, improving horizontal navigation, but very wide rows can require more horizontal movement.

## Remaining open questions

None.

## Execution guidance

If execution deviates from this approved plan, update this plan file to reflect the latest approved plan and surface the deviation to the user.
