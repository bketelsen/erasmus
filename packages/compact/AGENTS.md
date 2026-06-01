# packages/compact Agent Guide

## Boundary

`compact` prepares transcript compaction and runs summary generation through an injected provider stream.

## Rules

- Keep this package independent from sessions, CLI, TUI, and harness construction.
- Harness orchestrates persistence and event emission around compaction.
- Prefer deterministic preparation tests; use fake streams for model behavior.

## Quirks

- Compaction output must remain suitable for rebuilding session context and continuing a conversation.

