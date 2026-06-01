# packages/tools Agent Guide

## Boundary

`tools` implements built-in coding tools: read, write, edit, bash, and the default registry.

## Rules

- All filesystem/process behavior must go through sandbox policy.
- Do not import CLI/TUI packages.
- Return tool errors as structured `tool.Result` content when the model can recover.
- Emit progress through callbacks only.

## Quirks

- Smoke tests exercise read/write/edit/bash through fake-provider prompts; keep command phrasing behavior stable.

