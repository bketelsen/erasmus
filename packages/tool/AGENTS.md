# packages/tool Agent Guide

## Boundary

`tool` defines tool interfaces, specs, registry behavior, progress, and results.

## Rules

- Keep this package independent from built-in tool implementations.
- Tool schemas should remain stable and provider-adaptable.
- Results should use canonical `message.Content`.

## Quirks

- Tool execution policy metadata may expand; avoid hardcoding built-in tool assumptions here.

