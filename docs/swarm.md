# Swarm

Swarm runs background Erasmus agents on the same harness runtime used by CLI, TUI, and RPC. It supports one-shot task execution, long-lived socket servers, agent control commands, event logs, and TUI dashboard integration.

Swarm is useful when a caller needs multiple durable agents or a background process that can be inspected and controlled later.

## One-Shot Tasks

Run a task in-process:

```sh
./erasmus swarm run "summarize this repository"
```

Run with explicit session storage:

```sh
./erasmus swarm run --session /tmp/agent.jsonl "continue the task"
./erasmus swarm run --session-dir /tmp/erasmus-sessions "start with a generated session"
./erasmus swarm run --memory "throwaway task"
```

Run through a subprocess:

```sh
./erasmus swarm run --subprocess "background-safe task"
```

Use only one of `--session` and `--session-dir`.

## Socket Server

Start a long-lived swarm server:

```sh
./erasmus swarm serve --socket /tmp/erasmus.sock --name local
```

The server registers itself in the local swarm registry when `--name` is supplied. Registry state lives under the app XDG state directory.

List and clean registry entries:

```sh
./erasmus swarm ps
./erasmus swarm prune
```

Close or inspect a server by name:

```sh
./erasmus swarm status local
./erasmus swarm close local
```

Most control commands accept either `--socket <addr>` or `--name <name>`.

## Agents

Spawn an agent:

```sh
./erasmus swarm spawn --name local --id review "review the latest diff"
```

Send text and wait:

```sh
./erasmus swarm send --name local review "focus on tests"
./erasmus swarm wait --name local review
```

Stop an agent:

```sh
./erasmus swarm stop --name local review
```

Spawn can use `--memory` for an in-memory agent session. Without `--memory`, agents use durable session paths managed by the swarm runner.

## Dashboard, Logs, And Attach

Render one dashboard snapshot:

```sh
./erasmus swarm dashboard local --once
```

Watch repeatedly:

```sh
./erasmus swarm watch local --interval 2s
```

Print an agent event log:

```sh
./erasmus swarm logs local review
```

Attach to an agent with a line-oriented send/wait loop:

```sh
./erasmus swarm attach local review
```

Type `/quit` or `/exit` to leave attach mode.

The full-screen TUI also exposes a swarm dashboard with `ctrl+w`.

## Status Shape

Swarm status includes server metadata and per-agent summaries:

- process ID,
- socket address,
- working directory,
- provider and model,
- uptime,
- agent ID and task,
- state/running status,
- message count,
- pending tool count,
- event count and last event type,
- session ID,
- event-log path,
- latest error when present.

The CLI dashboard prints a compact text view of this status. Socket clients receive JSON.

## Extension Background Actions

Configured extensions can request swarm-hosted background lifecycle actions during startup:

- `background_spawn`,
- `background_send`,
- `background_stop`.

These actions are handled by the app/swarm layer and are separate from extension runtime event subscriptions.

## Boundaries

Swarm should supervise harness runtimes; it should not own provider, tool, prompt, or session semantics. Durable transcript behavior belongs to sessions and harness, and provider/auth/model resolution belongs to the app layer.
