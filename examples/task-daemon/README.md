# Task Daemon Example

This example embeds the Erasmus harness in a small autonomous daemon. The daemon watches a local Markdown inbox, runs each task through a durable harness session, and writes inspectable outputs for every run.

The CLI here only starts the daemon. The agent behavior is driven through `packages/harness`, with JSONL sessions, runtime events, sandboxed tools, and per-task output directories.

## Try it

```sh
go run . --once
```

By default the example reads tasks from `./inbox`, writes results to `./out`, and stores sessions under `.erasmus/task-daemon`.

Run continuously:

```sh
go run . --watch --interval 30s
```

Add a task by dropping a Markdown file into the inbox:

```md
# README review

Review the README and suggest one improvement.
```

Each processed task gets an output directory containing:

- `summary.md`: assistant text from the run,
- `events.jsonl`: harness runtime events,
- `status.json`: task status and session path,
- `session.jsonl`: durable transcript/session data.

The default provider is deterministic and local so the example can run without credentials. It is intentionally simple: the point is to show the daemon pattern around the harness SDK.
