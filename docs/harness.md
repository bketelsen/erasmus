# Harness

`packages/harness` is the durable runtime API that frontends and embedders should drive. It sits above the agent loop and below CLI, TUI, RPC, swarm, and extension host code.

Use the harness when a caller needs a long-lived Erasmus runtime with persistent sessions, provider streaming, tools, skills, hooks, model changes, compaction, and session-tree navigation.

## Construction

Create a harness with `harness.New(ctx, harness.Config{...})`.

Required fields:

- `Session`: a `session.Session` backend.
- `Stream`: a `provider.StreamFunc`.

Common optional fields:

- `Model` and `Reasoning`: initial runtime settings. If omitted, the harness uses values replayed from the session.
- `SystemPrompt`: explicit system prompt.
- `Prompt`: prompt builder used when `SystemPrompt` is empty.
- `Skills`: prompt resources passed to the prompt builder.
- `Tools`: known tool registry.
- `ActiveTools`: selected tool names. Empty selection means the tool package default selection rules apply.
- `Hooks`: harness-level lifecycle hooks.
- `LoopHooks`: lower-level loop hooks for embedding and compatibility.
- `ConfirmToolCall`: tool confirmation callback composed into loop hooks.
- `MaxSteps`: loop step limit.

Most application frontends should not assemble this manually. Prefer `packages/app.ResolveHarnessConfig` when you need normal config, auth, provider, model catalog, tools, session, skills, and extension wiring.

## Startup Flow

`harness.New` performs this setup:

1. Replays durable context from `Session.BuildContext`.
2. Resolves model and reasoning from config or replayed session state.
3. Selects active tools from the configured registry.
4. Builds the system prompt from `Prompt` when no explicit `SystemPrompt` is supplied.
5. Composes harness hooks, loop hooks, and tool confirmation.
6. Wraps the provider stream for provider-response hooks.
7. Creates the in-memory agent and subscribes persistence handlers.

The system prompt is built at construction time. Runtime `SetSkills` and `SetResources` update harness resource state and emit resource events, but they do not rebuild the already-created agent system prompt.

## Driving Runs

Core methods:

- `Prompt(ctx, text, opts)`: append a user prompt and start a run.
- `Continue(ctx)`: continue from existing session context without a new user prompt.
- `Abort(ctx)`: request cancellation of the active run.
- `Wait(ctx)`: wait for the active run to finish.
- `State(ctx)`: read a snapshot of agent, session, and skill state.
- `Subscribe(fn)`: observe runtime events.
- `Session()`: access the backing session.

`Prompt` and `Continue` return an event channel. The harness also supports callback subscriptions through `Subscribe`, which is how frontends, RPC runtimes, swarm agents, and extension forwarding observe the same runtime stream.

## Persistence

The harness persists runtime output through its event handler:

- `event.AgentEnd` causes newly seen transcript messages to be appended to the session.
- `event.Usage` appends usage and cumulative usage.
- model, reasoning, compaction, active-tool, checkpoint, and tree mutations are persisted by the corresponding harness methods.

The harness tracks how many messages were already persisted so resumed sessions do not duplicate historical transcript entries.

## Runtime Mutators

The harness exposes resource and setting changes that frontends can use while a session is open:

- `SetModel(ctx, model)`: persist and apply a model change.
- `SetModelAndStream(ctx, model, stream)`: atomically switch provider stream and model.
- `SetReasoning(ctx, level)`: persist and apply reasoning level.
- `SetSkills(ctx, skills)`: replace skill resources and emit `resources_update`.
- `SetTools(ctx, tools, active)`: replace the known registry and active selection.
- `SetActiveTools(ctx, names)`: select from known tools.
- `SetResources(ctx, resources)`: update skills, tool registry, and active tools together.

These APIs are the preferred surface for UI controls, RPC methods, and extension host actions.

## Hooks

Harness hooks are higher-level than `loop.Hooks` and are intended for guards, policy, telemetry, and extension integration:

- `BeforeAgentStart`: inspect or patch prompt/continue requests before an agent run starts.
- `Context`: inspect, replace, or reject provider-bound canonical messages before request construction.
- `BeforeProviderRequest`: inspect, mutate, or reject provider requests.
- `AfterProviderResponse`: observe completed normalized provider streams.
- `ToolCall`: allow, deny, replace arguments, or provide a synthetic result.
- `ToolResult`: patch a completed tool result.
- `BeforeAssistantCommit`: patch an assistant message before it is committed.
- `BeforeCompact` and `AfterCompact`: observe or patch compaction.
- `SessionTree`: observe, patch, or reject tree navigation and branching.

Existing low-level loop hooks still work and are composed before matching harness hooks.

There is intentionally no provider-native payload hook in the public harness API. Provider-specific request payloads are built inside provider adapters; cross-provider policy should use canonical messages through `Context` or normalized requests through `BeforeProviderRequest`.

## Compaction And Checkpoints

`Compact(ctx, opts)` prepares a compaction request from current messages, runs it through the configured provider stream, appends a compaction entry, updates the agent transcript, and publishes `session_compact`.

`SavePoint(ctx, label, data)` appends a custom `checkpoint` entry to the session and publishes `save_point`. Save points are durable markers for extension and workflow state; they do not change the active transcript by themselves.

## Session Trees

When the backing session implements `session.Tree`, the harness exposes:

- `Tree(ctx)`: current leaf and navigable entries.
- `MoveTo(ctx, id, summary)`: move the active leaf and reload agent messages from session context.
- `Branch(ctx, id)`: create a branched session from an entry.

Memory and JSONL sessions both implement tree navigation.

## Embedding Advice

For tests, pair the harness with `packages/session/memory` and `packages/provider/fake`. For product frontends, use app-layer helpers so provider credentials, model catalog behavior, default storage paths, tools, skills, and extensions stay consistent.

Keep provider-specific types at the provider boundary. Harness callers should use `packages/message`, `packages/model`, `packages/tool`, `packages/event`, and `packages/session` types.
