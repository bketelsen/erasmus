# Erasmus Extensions

Erasmus extensions are headless subprocesses that speak newline-delimited JSON over stdin/stdout. They can register tools for agent runs and commands for explicit host actions.

The canonical wire types live in `packages/extension/proto`. The optional Go authoring helpers live in `packages/extension/sdk`.

## Protocol

Every stdout line from an extension must be one JSON frame:

```json
{"type":"hello","data":{"name":"demo","version":"v0"}}
{"type":"register_tool","data":{"name":"echo","description":"Echo text"}}
{"type":"register_command","data":{"name":"hello","description":"Print a greeting"}}
{"type":"register_skill","data":{"name":"review","description":"Review code","body":"Review carefully."}}
{"type":"subscribe","data":{"events":["settled","save_point"]}}
{"type":"subscribe_hooks","data":{"hooks":["context_transform","provider_request","provider_response"]}}
```

The host calls tools and commands by writing frames to the extension stdin:

```json
{"type":"tool_call","id":"echo-1","data":{"id":"echo-1","name":"echo","args":{"text":"hi"}}}
{"type":"command_call","id":"hello-1","data":{"id":"hello-1","name":"hello","input":{"text":"Ada"}}}
{"type":"event","id":"settled","data":{"type":"settled","data":{}}}
{"type":"hook_call","id":"context-transform-1","data":{"id":"context-transform-1","hook":"context_transform","messages":[{"role":"user","content":[{"text":"hi"}]}]}}
{"type":"hook_call","id":"provider-request-1","data":{"id":"provider-request-1","hook":"provider_request","request":{"messages":[]}}}
```

The extension answers with matching result frames:

```json
{"type":"tool_result","id":"echo-1","data":{"id":"echo-1","result":{"content":[{"text":"hi"}]}}}
{"type":"command_result","id":"hello-1","data":{"id":"hello-1","actions":[{"type":"print","data":{"text":"hello Ada"}}]}}
{"type":"host_action","data":{"type":"print","data":{"text":"saw settled"}}}
{"type":"host_action","data":{"type":"set_active_tools","data":{"names":["read","bash"]}}}
{"type":"host_action","data":{"type":"set_resources","data":{"active_tools":["read"],"skills":[{"name":"review","body":"Review carefully."}]}}}
{"type":"host_action","data":{"type":"save_point","data":{"label":"before-change","data":{"source":"extension"}}}}
{"type":"host_action","data":{"type":"background_spawn","data":{"id":"agent-1","task":"hello","session_scope":"memory"}}}
{"type":"hook_result","id":"context-transform-1","data":{"id":"context-transform-1","messages":[{"role":"user","content":[{"text":"rewritten"}]}]}}
{"type":"hook_result","id":"provider-request-1","data":{"id":"provider-request-1","deny":true,"error":"blocked by policy"}}
```

Tool result `content` uses Erasmus canonical message content. Text parts are encoded as `{"text":"..."}`.

`subscribe` lets an extension request runtime events by event type. Use `"*"` to request every forwarded event. One-shot `run`, RPC runtimes, TUI sessions, and swarm agents forward subscribed events and apply host actions emitted by event handlers. Event delivery is best-effort and currently one-way; extensions should not block host progress waiting for event acknowledgement.

`subscribe_hooks` lets an extension request blocking runtime hook calls. Supported hooks are `context_transform`, which rewrites provider-facing context messages without changing the durable transcript; `provider_request`, which runs before the provider stream begins; and `provider_response`, which runs after the provider stream completes. A hook result can deny the operation with `{"deny":true,"error":"..."}`. `context_transform` can replace context by returning `messages`, and `provider_request` can replace the provider request by returning a `request` object. Hook calls are blocking, so handlers should return quickly.

Supported host actions:

- `print`: asks the host to display text when the caller supports host output.
- `set_active_tools`: asks the host to replace the active tool selection for subsequent runs or, during one-shot startup, before the prompt begins.
- `set_resources`: asks the host to patch runtime resources. Currently supported fields are `active_tools` and `skills`; tool definitions are still supplied through startup registration.
- `save_point`: asks the host to persist a checkpoint marker when the runtime supports durable sessions.
- `background_spawn`, `background_send`, `background_stop`: ask a swarm-capable host to manage background agents. Non-swarm hosts may ignore these actions.

Registered skills are merged with project/user skills before harness construction for configured extension frontends. Use `set_resources` for later skill replacement requests.

## Raw Example

A minimal shell extension can register a tool:

```sh
#!/usr/bin/env sh
printf '%s\n' '{"type":"hello","data":{"name":"shell-demo"}}'
printf '%s\n' '{"type":"register_tool","data":{"name":"shell_echo","description":"Echo from shell"}}'
while IFS= read -r line; do
  case "$line" in
    *tool_call*)
      printf '%s\n' '{"type":"tool_result","id":"shell_echo-1","data":{"result":{"content":[{"text":"shell extension result"}]}}}'
      ;;
  esac
done
```

For real extensions, preserve the incoming frame `id` in the result.

## Go SDK

Use `packages/extension/sdk` when writing Go extensions:

```go
package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/bketelsen/erasmus/packages/extension/proto"
	"github.com/bketelsen/erasmus/packages/extension/sdk"
	"github.com/bketelsen/erasmus/packages/skill"
)

func main() {
	err := sdk.Run(context.Background(), sdk.Extension{
		Name: "go-demo",
		Events: []string{"settled"},
		Hooks: []string{"context_transform", "provider_request", "provider_response"},
		Skills: []skill.Skill{{
			Name:        "review",
			Description: "Review code",
			Body:        "Review carefully.",
		}},
		OnEvent: func(ctx context.Context, ev proto.Event) ([]proto.HostAction, error) {
			return []proto.HostAction{sdk.PrintAction("saw " + ev.Type)}, nil
		},
		OnHook: func(ctx context.Context, call proto.HookCall) (proto.HookResult, error) {
			return proto.HookResult{ID: call.ID}, nil
		},
		Tools: []sdk.Tool{{
			Name:        "echo_go",
			Description: "Echo text",
			Handler: func(ctx context.Context, args json.RawMessage) (sdk.ToolResult, error) {
				return sdk.TextResult(string(args)), nil
			},
		}},
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

See `examples/extension-go` for a complete Go module with one tool, one command, and tests.

## CLI

Inspect an extension process:

```sh
erasmus extension list <command> [args...]
```

Execute an extension command:

```sh
erasmus extension exec <process> [process-args...] -- <command> [input]
```

`input` is passed as JSON when valid JSON is supplied. Plain text is wrapped as:

```json
{"text":"..."}
```

Configure extension subprocesses for normal runs:

```sh
erasmus config set extension /path/to/extension
erasmus config set extensions /path/one,/path/two
```

Configured extension tools are merged into harness tool registries for `run`, TUI, RPC, and swarm paths.

## Diagnostics

Extension stderr and host-side protocol diagnostics are captured in recent in-memory diagnostics and persistent logs under the XDG state directory:

```text
$XDG_STATE_HOME/erasmus/extensions/logs/
```

Startup errors and command/tool failures include the log path when available. Invalid stdout JSON frames are treated as diagnostics, not valid protocol frames.

## Current Gaps

The protocol currently covers startup registration, tools, commands, command host actions, runtime event subscriptions, context transforms, provider request/response hooks, active-tool and skill resource mutation requests, diagnostics, and configured subprocess tools.

The stable protocol does not yet expose dynamic tool-definition mutation or panels. Keep extensions headless and avoid depending on a specific frontend.
