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

func TestRunTUIConfiguredPersistsJSONLSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	var out bytes.Buffer
	err := RunTUIConfigured(context.Background(), TUIOptions{In: strings.NewReader("hello\n/quit\n"), Out: &out, SessionPath: path}, config.Config{Provider: "fake", Model: "echo"}, auth.NewMemoryStore())
	if err != nil {
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

func TestDefaultTUISessionPath(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	got := DefaultTUISessionPath("/tmp/project")
	if !strings.HasPrefix(got, filepath.Join(stateHome, "erasmus", "sessions")+string(os.PathSeparator)) {
		t.Fatalf("path %q does not use XDG state home %q", got, stateHome)
	}
	if !strings.HasSuffix(got, filepath.Join("default.jsonl")) {
		t.Fatalf("path %q does not end with default.jsonl", got)
	}
	if strings.Contains(got, ".erasmus") {
		t.Fatalf("path %q still uses project-local .erasmus storage", got)
	}
}

func TestRunTUIFake(t *testing.T) {
	var out bytes.Buffer
	if err := RunTUIFake(context.Background(), TUIOptions{In: strings.NewReader("hello\n/help\n/status\n/model\n/tree\n/move 1\n/branch 1\n/quit\n"), Out: &out}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "Erasmus TUI MVP") || !strings.Contains(got, "assistant: fake response: hello") || !strings.Contains(got, "commands:") || !strings.Contains(got, "/sessions [dir]") || !strings.Contains(got, "model=fake/echo") || !strings.Contains(got, "model: fake/echo") || !strings.Contains(got, "leaf=") || !strings.Contains(got, "branch session=") || !strings.Contains(got, "bye") {
		t.Fatalf("unexpected output:\n%s", got)
	}
}

func TestRunTUIConfiguredSessionsAndOpen(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.jsonl")
	second := filepath.Join(root, "second.jsonl")
	store := auth.NewMemoryStore()
	cfg := config.Config{Provider: "fake", Model: "echo", CWD: root}
	if err := RunTUIConfigured(context.Background(), TUIOptions{In: strings.NewReader("first message\n/quit\n"), SessionPath: first}, cfg, store); err != nil {
		t.Fatal(err)
	}
	if err := RunTUIConfigured(context.Background(), TUIOptions{In: strings.NewReader("second message\n/quit\n"), SessionPath: second}, cfg, store); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	input := "/sessions " + root + "\n/open " + first + "\n/messages 2\n/tree\n/quit\n"
	if err := RunTUIConfigured(context.Background(), TUIOptions{In: strings.NewReader(input), Out: &out, SessionPath: second}, cfg, store); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"first.jsonl", "second.jsonl", "opened session=", "first message", "leaf=", "* id="} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}
