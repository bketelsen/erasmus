# packages/event Agent Guide

## Boundary

`event` defines canonical runtime events shared by loop, agent, harness, and frontends.

## Rules

- Events are the frontend integration boundary. Keep them stable and UI-neutral.
- Do not import frontend packages.
- Add event fields conservatively and preserve JSON-friendly shapes where possible.

## Quirks

- UIs should be able to render from events plus `State()` without inspecting private runtime state.

