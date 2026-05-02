---
title: CLI provider and harness filters
description: Interactive CLI filters now own provider/harness filter state after initialization from flags.
date: 2026-05-01
slug: cli-provider-harness-filters
status: implemented
tags:
  - cli
  - filters
  - tui
related_paths:
  - cli/internal/cli/table.go
  - cli/internal/cli/flags.go
  - cli/internal/db/aggregate.go
  - cli/internal/db/filter_values.go
  - docs/design.md
---

## Why

The interactive CLI needed a quick way to narrow existing aggregate views by provider or harness without restarting the command with different flags.

## What

- `f` opens an interactive filter popup with two dimensions: `provider` and `harness`.
- `--harness` is supported alongside the existing `--provider` flag.
- CLI flags initialize the effective TUI filter state, but they are not immutable constraints after launch.
- The popup always reflects the current effective filter state. If no provider filter is active, no providers are selected; if `--provider openai` initialized the state, `openai` is selected when opening the provider filter.
- Users can clear filters by unchecking all selected values and pressing `enter`.

## How

- Provider discovery is global across token, TPS, request, and tool-call data, not active-tab-specific.
- Provider discovery respects the selected period/date/session/model filters and the current harness filter.
- Harness discovery respects the selected period/date/session/model filters and the current provider filter.
- Same-dimension selected values use OR behavior, e.g. `openai` plus `anthropic` means `provider IN ('openai', 'anthropic')`.
- Harness filtering is implemented by selecting which table families are queried (`oc_*`, `pi_*`) rather than changing schema.

## Tradeoffs

- Global provider discovery keeps filters stable across tabs, but a provider may appear even when the active tab has no matching rows.
- Letting the popup replace filters initialized from CLI flags makes TUI behavior intuitive, but means flags are startup state rather than permanent constraints.

## Gotchas

- `esc` closes the filter popup without applying staged changes.
- In the filter dimension step, `space` or `enter` enters value selection. In value selection, `space` toggles values and `enter` applies.
- No schema change was made or needed.
