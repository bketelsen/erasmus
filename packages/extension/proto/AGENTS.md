# packages/extension/proto Agent Guide

## Boundary

`proto` defines the JSON-line protocol frames used between Erasmus and extension subprocesses.

## Rules

- Keep protocol structs stable and explicit.
- Avoid importing host manager logic here.
- Treat unknown or invalid frames as host-level diagnostics, not panics.

## Quirks

- Protocol changes need matching tests in both `proto` and `packages/extension` process/host paths.

