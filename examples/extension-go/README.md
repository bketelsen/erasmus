# Erasmus Go extension example

This example demonstrates the minimal Go authoring SDK for Erasmus JSON-line subprocess extensions.

It registers:

- `echo_go`, a tool that returns text from JSON input,
- `hello_go`, a command that asks the host to print a greeting.

Run it as a subprocess extension from the repository root:

```sh
(cd examples/extension-go && go build -o /tmp/erasmus-extension-go .)
go run ./cmd/erasmus extension list /tmp/erasmus-extension-go
```

During development, use:

```sh
cd examples/extension-go
go run .
go test ./...
```
