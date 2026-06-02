# Swarm Workflow Example

This example builds an in-process Erasmus swarm server with three named agents:

- `planner`: turns the task into an implementation plan,
- `reviewer`: checks the plan for gaps and risk,
- `executor`: produces an execution summary from the approved plan and review.

The agents are supervised by `packages/swarm`. Each named agent gets its own harness, session, event stream, and event log. The example uses deterministic local fake streams so it runs without provider credentials.

## Try It

```sh
go run . --task "Add transcript export support"
```

Or pass the task as positional text:

```sh
go run . "Add transcript export support"
```

By default, event logs are written under:

```text
.erasmus/swarm-workflow/events/
```

Use `--state` to choose another directory:

```sh
go run . --state /tmp/erasmus-swarm-workflow "Plan a release checklist"
```

## What To Notice

The important pattern is the server and workflow boundary:

- `swarm.New` owns the named background agents.
- The factory builds one harness per agent name.
- `Spawn` starts `planner`, `reviewer`, and `executor` as distinct supervised agents.
- Each agent is waited independently.
- The orchestrator collects assistant deltas from each agent and passes outputs to the next phase.

This example is intentionally small. A production version would replace the fake role streams with configured provider streams and likely give the executor real tools.
