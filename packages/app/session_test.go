package app_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bketelsen/erasmus/packages/app"
	"github.com/bketelsen/erasmus/packages/auth"
	"github.com/bketelsen/erasmus/packages/config"
)

func TestSessionUXCommands(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.jsonl")
	if err := app.RunConfigured(ctx, app.RunOptions{Prompt: "hello session", SessionPath: path}, config.Config{Provider: "fake", Model: "echo"}, auth.NewMemoryStore()); err != nil {
		t.Fatal(err)
	}

	entries, err := app.ListSessions(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != path || entries[0].Messages != 2 {
		t.Fatalf("entries = %+v", entries)
	}

	var out bytes.Buffer
	if err := app.PrintSessions(ctx, &out, dir); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "demo.jsonl") || !strings.Contains(out.String(), "messages=2") {
		t.Fatalf("list output:\n%s", out.String())
	}

	out.Reset()
	if err := app.PrintSessionShow(ctx, &out, path); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "user: hello session") || !strings.Contains(out.String(), "assistant: fake response: hello session") {
		t.Fatalf("show output:\n%s", out.String())
	}

	out.Reset()
	if err := app.PrintSessionTree(ctx, &out, path); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "leaf=") || !strings.Contains(out.String(), "entries=2") || !strings.Contains(out.String(), "type=message") {
		t.Fatalf("tree output:\n%s", out.String())
	}
}

func TestListSessionsDefaultUsesXDGState(t *testing.T) {
	ctx := context.Background()
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	path := app.DefaultTUISessionPath("")
	if !strings.HasPrefix(path, filepath.Join(stateHome, "erasmus", "sessions")+string(filepath.Separator)) {
		t.Fatalf("default session path = %q, want XDG state under %q", path, stateHome)
	}
	if err := app.RunConfigured(ctx, app.RunOptions{Prompt: "hello default sessions", SessionPath: path}, config.Config{Provider: "fake", Model: "echo"}, auth.NewMemoryStore()); err != nil {
		t.Fatal(err)
	}
	entries, err := app.ListSessions(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != path {
		t.Fatalf("entries = %+v", entries)
	}
}
