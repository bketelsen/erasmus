package extension_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"erasmus/packages/event"
	"erasmus/packages/extension"
	"erasmus/packages/extension/proto"
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

func TestProcessForwardsSubscribedRuntimeEvents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	path := filepath.Join(t.TempDir(), "ext.sh")
	script := `#!/usr/bin/env bash
printf '%s\n' '{"type":"hello","data":{"name":"event-test","version":"1"}}'
printf '%s\n' '{"type":"subscribe","data":{"events":["settled"]}}'
while IFS= read -r line; do
  case "$line" in
    *'"type":"event"'*settled*)
      printf '%s\n' '{"type":"host_action","data":{"type":"print","data":{"text":"saw settled"}}}'
      ;;
  esac
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

	if err := proc.PublishEvent(ctx, event.Settled{}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		actions := proc.Manager().DrainHostActions()
		for _, action := range actions {
			if action.Type == "print" && string(action.Data) == `{"text":"saw settled"}` {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("extension did not observe settled event")
}

func TestProcessCallsSubscribedRuntimeHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	path := filepath.Join(t.TempDir(), "ext.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"hook-test","version":"1"}}), flush=True)
print(json.dumps({"type":"subscribe_hooks","data":{"hooks":["provider_request"]}}), flush=True)
for line in sys.stdin:
    frame = json.loads(line)
    if frame.get("type") == "hook_call" and frame.get("data", {}).get("hook") == "provider_request":
        print(json.dumps({"type":"hook_result","id":frame.get("id"),"data":{"id":frame.get("id"),"deny":True,"error":"blocked by hook"}}), flush=True)
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
	if !proc.HookSubscribed("provider_request") {
		t.Fatal("hook not subscribed")
	}
	res, err := proc.CallHook(ctx, proto.HookCall{ID: "hook-1", Hook: "provider_request"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Deny || res.Error != "blocked by hook" {
		t.Fatalf("result = %+v", res)
	}
}
