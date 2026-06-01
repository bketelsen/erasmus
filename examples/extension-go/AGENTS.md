# examples/extension-go Agent Guide

## Boundary

This example demonstrates the minimal Go extension SDK for subprocess extensions.

## Rules

- Keep the example small: one tool, one command, no provider/runtime dependencies.
- Use `packages/extension/sdk` instead of hand-written JSON-line frame loops.
- Tests should exercise the extension over in-memory streams so no subprocess or real Erasmus binary is required.

## Quirks

- This example is a separate Go module and is validated by `make test-examples`.
