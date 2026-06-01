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

func TestRunConfiguredIncludesExtensionRegisteredSkills(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	path := filepath.Join(dir, "ext.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"skill-test","version":"1"}}), flush=True)
print(json.dumps({"type":"register_skill","data":{"name":"extension-review","description":"Review from extension","body":"Review carefully."}}), flush=True)
print(json.dumps({"type":"subscribe_hooks","data":{"hooks":["provider_request"]}}), flush=True)
for line in sys.stdin:
    frame = json.loads(line)
    data = frame.get("data", {})
    if frame.get("type") == "hook_call" and data.get("hook") == "provider_request":
        system = data.get("request", {}).get("system", "")
        if "extension-review" in system and "Review from extension" in system:
            print(json.dumps({"type":"hook_result","id":frame.get("id"),"data":{"id":frame.get("id"),"deny":True,"error":"extension skill included"}}), flush=True)
        else:
            print(json.dumps({"type":"hook_result","id":frame.get("id"),"data":{"id":frame.get("id")}}), flush=True)
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := app.RunConfigured(context.Background(), app.RunOptions{Prompt: "hello", Out: &out}, config.Config{Provider: "fake", Model: "echo", Extensions: []config.ExtensionConfig{{Command: path}}}, auth.NewMemoryStore())
	if err == nil || !strings.Contains(err.Error(), "extension skill included") {
		t.Fatalf("err = %v, output:\n%s", err, out.String())
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

func TestRunConfiguredAppliesExtensionResourceMutationRequests(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	path := filepath.Join(dir, "ext.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"resource-test","version":"1"}}), flush=True)
print(json.dumps({"type":"host_action","data":{"type":"set_active_tools","data":{"names":["read"]}}}), flush=True)
for line in sys.stdin:
    pass
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := app.RunConfigured(context.Background(), app.RunOptions{Prompt: "tool write", Out: &out}, config.Config{Provider: "fake", Model: "echo", Extensions: []config.ExtensionConfig{{Command: path}}}, auth.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "fake response: tool write") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestRunConfiguredAppliesExtensionSetResourcesRequests(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	path := filepath.Join(dir, "ext.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"resources-test","version":"1"}}), flush=True)
print(json.dumps({"type":"host_action","data":{"type":"set_resources","data":{"active_tools":["read"],"skills":[{"name":"extension-review","description":"Review from extension","body":"Review carefully.","source":"extension"}]}}}), flush=True)
for line in sys.stdin:
    pass
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := app.RunConfigured(context.Background(), app.RunOptions{Prompt: "tool write", Out: &out}, config.Config{Provider: "fake", Model: "echo", Extensions: []config.ExtensionConfig{{Command: path}}}, auth.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "fake response: tool write") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestRunConfiguredAppliesExtensionSavePointRequests(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	sessionPath := filepath.Join(dir, "session.jsonl")
	path := filepath.Join(dir, "ext.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"checkpoint-test","version":"1"}}), flush=True)
print(json.dumps({"type":"host_action","data":{"type":"save_point","data":{"label":"extension-start","data":{"source":"extension"}}}}), flush=True)
for line in sys.stdin:
    pass
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := app.RunConfigured(context.Background(), app.RunOptions{Prompt: "hello", Out: &out, SessionPath: sessionPath}, config.Config{Provider: "fake", Model: "echo", Extensions: []config.ExtensionConfig{{Command: path}}}, auth.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"custom_type":"checkpoint"`) || !strings.Contains(string(data), `"label":"extension-start"`) {
		t.Fatalf("session log missing checkpoint:\n%s", data)
	}
}

func TestRunConfiguredLetsExtensionRejectProviderRequest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	path := filepath.Join(dir, "ext.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"hook-test","version":"1"}}), flush=True)
print(json.dumps({"type":"subscribe_hooks","data":{"hooks":["provider_request"]}}), flush=True)
for line in sys.stdin:
    frame = json.loads(line)
    if frame.get("type") == "hook_call" and frame.get("data", {}).get("hook") == "provider_request":
        print(json.dumps({"type":"hook_result","id":frame.get("id"),"data":{"id":frame.get("id"),"deny":True,"error":"blocked by extension"}}), flush=True)
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := app.RunConfigured(context.Background(), app.RunOptions{Prompt: "hello", Out: &out}, config.Config{Provider: "fake", Model: "echo", Extensions: []config.ExtensionConfig{{Command: path}}}, auth.NewMemoryStore())
	if err == nil || !strings.Contains(err.Error(), "blocked by extension") {
		t.Fatalf("err = %v, output:\n%s", err, out.String())
	}
}

func TestRunConfiguredLetsExtensionRejectProviderResponse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	path := filepath.Join(dir, "ext.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"response-hook-test","version":"1"}}), flush=True)
print(json.dumps({"type":"subscribe_hooks","data":{"hooks":["provider_response"]}}), flush=True)
for line in sys.stdin:
    frame = json.loads(line)
    data = frame.get("data", {})
    if frame.get("type") == "hook_call" and data.get("hook") == "provider_response":
        event_types = [event.get("type") for event in data.get("events", [])]
        if "text_delta" in event_types:
            print(json.dumps({"type":"hook_result","id":frame.get("id"),"data":{"id":frame.get("id"),"deny":True,"error":"response blocked by extension"}}), flush=True)
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := app.RunConfigured(context.Background(), app.RunOptions{Prompt: "hello", Out: &out}, config.Config{Provider: "fake", Model: "echo", Extensions: []config.ExtensionConfig{{Command: path}}}, auth.NewMemoryStore())
	if err == nil || !strings.Contains(err.Error(), "response blocked by extension") {
		t.Fatalf("err = %v, output:\n%s", err, out.String())
	}
}

func TestRunConfiguredLetsExtensionTransformContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	path := filepath.Join(dir, "ext.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"context-hook-test","version":"1"}}), flush=True)
print(json.dumps({"type":"subscribe_hooks","data":{"hooks":["context_transform"]}}), flush=True)
for line in sys.stdin:
    frame = json.loads(line)
    data = frame.get("data", {})
    if frame.get("type") == "hook_call" and data.get("hook") == "context_transform":
        messages = data.get("messages", [])
        messages[0]["content"] = [{"text":"transformed by extension"}]
        print(json.dumps({"type":"hook_result","id":frame.get("id"),"data":{"id":frame.get("id"),"messages":messages}}), flush=True)
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := app.RunConfigured(context.Background(), app.RunOptions{Prompt: "original", Out: &out}, config.Config{Provider: "fake", Model: "echo", Extensions: []config.ExtensionConfig{{Command: path}}}, auth.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "fake response: transformed by extension") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func quotePy(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}
