package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunRPCExtensionCommands(t *testing.T) {
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
        text = frame.get("data", {}).get("input", {}).get("text", "")
        print(json.dumps({"type":"command_result","id":frame.get("id"),"data":{"actions":[{"type":"print","data":{"text":"hello " + text}}]}}), flush=True)
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	in := strings.NewReader(
		`{"id":"1","method":"runtime_create","params":{"id":"main","extensions":[{"command":` + quote(path) + `}]}}` + "\n" +
			`{"id":"2","method":"runtime_extension_commands","params":{"runtime_id":"main"}}` + "\n" +
			`{"id":"3","method":"runtime_extension_command","params":{"runtime_id":"main","command":"hello","input":{"text":"world"}}}` + "\n" +
			`{"id":"4","method":"runtime_close","params":{"runtime_id":"main"}}` + "\n")
	var out bytes.Buffer
	if err := RunRPCFake(context.Background(), in, &out, ""); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{`"hello command"`, `"actions"`, `"hello world"`, `"status":"closed"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %s:\n%s", want, got)
		}
	}
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
