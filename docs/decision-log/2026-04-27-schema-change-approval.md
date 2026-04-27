---
title: Schema changes require explicit user approval
description: Durable rule that any schema change must be surfaced to the user with rationale and approved before implementation
date: 2026-04-27
slug: schema-change-approval
status: implemented
tags:
  - schema
  - governance
  - agents
related_paths:
  - AGENTS.md
  - docs/design.md
  - README.md
  - schema/schema.sql
---

## Why

The schema is the single source of truth for the entire project (plugins, CLI, cross-language contract). Silent or implicit schema changes risk breaking the plugin/CLI boundary, corrupting existing data, or creating migration conflicts. Agents must not make schema changes autonomously.

## What

Any change to `schema/schema.sql`, table structures, column definitions, or the cross-language schema contract must be explicitly approved by the user before implementation. This applies equally to breaking and non-breaking (additive) changes.

## How

- Surface the rationale, impact, and scope to the user
- Ask for explicit approval before modifying any schema-related files
- Update `AGENTS.md`, `docs/design.md`, and `README.md` to reinforce this rule
- Run `bun run scripts/check-schema.ts` after any approved schema change
