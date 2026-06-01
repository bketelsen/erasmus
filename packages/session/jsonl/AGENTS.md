# packages/session/jsonl Agent Guide

## Boundary

`jsonl` is the durable JSONL session backend.

## Rules

- Preserve append-only behavior where possible.
- Keep reopen/build-context behavior compatible with existing session files.
- Test parent/leaf persistence and branch/move behavior.

## Quirks

- Local `.jsonl` files are ignored by git; tests should use temp directories.

