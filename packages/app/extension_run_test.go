package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"erasmus/packages/app"
	"erasmus/packages/auth"
	"erasmus/packages/config"
)

func TestRunConfiguredUsesExtensionTool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	path := filepath.Join(t.TempDir(), "ext.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"test","version":"1"}}), flush=True)
print(json.dumps({"type":"register_tool","data":{"name":"echo_ext","description":"echo extension"}}), flush=True)
for line in sys.stdin:
    frame = json.loads(line)
    if frame.get("type") == "tool_call":
        print(json.dumps({"type":"tool_result","id":frame.get("id"),"data":{"result":{"content":[{"text":"extension tool result"}]}}}), flush=True)
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := app.RunConfigured(context.Background(), app.RunOptions{Prompt: "use-tool echo_ext", Out: &out}, config.Config{Provider: "fake", Model: "echo", Extensions: []config.ExtensionConfig{{Command: path}}}, auth.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "extension tool result") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestRunConfiguredForwardsRuntimeEventsToExtensions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	path := filepath.Join(dir, "ext.py")
	seenPath := filepath.Join(dir, "seen.txt")
	script := `#!/usr/bin/env python3
import json, sys
seen_path = ` + quotePy(seenPath) + `
print(json.dumps({"type":"hello","data":{"name":"event-test","version":"1"}}), flush=True)
print(json.dumps({"type":"subscribe","data":{"events":["settled"]}}), flush=True)
for line in sys.stdin:
    frame = json.loads(line)
    if frame.get("type") == "event" and frame.get("data", {}).get("type") == "settled":
        with open(seen_path, "w", encoding="utf-8") as f:
            f.write("settled")
        print(json.dumps({"type":"host_action","data":{"type":"print","data":{"text":"saw settled"}}}), flush=True)
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := app.RunConfigured(context.Background(), app.RunOptions{Prompt: "hello", Out: &out}, config.Config{Provider: "fake", Model: "echo", Extensions: []config.ExtensionConfig{{Command: path}}}, auth.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(seenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "settled" {
		t.Fatalf("seen = %q", got)
	}
}

func quotePy(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}
