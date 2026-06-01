package app_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"erasmus/packages/app"
)

func TestExtensionListProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	path := filepath.Join(t.TempDir(), "ext.sh")
	script := `#!/usr/bin/env bash
printf '%s\n' '{"type":"hello","data":{"name":"test","version":"1"}}'
printf '%s\n' '{"type":"register_tool","data":{"name":"echo_ext","description":"echo extension"}}'
printf '%s\n' '{"type":"register_command","data":{"name":"hello","description":"hello command"}}'
while IFS= read -r line; do :; done
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := app.ExtensionListProcess(context.Background(), &out, path); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "tool\techo_ext\techo extension") || !strings.Contains(out.String(), "command\thello\thello command") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestExtensionListProcessStartupErrorIncludesPersistentLogPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	stateHome := filepath.Join(t.TempDir(), "state")
	t.Setenv("XDG_STATE_HOME", stateHome)
	path := filepath.Join(t.TempDir(), "bad-ext.sh")
	script := `#!/usr/bin/env bash
printf '%s\n' 'not json'
printf '%s\n' 'startup failed loudly' >&2
sleep 3
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	err := app.ExtensionListProcess(context.Background(), io.Discard, path)
	if err == nil {
		t.Fatal("expected startup error")
	}
	got := err.Error()
	if !strings.Contains(got, filepath.Join(stateHome, "erasmus", "extensions", "logs")) {
		t.Fatalf("error missing extension log path:\n%s", got)
	}
	if !strings.Contains(got, "startup failed loudly") {
		t.Fatalf("error missing stderr diagnostic:\n%s", got)
	}
	matches, globErr := filepath.Glob(filepath.Join(stateHome, "erasmus", "extensions", "logs", "*.jsonl"))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(matches) != 1 {
		t.Fatalf("log files = %v, want one log file", matches)
	}
	data, readErr := os.ReadFile(matches[0])
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(data), "startup failed loudly") || !strings.Contains(string(data), "invalid JSON frame") {
		t.Fatalf("log content missing diagnostics:\n%s", string(data))
	}
}

func TestExtensionExecProcessCommandErrorIncludesPersistentLogPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	stateHome := filepath.Join(t.TempDir(), "state")
	t.Setenv("XDG_STATE_HOME", stateHome)
	path := filepath.Join(t.TempDir(), "cmd.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"test","version":"1"}}), flush=True)
print(json.dumps({"type":"register_command","data":{"name":"fail","description":"fail command"}}), flush=True)
for line in sys.stdin:
    frame = json.loads(line)
    if frame.get("type") == "command_call":
        print(json.dumps({"type":"command_result","id":frame.get("id"),"data":{"error":"command failed loudly"}}), flush=True)
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	err := app.ExtensionExecProcess(context.Background(), io.Discard, path, nil, "fail", "")
	if err == nil {
		t.Fatal("expected command error")
	}
	got := err.Error()
	if !strings.Contains(got, "command failed loudly") || !strings.Contains(got, filepath.Join(stateHome, "erasmus", "extensions", "logs")) {
		t.Fatalf("error missing command failure/log path:\n%s", got)
	}
}
