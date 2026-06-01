# Erasmus Architecture

Erasmus is organized around a reusable harness runtime with several frontends layered on top. The main rule is that provider APIs, UI code, and subprocess extensions stay outside the durable runtime core.

## Runtime Layers

The package flow is intentionally narrow:

```text
message -> provider -> loop -> agent -> harness -> app/frontends
```

- `packages/message` defines provider-independent transcript messages and content parts.
- `packages/provider` defines normalized provider requests and streaming events.
- `packages/loop` owns the low-level provider/tool turn cycle.
- `packages/agent` wraps the loop with mutable in-memory run state.
- `packages/harness` owns durable sessions, resources, hooks, and frontend-facing runtime APIs.
- `packages/app` resolves config/auth/provider/session concerns and wires frontends.

Provider-specific code belongs under `packages/provider/...`; UI-specific code belongs under a frontend package or `packages/app`; neither should leak into `loop` or `harness`.

## Messages And Providers

Canonical messages live in `packages/message`. Providers adapt those messages into their native request formats at the boundary. This keeps session storage, tests, tools, hooks, and frontends independent of OpenAI, Codex, or any future provider shape.

Provider clients stream normalized events:

- message start and text deltas,
- tool calls,
- usage,
- message end,
- provider errors.

The loop consumes only that normalized event stream.

## Loop And Agent

`packages/loop` handles one provider/tool cycle at a time:

1. Build provider-facing context.
2. Run context/request hooks.
3. Stream provider events.
4. Execute tool calls.
5. Commit assistant messages.
6. Repeat until the provider finishes without tool calls.

`packages/agent` owns in-memory state around the loop: current model, reasoning level, tools, transcript, pending tool calls, and streaming/error state.

## Harness

`packages/harness` is the primary embedding API. It combines:

- a session backend,
- an agent,
- prompt resource construction,
- runtime hooks,
- event publication,
- tool/skill/model/reasoning mutators,
- compaction and session tree operations.

Frontends should construct and drive a `Harness` instead of constructing loops or providers directly. The harness emits canonical runtime events that RPC, TUI, swarm, and extensions can all consume.

See [harness.md](harness.md) for the embedding API, runtime mutators, hooks, compaction, and session tree support.

## Sessions

Sessions are durable transcript stores behind `packages/session`. The JSONL implementation in `packages/session/jsonl` records metadata, messages, usage, model/reasoning changes, active-tool changes, compactions, checkpoints, and tree movement.

The harness persists new transcript messages from `AgentEnd` events. Session tree APIs let frontends inspect, move, and branch durable conversations without coupling to JSONL internals.

See [sessions.md](sessions.md) for backend interfaces, JSONL behavior, tree navigation, compaction, and default storage paths.

## Tools And Skills

Tools are runtime capabilities exposed through `packages/tool`. Built-in tools live in `packages/tools`; extension tools are adapted into the same interface.

Skills are prompt resources from `packages/skill`. The app layer discovers user/project skills and merges extension-provided skills before harness construction. Harness resource mutators can update skills and active tools at runtime and emit resource update events.

## Extensions

Extensions are headless subprocesses that speak JSON lines over stdin/stdout. The protocol is defined in `packages/extension/proto`; Go authoring helpers live in `packages/extension/sdk`.

Configured extensions can:

- register tools, commands, and skills,
- subscribe to runtime events,
- request blocking hooks for context transforms and provider request/response checks,
- request host actions such as checkpoints, active-tool/resource updates, and swarm background actions.

See [extensions.md](extensions.md) for the protocol details.

## Frontends

The `app` package is the shared composition layer for frontends. It resolves config, credentials, model catalog entries, provider streams, sessions, skills, tools, and extensions into a harness.

Current frontends:

- CLI commands in `cmd/erasmus`.
- Line-oriented and Bubble Tea TUI flows in `packages/tui`.
- JSON-lines RPC in `packages/rpc`.
- Background harness supervision in `packages/swarm`.

See [tui.md](tui.md) for TUI behavior and troubleshooting.

## Deferrals

Two extension surfaces are deliberately not part of the stable protocol right now:

- panels or other frontend-owned UI surfaces,
- dynamic mutation of registered tool definitions after startup.

No compatibility or migration layer for another project is planned. Erasmus should keep clean native package boundaries and public APIs rather than preserving external storage, config, protocol, or SDK shapes.
