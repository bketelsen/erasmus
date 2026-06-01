# packages/app Agent Guide

## Boundary

`app` is composition glue for frontends. It resolves config/auth/model/tools/sessions and builds harnesses for CLI, TUI, RPC, swarm, extensions, and examples.

## Rules

- Put frontend construction and command-handler logic here when it is shared or should stay out of `cmd/erasmus`.
- Do not put core runtime semantics here if they belong in `harness`, `agent`, or `loop`.
- Keep fake-provider paths isolated from local real-provider config/auth.
- Preserve env overrides such as `ERASMUS_CONFIG_FILE` and `ERASMUS_AUTH_FILE`.

## Quirks

- Some tests open localhost sockets through httptest or swarm; sandboxed runs may need local socket permission.
- CLI user-level config/auth storage is resolved in `cmd/erasmus`; TUI sessions and swarm state default to XDG state directories in this package.
