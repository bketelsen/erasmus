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
	stateHome := filepath.Join(t.TempDir(), "state")
	t.Setenv("XDG_STATE_HOME", stateHome)
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
			`{"id":"4","method":"runtime_extension_diagnostics","params":{"runtime_id":"main"}}` + "\n" +
			`{"id":"5","method":"runtime_close","params":{"runtime_id":"main"}}` + "\n")
	var out bytes.Buffer
	if err := RunRPCFake(context.Background(), in, &out, ""); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{`"hello command"`, `"actions"`, `"hello world"`, filepath.Join(stateHome, "erasmus", "extensions", "logs"), `"status":"closed"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %s:\n%s", want, got)
		}
	}
}

func TestRunRPCExtensionCommandErrorIncludesPersistentLogPath(t *testing.T) {
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
        print(json.dumps({"type":"command_result","id":frame.get("id"),"data":{"error":"rpc command failed loudly"}}), flush=True)
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	in := strings.NewReader(
		`{"id":"1","method":"runtime_create","params":{"id":"main","extensions":[{"command":` + quote(path) + `}]}}` + "\n" +
			`{"id":"2","method":"runtime_extension_command","params":{"runtime_id":"main","command":"fail"}}` + "\n" +
			`{"id":"3","method":"runtime_close","params":{"runtime_id":"main"}}` + "\n")
	var out bytes.Buffer
	if err := RunRPCFake(context.Background(), in, &out, ""); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "rpc command failed loudly") || !strings.Contains(got, filepath.Join(stateHome, "erasmus", "extensions", "logs")) {
		t.Fatalf("output missing command failure/log path:\n%s", got)
	}
}

func TestRunRPCForwardsRuntimeEventsToExtensions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	dir := t.TempDir()
	stateHome := filepath.Join(dir, "state")
	t.Setenv("XDG_STATE_HOME", stateHome)
	sessionPath := filepath.Join(dir, "session.jsonl")
	path := filepath.Join(dir, "events.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"event-test","version":"1"}}), flush=True)
print(json.dumps({"type":"subscribe","data":{"events":["settled"]}}), flush=True)
for line in sys.stdin:
    frame = json.loads(line)
    if frame.get("type") == "event" and frame.get("data", {}).get("type") == "settled":
        print(json.dumps({"type":"host_action","data":{"type":"save_point","data":{"label":"rpc-settled","data":{"source":"extension"}}}}), flush=True)
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	in := strings.NewReader(
		`{"id":"1","method":"runtime_create","params":{"id":"main","session_path":` + quote(sessionPath) + `,"extensions":[{"command":` + quote(path) + `}]}}` + "\n" +
			`{"id":"2","method":"runtime_prompt","params":{"runtime_id":"main","text":"hello"}}` + "\n" +
			`{"id":"3","method":"runtime_wait","params":{"runtime_id":"main"}}` + "\n")
	var out bytes.Buffer
	if err := RunRPCFake(context.Background(), in, &out, ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"custom_type":"checkpoint"`) || !strings.Contains(string(data), `"label":"rpc-settled"`) {
		t.Fatalf("session log missing extension checkpoint:\n%s\nrpc output:\n%s", data, out.String())
	}
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
