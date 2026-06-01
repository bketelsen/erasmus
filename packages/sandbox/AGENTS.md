# packages/sandbox Agent Guide

## Boundary

`sandbox` owns path and process policy primitives for tools.

## Rules

- Path validation must prevent cwd escapes, including symlink escapes.
- Keep policy independent from UI confirmation.
- Add tests for every path edge case.

## Quirks

- Tool packages rely on this layer for safety; avoid ad hoc path checks in tools when policy helpers exist.

