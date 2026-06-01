# packages/harness Agent Guide

## Boundary

`harness` is the durable runtime orchestration layer. It owns session persistence, model/reasoning state, prompt resources, tools, skills, extensions, compaction, session tree operations, and event publication around an `agent.Agent`.

## Rules

- If runtime behavior matters outside one frontend, it probably belongs here or lower.
- Do not add terminal rendering, CLI parsing, or TUI key handling.
- Persist session changes before publishing related update events when possible.
- Keep harness APIs usable headlessly.

## Quirks

- `SetModelAndStream` must update model metadata and the provider stream together for cross-provider switching.
- The harness persists messages by observing agent events; event ordering is important.

