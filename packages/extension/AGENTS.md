# packages/extension Agent Guide

## Boundary

`extension` owns the JSON-line extension host, subprocess lifecycle, diagnostics, extension tools, commands, and harness hook integration.

## Rules

- Extensions must stay headless. Do not depend on TUI behavior directly.
- Extension tools must satisfy `tool.Tool` and use canonical message content for results.
- Surface subprocess stderr and invalid stdout diagnostics in startup errors.
- Keep recent in-memory diagnostics until persistent logs are implemented.

## Quirks

- Startup registration uses a quiet-period/deadline strategy to avoid racing process output.
- Persistent structured extension logs are planned.

