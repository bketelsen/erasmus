# packages/agent Agent Guide

## Boundary

`agent` is the stateful in-memory wrapper around `loop`. It owns transcript state, active run lifecycle, event subscriptions, abort/wait, steering, and follow-up queues.

## Rules

- Do not add session persistence, CLI behavior, TUI behavior, or provider construction here.
- Keep the max-one-active-run guard intact.
- Mutate state under the agent mutex and return snapshots as copies.

## Quirks

- The harness depends on agent events for persistence; preserve terminal `AgentEnd` behavior on errors and cancellation.
- `SetStream`, `SetModel`, and `SetReasoning` affect subsequent runs only.

