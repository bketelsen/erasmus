# packages/session Agent Guide

## Boundary

`session` defines durable session interfaces, metadata, context building contracts, and optional tree/branching interfaces.

## Rules

- Keep backend-specific storage out of this package.
- Interface changes must be reflected in memory and JSONL backends.
- Preserve canonical message/event data shapes.

## Quirks

- Tree behavior is optional; callers must type-check for `session.Tree`.

