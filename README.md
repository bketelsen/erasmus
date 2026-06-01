# Erasmus

Erasmus is a Go-native agent harness and terminal product. The core runtime is reusable: CLI, TUI, RPC, swarm, examples, and extensions all build on the same durable harness.

Status: prerelease. APIs and file formats are still allowed to change before `v0.1.0`.

## What It Provides

- durable JSONL sessions with tree navigation and branching,
- provider-independent message, tool, event, and session types,
- OpenAI API-key and OpenAI Codex subscription provider paths,
- runtime model, reasoning, skill, and tool resource mutation,
- compaction and checkpoints,
- subprocess extensions with tools, commands, skills, hooks, resource actions, and runtime event subscriptions,
- terminal UI, JSON-lines RPC, and background swarm frontends,
- examples for web and extension embedding.

## Build

Prerequisite: Go `1.26.3` or newer.

```sh
make doctor
make build
```

The binary is written to `./erasmus` by default. Override common paths when needed:

```sh
make build GO=/path/to/go BINARY=bin/erasmus
```

## Test

```sh
make test
make test-examples
make ci
```

`make ci` runs tests, vet, lint, smoke coverage, example validation, and a build. The Makefile prints install suggestions for missing external tools such as `go`, `golangci-lint`, and `goreleaser`.

## Basic Usage

List configured models:

```sh
./erasmus models
```

Run a one-shot prompt:

```sh
./erasmus run "hello"
```

Start the TUI:

```sh
./erasmus tui
```

Use `./erasmus --help` and subcommand help for the full CLI surface.

## Storage

User-level storage follows XDG directories:

- config under `$XDG_CONFIG_HOME/erasmus` or `~/.config/erasmus`,
- auth/data under `$XDG_DATA_HOME/erasmus` or `~/.local/share/erasmus`,
- runtime state such as TUI sessions and swarm metadata under `$XDG_STATE_HOME/erasmus` or `~/.local/state/erasmus`,
- model catalog cache under `$XDG_CACHE_HOME/erasmus` or `~/.cache/erasmus`.

TUI sessions can be overridden with `--session`, `--memory`, or `ERASMUS_SESSION_DIR`.

## Documentation

- [Architecture](docs/architecture.md)
- [Harness API](docs/harness.md)
- [Sessions](docs/sessions.md)
- [Extensions](docs/extensions.md)
- [RPC](docs/rpc.md)
- [Swarm](docs/swarm.md)
- [TUI](docs/tui.md)

## Examples

- `examples/godom`: browser-based harness application using godom.
- `examples/extension-go`: minimal Go subprocess extension.

Validate examples with:

```sh
make test-examples
```

## Acknowledgements

Erasmus is inspired by two projects:

- [zot](https://github.com/patriceckhart/zot) for practical terminal coding-agent capabilities, including sessions, tools, extensions, auth/model workflows, compaction, swarm, TUI, and RPC ideas.
- [Pi](https://github.com/earendil-works/pi) for the clean layered shape around loop, agent, harness, and apps.

Erasmus does not aim to preserve zot or Pi storage, config, protocol, CLI, or SDK compatibility.

## License

MIT. See [LICENSE](LICENSE).
