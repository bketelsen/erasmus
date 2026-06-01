package extension_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"erasmus/packages/extension"
	"erasmus/packages/message"
)

func TestProcessExtensionTool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	path := filepath.Join(t.TempDir(), "ext.sh")
	script := `#!/usr/bin/env bash
printf '%s\n' '{"type":"hello","data":{"name":"test","version":"1"}}'
printf '%s\n' '{"type":"register_tool","data":{"name":"echo_ext","description":"echo extension"}}'
while IFS= read -r line; do
  id=$(printf '%s' "$line" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
  printf '{"type":"tool_result","id":"%s","data":{"id":"%s","result":{"content":[{"Text":"hello from extension"}]}}}\n' "$id" "$id"
done
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	proc, err := extension.StartProcess(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()
	tool, ok := proc.Manager().Registry().Get("echo_ext")
	if !ok {
		t.Fatal("tool not registered")
	}
	res, err := tool.Execute(ctx, json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Content[0].(message.Text).Text; got != "hello from extension" {
		t.Fatalf("got %q", got)
	}
}
