# packages/swarm Agent Guide

## Boundary

`swarm` owns background agent supervision primitives, in-process runners, subprocess/stdio control, socket clients, and snapshots.

## Rules

- Child agents should be harness-based.
- Keep socket/stdio protocols testable and explicit.
- Preserve durable event log behavior and snapshot metadata.
- Do not put TUI dashboard rendering here.

## Quirks

- Tests and smoke paths open local sockets; sandboxed runs may need permission.
- State values are `running`, `settled`, and `error`.

