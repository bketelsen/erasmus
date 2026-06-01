package app_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"erasmus/packages/app"
)

func TestExtensionExecProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	path := filepath.Join(t.TempDir(), "cmd.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"test","version":"1"}}), flush=True)
print(json.dumps({"type":"register_command","data":{"name":"hello","description":"hello command"}}), flush=True)
for line in sys.stdin:
    frame = json.loads(line)
    if frame.get("type") == "command_call":
        data = frame.get("data", {})
        print(json.dumps({"type":"command_result","id":frame.get("id"),"data":{"actions":[{"type":"print","data":{"text":"hello " + data.get("input", {}).get("text", "")}}]}}), flush=True)
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := app.ExtensionExecProcess(context.Background(), &out, path, nil, "hello", "world"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "action\tprint") || !strings.Contains(out.String(), "hello world") {
		t.Fatalf("output:\n%s", out.String())
	}
}
