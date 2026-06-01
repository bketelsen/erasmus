# Sessions

`packages/session` defines the durable transcript interface used by the harness. Sessions store replayable context and runtime metadata without depending on provider-specific message formats.

Current backends:

- `packages/session/memory`: in-memory sessions for tests and lightweight embedding.
- `packages/session/jsonl`: append-only JSON-lines sessions for durable local use.

## Session Interface

Every backend implements `session.Session`:

- `ID()`: stable session identifier.
- `Metadata(ctx)`: ID, working directory, creation time, and update time.
- `BuildContext(ctx)`: replay the active branch into messages, usage, model, reasoning, and active tools.
- `AppendMessage(ctx, msg)`: append a transcript message.
- `AppendUsage(ctx, usage, cumulative)`: append usage accounting.
- `AppendModelChange(ctx, provider, model)`: persist model selection.
- `AppendReasoningChange(ctx, level)`: persist reasoning selection.
- `AppendActiveToolsChange(ctx, names)`: persist active tool names.
- `AppendCompaction(ctx, compaction)`: persist a compaction summary.
- `AppendCustom(ctx, typ, data)`: append backend-owned or feature-owned metadata.
- `Close(ctx)`: release backend resources.

`BuildContext` returns `session.Context`, which contains the replayed transcript and the latest durable runtime settings. Harness construction uses this context to resume sessions.

## Message Shape

Sessions store provider-independent `packages/message` values. Supported content parts include:

- text,
- images,
- tool calls,
- tool results,
- reasoning summaries or encrypted reasoning payloads,
- custom content.

Provider packages are responsible for adapting these canonical messages into provider-native request payloads.

## JSONL Backend

Open or create a durable session with:

```go
s, err := jsonl.Open(path, session.Metadata{ID: filepath.Base(path), CWD: cwd})
```

The JSONL backend creates parent directories, opens the file append-only, and writes a metadata entry for a new file. Files are mode `0600`.

Each line is a JSON object with a `type`. Current entry types include:

- `meta`,
- `message`,
- `usage`,
- `model_change`,
- `reasoning_change`,
- `active_tools_change`,
- `compaction`,
- `custom`,
- `leaf`.

`leaf` entries record the active branch tip. On replay, JSONL follows parent links from the active leaf so abandoned branches remain in the file but do not appear in the current context.

The JSONL file is the durable local artifact, but its internal wire format is not yet promised as a stable public import/export format.

## Tree And Branching

Backends can optionally implement `session.Tree`:

- `LeafID(ctx)`: active branch tip.
- `MoveTo(ctx, id, summary)`: move the active branch tip to an existing entry.
- `Branch(ctx, at)`: create a new session whose active leaf starts at an existing entry.
- `Entries(ctx)`: list navigable entries with IDs, parents, types, and timestamps.

Both memory and JSONL sessions implement this interface. The harness exposes tree support through `Tree`, `MoveTo`, and `Branch` and reloads agent messages after moving the active leaf.

When `MoveTo` receives a branch summary, the backend appends a custom `branch_summary` marker after moving.

## Compaction

A compaction entry stores a summary and optional metadata. During replay, compaction replaces earlier transcript context with one system message containing the summary, then subsequent active-branch messages continue after it.

The harness `Compact` method is responsible for generating the summary, appending the compaction, appending retained messages, updating in-memory agent state, and publishing runtime events.

## Checkpoints And Custom Entries

`AppendCustom` stores feature-owned JSON data. The harness uses it for `checkpoint` entries through `SavePoint(ctx, label, data)`.

Custom entries are durable markers. They are included in tree listings, but they do not become transcript messages unless a future feature explicitly replays them into context.

## Default Storage

Session storage location is chosen by the app layer, not the session package.

The TUI default is:

```text
$XDG_STATE_HOME/erasmus/sessions/<project-key>/default.jsonl
```

If `XDG_STATE_HOME` is unset, Erasmus falls back to `~/.local/state`. The `ERASMUS_SESSION_DIR` environment variable overrides the default session directory, and CLI/TUI flags can pass explicit session paths.

Config and auth defaults use XDG config/data directories from `cmd/erasmus`; model catalog cache uses the app cache directory.

## Migration Stance

No compatibility or migration layer for another project's session format is planned. Erasmus sessions should keep native package boundaries and add explicit import/export commands only if a real workflow needs them.
