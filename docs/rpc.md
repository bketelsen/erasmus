# RPC

`erasmus rpc` runs a newline-delimited JSON RPC server over stdin/stdout. Each input line is one JSON request object. Each output line is one response or event notification object.

The RPC layer is a frontend adapter over the harness and app composition APIs. Runtime semantics should stay in `packages/harness`, `packages/session`, `packages/event`, and adjacent core packages.

## Start

```sh
./erasmus rpc
```

The server uses normal Erasmus config and auth resolution. Each `runtime_create` request can override provider, model, reasoning, working directory, tools, session path, and extension subprocesses for that runtime.

## Request And Response Shape

Request:

```json
{"id":"1","method":"runtime_list","params":{}}
```

Fields:

- `id`: optional client request ID echoed in the response.
- `method`: method name.
- `params`: optional method-specific object.

Success response:

```json
{"id":"1","result":{}}
```

Error response:

```json
{"id":"1","error":"message"}
```

## Events

Long-running methods such as `runtime_prompt` and `runtime_continue` return a started response, then emit runtime events asynchronously:

```json
{"method":"runtime_event","params":{"runtime_id":"main","type":"message_delta","event":{"message_id":"assistant","text":"hello"}}}
```

`params.type` is the canonical Erasmus event type. `params.event` contains the concrete event payload.

## Runtime Lifecycle

Create a runtime:

```json
{"id":"1","method":"runtime_create","params":{"id":"main","provider":"fake","model":"echo"}}
```

Create a durable JSONL-backed runtime:

```json
{"id":"1","method":"runtime_create","params":{"id":"main","session_path":"/tmp/erasmus-main.jsonl"}}
```

Attach per-runtime extension subprocesses:

```json
{"id":"1","method":"runtime_create","params":{"id":"main","extensions":[{"command":"/path/to/ext","args":["--flag"]}]}}
```

List and close runtimes:

```json
{"id":"2","method":"runtime_list"}
{"id":"3","method":"runtime_close","params":{"runtime_id":"main"}}
```

## Runtime State And Sessions

```json
{"id":"1","method":"runtime_state","params":{"runtime_id":"main"}}
{"id":"2","method":"runtime_session","params":{"runtime_id":"main"}}
{"id":"3","method":"runtime_session_context","params":{"runtime_id":"main"}}
```

Tree-capable sessions can be inspected and navigated:

```json
{"id":"4","method":"runtime_tree","params":{"runtime_id":"main"}}
{"id":"5","method":"runtime_move_to","params":{"runtime_id":"main","entry_id":"1","summary":"Returning to earlier branch."}}
{"id":"6","method":"runtime_branch","params":{"runtime_id":"main","entry_id":"1"}}
```

## Runs

Start a prompt and wait for it to settle:

```json
{"id":"1","method":"runtime_prompt","params":{"runtime_id":"main","text":"hello"}}
{"id":"2","method":"runtime_wait","params":{"runtime_id":"main"}}
```

Continue or abort a runtime:

```json
{"id":"3","method":"runtime_continue","params":{"runtime_id":"main"}}
{"id":"4","method":"runtime_abort","params":{"runtime_id":"main"}}
```

## Runtime Settings

```json
{"id":"1","method":"runtime_set_model","params":{"runtime_id":"main","provider":"fake","model":"echo"}}
{"id":"2","method":"runtime_set_reasoning","params":{"runtime_id":"main","reasoning":"low"}}
{"id":"3","method":"runtime_reload_skills","params":{"runtime_id":"main"}}
```

Model catalog and process-local auth methods:

```json
{"id":"4","method":"models"}
{"id":"5","method":"auth_status"}
{"id":"6","method":"auth_login","params":{"provider":"fake","api_key":"secret"}}
{"id":"7","method":"auth_logout","params":{"provider":"fake"}}
```

`auth_status` returns provider names only; it does not expose secrets.

## Compaction And Checkpoints

```json
{"id":"1","method":"runtime_checkpoint","params":{"runtime_id":"main","label":"before refactor","data":{"source":"client"}}}
{"id":"2","method":"runtime_compact","params":{"runtime_id":"main","keep_tail":8,"custom_instructions":"Preserve decisions and file changes."}}
```

Checkpoints append durable custom session markers. Compaction runs through the runtime harness and provider stream.

## Extension Commands

Per-runtime extension commands can be listed, executed, and diagnosed:

```json
{"id":"1","method":"runtime_extension_commands","params":{"runtime_id":"main"}}
{"id":"2","method":"runtime_extension_command","params":{"runtime_id":"main","command":"hello","input":{"text":"world"}}}
{"id":"3","method":"runtime_extension_diagnostics","params":{"runtime_id":"main"}}
```

Extension tools are merged into the runtime harness at creation time. Extension runtime event subscriptions and host actions are handled by the app layer while the runtime is active.

## Single-Harness Server

`packages/rpc.Server` is a smaller single-harness JSON-lines adapter used by tests and embedding. It supports `state`, `session`, `session_context`, `models`, `auth_status`, `prompt`, `continue`, `abort`, and `wait`.

Prefer `erasmus rpc` or `packages/rpc.MultiServer` for product frontends.
