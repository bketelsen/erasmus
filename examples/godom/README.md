# Erasmus godom example

This example builds a local browser UI with [`github.com/anupshinde/godom`](https://github.com/anupshinde/godom) on top of Erasmus runtime packages.

It is intentionally broader than a counter demo. The app exercises:

- `harness.Harness` prompt, continue, abort, event subscription, state snapshots, model changes, reasoning changes, skill updates, and compaction.
- Durable JSONL sessions under `.erasmus/examples/godom/session.jsonl`.
- Built-in sandboxed tools: `read`, `write`, `edit`, and `bash`.
- Skill discovery from `.erasmus/examples/godom/skills`.
- Runtime event rendering: message deltas, tool start/end, usage, resources, model/reasoning, and compaction.
- In-process `swarm.Swarm` workers with per-agent JSONL sessions and event logs.
- godom directives: `g-text`, `g-bind`, `g-show`, `g-click`, `g-for`, presentational/static assets, stateful components, props, and `Emit`.

The provider is a deterministic local fake stream, so no OpenAI/Codex credentials are needed.

## Run

From this directory:

```sh
go run . --no-browser
```

Or from the repository root:

```sh
(cd examples/godom && go run . --no-browser)
```

Remove `--no-browser` if you want godom to open your default browser automatically.

## Suggested clicks

1. Click **Run prompt** with the default prompt. It should call the `write` tool and persist the turn.
2. Change the prompt to include `read`, `bash`, or `edit` and run again.
3. Click **Compact** to summarize earlier transcript state.
4. Click **Switch model** and **Toggle reasoning** to exercise runtime state updates.
5. Click **Spawn worker** to run a supervised swarm worker, then **Send follow-up**.
6. Click tool/skill cards; these are stateful godom components that emit events to the root app.
