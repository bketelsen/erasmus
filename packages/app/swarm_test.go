package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"erasmus/packages/auth"
	"erasmus/packages/config"
)

func TestRunSwarmConfiguredPersistsSessionPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "swarm.jsonl")
	var out bytes.Buffer
	if err := RunSwarmConfigured(context.Background(), SwarmRunOptions{Task: "hello", Out: &out, SessionPath: path}, config.Config{Provider: "fake", Model: "echo"}, auth.NewMemoryStore()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hello") || !strings.Contains(string(data), "fake response: hello") {
		t.Fatalf("session did not persist transcript:\n%s", string(data))
	}
}

func TestRunSwarmConfiguredForwardsRuntimeEventsToExtensions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	sessionPath := filepath.Join(dir, "swarm.jsonl")
	path := filepath.Join(dir, "events.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"swarm-event-test","version":"1"}}), flush=True)
print(json.dumps({"type":"subscribe","data":{"events":["settled"]}}), flush=True)
for line in sys.stdin:
    frame = json.loads(line)
    if frame.get("type") == "event" and frame.get("data", {}).get("type") == "settled":
        print(json.dumps({"type":"host_action","data":{"type":"save_point","data":{"label":"swarm-settled","data":{"source":"extension"}}}}), flush=True)
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := RunSwarmConfigured(context.Background(), SwarmRunOptions{Task: "hello", Out: &out, SessionPath: sessionPath}, config.Config{Provider: "fake", Model: "echo", Extensions: []config.ExtensionConfig{{Command: path}}}, auth.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"custom_type":"checkpoint"`) || !strings.Contains(string(data), `"label":"swarm-settled"`) {
		t.Fatalf("session log missing extension checkpoint:\n%s\noutput:\n%s", data, out.String())
	}
}

func TestRunSwarmConfiguredPersistsSessionDir(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if err := RunSwarmConfigured(context.Background(), SwarmRunOptions{Task: "hello", Out: &out, SessionDir: dir}, config.Config{Provider: "fake", Model: "echo"}, auth.NewMemoryStore()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "main.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hello") || !strings.Contains(string(data), "fake response: hello") {
		t.Fatalf("session did not persist transcript:\n%s", string(data))
	}
}

func TestRunSwarmFake(t *testing.T) {
	var out bytes.Buffer
	logDir := t.TempDir()
	if err := RunSwarmFake(context.Background(), SwarmRunOptions{Task: "hello", Out: &out, EventLogDir: logDir}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "swarm agent main settled") || !strings.Contains(got, "event log:") || !strings.Contains(got, "fake response: hello") {
		t.Fatalf("unexpected output:\n%s", got)
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected event log file")
	}
}
