# packages/loop Agent Guide

## Boundary

`loop` is the smallest provider/tool runtime unit. It consumes messages, tools, hooks, and a provider stream function, then emits runtime events.

## Rules

- No sessions, CLI, TUI, swarm, extensions, config files, or provider construction.
- Tool errors and unknown tools should become error tool results so the model can recover.
- Respect context cancellation even when provider streams close at the same time.
- Keep event ordering tests current when changing loop behavior.

## Quirks

- Cancellation races are subtle; see context cancellation tests before editing stream consumption.

