# Erasmus Agent Guide

This file is the project-level map for contributors and coding agents. More specific `AGENTS.md` files in subdirectories override or refine this guidance.

## Project Shape

- `cmd/erasmus`: CLI entrypoint. Uses Fang v2 over Cobra with pflag command wiring and Viper-backed root settings.
- `packages/message`, `packages/event`: canonical runtime data types.
- `packages/model`, `packages/provider`, `packages/auth`: model metadata, provider streaming, and credentials.
- `packages/tool`, `packages/tools`, `packages/sandbox`: tool interfaces, built-in coding tools, and filesystem/process policy.
- `packages/loop`: low-level provider/tool loop. No sessions, CLI, TUI, or persistence.
- `packages/agent`: in-memory stateful wrapper around `loop`.
- `packages/harness`: central durable runtime orchestration layer.
- `packages/session`: session interfaces plus `jsonl` and `memory` implementations.
- `packages/prompt`, `packages/skill`, `packages/compact`: prompt resources, skill discovery, and compaction.
- `packages/extension`: JSON-line extension protocol, subprocess host, tools, commands, and hooks.
- `packages/extension/sdk`: thin Go helpers for authoring JSON-line extension subprocesses.
- `packages/rpc`: JSON-lines RPC server around harness runtimes.
- `packages/swarm`: background agent supervision and socket/stdio clients.
- `packages/tui`: line-oriented and full-screen Bubble Tea TUI.
- `packages/app`: application composition and frontend wiring. CLI handlers should delegate here when behavior belongs outside the CLI.
- `examples/godom`: browser example module using godom.
- `examples/extension-go`: minimal Go extension subprocess example using the extension SDK.

## Dependency Direction

- Runtime semantics belong in `harness` or lower, not in CLI/TUI/RPC.
- `loop` must stay headless and persistence-free.
- `agent` must stay UI-free and session-free.
- `harness` may own persistence, prompt resources, model state, tools, compaction, session tree operations, and runtime events.
- Frontends observe events and call app/harness services; they should not inspect private runtime internals.
- Do not import TUI packages below `packages/tui`.
- Prefer canonical `message` and `event` types over provider-specific types outside provider adapters.

## Current Config And Tooling

- Use `make test`, `make lint`, `make smoke`, `make build`, or `make ci`.
- The Makefile auto-detects Linuxbrew Go under `/home/linuxbrew/.linuxbrew/bin` and uses `/tmp` caches for sandbox-friendly runs.
- `make test`, `make smoke`, and `make ci` may need local socket permission because httptest and swarm tests open localhost listeners.
- `docs/prerelease/` is intentionally ignored; it contains early planning notes and is not meant for the eventual repository.
- `docs/extensions.md` documents the public extension protocol, Go SDK, CLI commands, diagnostics, and current gaps.
- Default user config/auth/session/swarm storage is XDG-compliant unless env vars or root flags override it.

## Editing Rules

- Keep changes scoped. Avoid unrelated refactors.
- Hard requirement: every commit must pass `make ci` error-free before it is created.
- Add tests before implementation for behavior changes.
- Use `gofmt` only on Go files.
- Prefer existing package patterns over new abstractions.
- Preserve fake-provider smoke paths; real-provider credentials must not be required for tests.
- Do not introduce old `github.com/charmbracelet/...` TUI imports unless deliberately migrating away from `charm.land` v2.
