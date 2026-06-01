package extension_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"erasmus/packages/extension"
)

func TestProcessStartupDiagnosticsIncludeStderrAndInvalidStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	path := filepath.Join(t.TempDir(), "bad-ext.sh")
	script := `#!/usr/bin/env bash
printf '%s\n' 'not json'
printf '%s\n' 'startup failed loudly' >&2
sleep 3
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := extension.StartProcess(ctx, path)
	if err == nil {
		t.Fatal("expected startup error")
	}
	got := err.Error()
	for _, want := range []string{"no startup frames", "invalid JSON frame", "startup failed loudly", "recent extension diagnostics"} {
		if !strings.Contains(got, want) {
			t.Fatalf("error missing %q:\n%s", want, got)
		}
	}
}

func TestProcessPersistentDiagnosticsLogIncludesStartupFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	path := filepath.Join(t.TempDir(), "bad-ext.sh")
	script := `#!/usr/bin/env bash
printf '%s\n' 'not json'
printf '%s\n' 'startup failed loudly' >&2
sleep 3
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "logs", "bad-ext.jsonl")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := extension.StartProcessWithOptions(ctx, path, extension.ProcessOptions{LogPath: logPath})
	if err == nil {
		t.Fatal("expected startup error")
	}
	if !strings.Contains(err.Error(), "extension log: "+logPath) {
		t.Fatalf("error missing log path:\n%s", err)
	}
	data, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Fatalf("log lines = %q, want stderr and invalid stdout", string(data))
	}
	var sawStdout, sawStderr bool
	for _, line := range lines {
		var entry struct {
			Time    string `json:"time"`
			Source  string `json:"source"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid JSON log line %q: %v", line, err)
		}
		if entry.Time == "" {
			t.Fatalf("log entry missing time: %+v", entry)
		}
		if entry.Source == "stdout" && strings.Contains(entry.Message, "invalid JSON frame") {
			sawStdout = true
		}
		if entry.Source == "stderr" && strings.Contains(entry.Message, "startup failed loudly") {
			sawStderr = true
		}
	}
	if !sawStdout || !sawStderr {
		t.Fatalf("log missing stdout/stderr diagnostics:\n%s", string(data))
	}
}
