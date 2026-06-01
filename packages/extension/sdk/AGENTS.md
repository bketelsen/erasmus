# packages/extension/sdk Agent Guide

## Boundary

`sdk` provides thin Go helpers for authoring Erasmus extension subprocesses that speak the JSON-line protocol.

## Rules

- Keep this package below the host runtime; do not import `harness`, `app`, CLI, TUI, or `packages/extension` host manager code.
- Prefer explicit protocol helpers over hidden behavior. `packages/extension/proto` remains the canonical wire shape.
- Keep handlers headless and stdin/stdout oriented.

## Quirks

- Tool results use canonical `tool.Result` and `message.Text` content, which marshal cleanly but should not be unmarshaled into interface-backed content in tests.
