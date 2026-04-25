# Plan: Full-Screen Bordered TUI for tokeninspector CLI

**Summary:** Switch the CLI interactive mode to a full-screen terminal UI with an outer border around the app and rounded borders around each major section (title, tabs, table, hint). The grouping popup becomes a centered floating modal.

---

## Key Implementation Changes

| File | What Changes |
|------|-------------|
| `cli/internal/cli/table.go` | Add `tea.WithAltScreen()` to enter full-screen; redesign `View()` with outer + section borders; make popup a centered modal |
| `cli/internal/cli/render.go` | Add new `lipgloss` border styles for outer, section, and popup; keep existing table/cell styles |
| `cli/internal/cli/render_test.go` | Likely no changes (tests call `renderTable` which stays the same) |
| `cli/internal/cli/testdata/*` | Likely no changes |

---

## Detailed Design

1. **Alt screen mode**
   - In `RunInteractive`, pass `tea.WithAltScreen()` to `tea.NewProgram`.
   - Terminal switches to alternate buffer on launch and restores the previous screen on quit.

2. **Section layout inside outer border**
   - Outer border: `lipgloss.RoundedBorder()`, width = `m.width`, color slightly dim (`240`).
   - Inner padding: 1 char on each side between outer border and sections.
   - Title box: rounded border, contains the title text.
   - Tab box: rounded border, contains styled tab pills.
   - Table area: **no extra wrapper** — the table's own existing `RoundedBorder()` serves as its section border. Width reduced from `m.width` to `m.width - 2` so it fits flush inside the outer border inner edge.
   - Hint box: rounded border, contains the hint text.
   - All sections joined vertically with no blank lines between them.

3. **Width math**
   - Outer border total width = `m.width`.
   - Each inner section total width = `m.width - 2` (outer border consumes 2 chars).
   - Table `Width(m.width - 2)` so its outer edges align with the inner edges of other section boxes.

4. **Popup modal**
   - When `showPopup` is true, render the popup centered using `lipgloss.Place(m.width, m.height, Center, Center, popupContent)`.
   - This replaces the entire frame with the popup surrounded by blank space — standard TUI modal behavior.
   - Popup keeps its existing rounded border and pink (`212`) border foreground.

5. **Styles**
   - Outer border style: `lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))`
   - Section border style: same rounded border, maybe `245` for subtle distinction
   - Reuse existing `popupStyle` for the modal

---

## Tradeoffs & Risks

| Concern | Decision |
|---------|----------|
| Double-bordering the table | Avoided by using the table's own border as its section border; no extra wrapper. |
| Screen real estate | Section borders + outer border cost ~4 extra vertical rows vs. current flat layout. Acceptable; alt screen gives full terminal height. |
| Popup background visibility | Standard modal behavior (popup centered on blank backdrop). True overlay isn't possible in text terminals without complex ANSI cursor games. |
| Width < table minimum | Lipgloss truncates gracefully. No extra guard needed. |

---

## Verification Steps

```sh
cd cli
go test ./...
go build -o tokeninspector-cli .
./tokeninspector-cli --db-path ~/.local/state/opencode/oc-tps.sqlite --day
```

- Verify the app launches full-screen.
- Verify title, tabs, table, and hint each have visible rounded borders.
- Verify tab switching, scrolling, and `g` popup still work.
- Verify `q` quits and terminal screen is restored.

---

## Open Questions

None — all scope decisions have been answered.
