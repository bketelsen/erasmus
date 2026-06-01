# Erasmus godom chat example

This example builds a local browser chat UI with [`github.com/anupshinde/godom`](https://github.com/anupshinde/godom) on top of Erasmus runtime packages.

The first screen is a simple assistant chat: durable chat history, an input box, and a send button. The sidebar keeps a compact view of session status and runtime events.

The app also exercises:

- `harness.Harness` prompt, continue, abort, event subscription, state snapshots, and compaction.
- Durable JSONL sessions under `.erasmus/examples/godom/session.jsonl`.
- Built-in sandboxed tools: `read`, `write`, `edit`, and `bash`.
- Skill discovery from `.erasmus/examples/godom/skills`.
- Runtime event rendering: message deltas, tool start/end, usage, resources, and compaction.
- godom directives: `g-text`, `g-bind`, `g-show`, `g-click`, and `g-for`.

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

## Live providers

Run with `--live` to use your normal Erasmus config and auth store instead of the deterministic fake demo provider:

```sh
go run . --live --provider github-copilot --model gpt-4.1 --no-browser
```

Live mode skips demo skills, the fake provider, and the demo swarm. It also disables tools by default so the app behaves like a simple provider-backed chat surface. The provider must already be authenticated with the main Erasmus CLI.

Useful live flags:

```text
--provider <id>       provider override, such as openai, openai-codex, or github-copilot
--model <id>          model override
--reasoning <level>   reasoning override
--config <path>       Erasmus config file override
--auth-file <path>    Erasmus auth file override
--session <path>      JSONL chat session path override
```

## Suggested clicks

1. Type a message and click **Send**. The deterministic fake provider replies without credentials.
2. Ask for `read`, `write`, `bash`, or `edit` to exercise the built-in tool loop.
3. Click **Compact** to summarize earlier transcript state.
4. Watch the **Events** panel update as message deltas, tool calls, usage, and compaction events arrive.
