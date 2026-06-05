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

	"github.com/bketelsen/erasmus/packages/app"
	"github.com/bketelsen/erasmus/packages/config"
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

func TestExtensionDoctorConfiguredReportsHealthyExtension(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	path := filepath.Join(t.TempDir(), "doctor-ext.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"doctor-test","version":"1"}}), flush=True)
print(json.dumps({"type":"register_tool","data":{"name":"echo_ext","description":"echo extension","schema":{"type":"object"}}}), flush=True)
print(json.dumps({"type":"register_command","data":{"name":"hello","description":"hello command"}}), flush=True)
print(json.dumps({"type":"subscribe_hooks","data":{"hooks":["provider_request"]}}), flush=True)
for line in sys.stdin:
    pass
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := app.ExtensionDoctorConfigured(context.Background(), &out, config.Config{Extensions: []config.ExtensionConfig{{Command: path}}})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"extension 1\tOK\t" + path,
		"protocol\tdoctor-test\t1",
		"tool\techo_ext\techo extension",
		"command\thello\thello command",
		"hook\tprovider_request",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, got)
		}
	}
}

func TestExtensionDoctorConfiguredReportsMissingExecutable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-extension")
	var out bytes.Buffer
	err := app.ExtensionDoctorConfigured(context.Background(), &out, config.Config{Extensions: []config.ExtensionConfig{{Command: missing}}})
	if err == nil {
		t.Fatal("expected doctor error")
	}
	got := out.String()
	if !strings.Contains(got, "extension 1\tFAIL\t"+missing) || !strings.Contains(got, "diagnostic\t") {
		t.Fatalf("doctor output missing failure diagnostic:\n%s", got)
	}
	if !strings.Contains(err.Error(), "1 extension diagnostic failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestExtensionDoctorConfiguredReportsInvalidToolSchema(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	path := filepath.Join(t.TempDir(), "bad-schema.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"bad-schema","version":"1"}}), flush=True)
print(json.dumps({"type":"register_tool","data":{"name":"bad_tool","description":"bad tool","schema":"not an object"}}), flush=True)
for line in sys.stdin:
    pass
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := app.ExtensionDoctorConfigured(context.Background(), &out, config.Config{Extensions: []config.ExtensionConfig{{Command: path}}})
	if err == nil {
		t.Fatal("expected doctor error")
	}
	got := out.String()
	if !strings.Contains(got, "extension 1\tFAIL\t"+path) || !strings.Contains(got, "invalid tool schema for bad_tool") {
		t.Fatalf("doctor output missing invalid schema diagnostic:\n%s", got)
	}
}

func TestExtensionDoctorConfiguredReportsBadProtocolFrame(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	path := filepath.Join(t.TempDir(), "bad-protocol.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"bad-protocol","version":"1"}}), flush=True)
print("not json", flush=True)
for line in sys.stdin:
    pass
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := app.ExtensionDoctorConfigured(context.Background(), &out, config.Config{Extensions: []config.ExtensionConfig{{Command: path}}})
	if err == nil {
		t.Fatal("expected doctor error")
	}
	got := out.String()
	if !strings.Contains(got, "extension 1\tFAIL\t"+path) || !strings.Contains(got, "invalid JSON frame") {
		t.Fatalf("doctor output missing bad protocol diagnostic:\n%s", got)
	}
}
