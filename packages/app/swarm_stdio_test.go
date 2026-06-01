package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"erasmus/packages/auth"
	"erasmus/packages/config"
)

func TestServeSwarmStdioSpawnWaitListSendClose(t *testing.T) {
	input := strings.Join([]string{
		`{"id":"1","method":"spawn","params":{"id":"main","task":"first","memory":true}}`,
		`{"id":"2","method":"wait","params":{"id":"main"}}`,
		`{"id":"3","method":"send","params":{"id":"main","text":"second"}}`,
		`{"id":"4","method":"wait","params":{"id":"main"}}`,
		`{"id":"5","method":"list"}`,
		`{"id":"6","method":"status"}`,
		`{"id":"7","method":"close"}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	err := ServeSwarmStdio(context.Background(), strings.NewReader(input), &out, config.Config{Provider: "fake", Model: "echo"}, auth.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 7 {
		t.Fatalf("responses = %d, output:\n%s", len(lines), out.String())
	}
	for _, line := range lines {
		var resp struct {
			ID     string          `json:"id"`
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
			Error  string          `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("bad response %q: %v", line, err)
		}
		if !resp.OK {
			t.Fatalf("response failed: %s", line)
		}
	}
	if !strings.Contains(out.String(), `"id":"main"`) || !strings.Contains(out.String(), `"running":false`) || !strings.Contains(out.String(), `"pid":`) {
		t.Fatalf("missing list/snapshot output:\n%s", out.String())
	}
}

func TestServeSwarmStdioAppliesExtensionBackgroundSpawnAction(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	path := filepath.Join(dir, "ext.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"background-test","version":"1"}}), flush=True)
print(json.dumps({"type":"host_action","data":{"type":"background_spawn","data":{"id":"from-extension","task":"hello","session_scope":"memory"}}}), flush=True)
for line in sys.stdin:
    pass
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		`{"id":"1","method":"wait","params":{"id":"from-extension"}}`,
		`{"id":"2","method":"list"}`,
		`{"id":"3","method":"close"}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	err := ServeSwarmStdio(context.Background(), strings.NewReader(input), &out, config.Config{Provider: "fake", Model: "echo", Extensions: []config.ExtensionConfig{{Command: path}}}, auth.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id":"from-extension"`) || !strings.Contains(out.String(), `"running":false`) {
		t.Fatalf("missing extension-spawned agent:\n%s", out.String())
	}
}

func TestServeSwarmStdioForwardsRuntimeEventsToExtensions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	sessionPath := filepath.Join(dir, "swarm.jsonl")
	path := filepath.Join(dir, "events.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"swarm-stdio-event-test","version":"1"}}), flush=True)
print(json.dumps({"type":"subscribe","data":{"events":["settled"]}}), flush=True)
for line in sys.stdin:
    frame = json.loads(line)
    if frame.get("type") == "event" and frame.get("data", {}).get("type") == "settled":
        print(json.dumps({"type":"host_action","data":{"type":"save_point","data":{"label":"swarm-stdio-settled","data":{"source":"extension"}}}}), flush=True)
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		`{"id":"1","method":"spawn","params":{"id":"main","task":"hello","session_path":` + quote(sessionPath) + `}}`,
		`{"id":"2","method":"wait","params":{"id":"main"}}`,
		`{"id":"3","method":"close"}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	err := ServeSwarmStdio(context.Background(), strings.NewReader(input), &out, config.Config{Provider: "fake", Model: "echo", Extensions: []config.ExtensionConfig{{Command: path}}}, auth.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"custom_type":"checkpoint"`) || !strings.Contains(string(data), `"label":"swarm-stdio-settled"`) {
		t.Fatalf("session log missing extension checkpoint:\n%s\noutput:\n%s", data, out.String())
	}
}
