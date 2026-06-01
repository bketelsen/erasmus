# packages/rpc Agent Guide

## Boundary

`rpc` exposes harness runtimes over JSON-lines RPC, including multi-runtime lifecycle and event streaming.

## Rules

- Keep wire methods explicit and documented in tests/docs.
- Runtime behavior should delegate to harness/app services rather than duplicating semantics.
- Preserve durable session runtime creation/resume behavior.

## Quirks

- RPC supports runtime extension tools/commands; extension lifecycle changes may need RPC updates.

