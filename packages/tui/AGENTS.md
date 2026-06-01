# packages/tui Agent Guide

## Boundary

`tui` renders harness/app state and events in line-oriented and full-screen terminal modes.

## Rules

- TUI owns rendering, keyboard handling, dialogs, and theme styling only.
- Runtime changes should call harness/app services.
- Keep line-oriented fallback working for pipes, tests, and smoke scripts.
- Use Charm v2 imports from `charm.land`.

## Quirks

- Bubble Tea models are copied by value; do not store `strings.Builder` or other unsafe-to-copy state in the model.
- Full-screen mode is selected only when both stdin and stdout are terminals.
- Text must fit in compact terminal layouts; avoid in-app explanatory text that duplicates help.

