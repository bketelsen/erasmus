# packages/prompt Agent Guide

## Boundary

`prompt` builds system prompt text from model state, active tools, session metadata, and resources such as skills.

## Rules

- Keep prompt construction deterministic and headless.
- Do not resolve files, config, or auth here.
- Frontends should not assemble system prompts directly.

## Quirks

- Prompt changes can affect every frontend; use focused tests around expected prompt content.

