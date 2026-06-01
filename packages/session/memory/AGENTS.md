# packages/session/memory Agent Guide

## Boundary

`memory` is the in-memory session backend for tests, smoke paths, and embedding.

## Rules

- Keep behavior aligned with JSONL semantics where practical.
- Implement tree/branch behavior when the interface requires it.
- Avoid filesystem dependencies.

## Quirks

- This backend is often the fastest place to specify desired session behavior before implementing durable support.

