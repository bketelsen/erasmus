package app_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"erasmus/packages/app"
	"erasmus/packages/auth"
	"erasmus/packages/config"
)

func TestRunConfiguredPersistsJSONLSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.jsonl")
	var out bytes.Buffer
	if err := app.RunConfigured(context.Background(), app.RunOptions{Prompt: "hello", Out: &out, SessionPath: path}, config.Config{Provider: "fake", Model: "echo"}, auth.NewMemoryStore()); err != nil {
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

func TestRunConfiguredResumesJSONLSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.jsonl")
	ctx := context.Background()
	store := auth.NewMemoryStore()
	cfg := config.Config{Provider: "fake", Model: "echo"}
	if err := app.RunConfigured(ctx, app.RunOptions{Prompt: "first", SessionPath: path}, cfg, store); err != nil {
		t.Fatal(err)
	}
	if err := app.RunConfigured(ctx, app.RunOptions{Prompt: "second", SessionPath: path}, cfg, store); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"first", "second", "fake response: first", "fake response: second"} {
		if !strings.Contains(text, want) {
			t.Fatalf("session missing %q:\n%s", want, text)
		}
	}
}

func TestRunFakeCanWriteReadAndEditThroughHarness(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	if err := app.RunFake(context.Background(), app.RunOptions{Prompt: "write note.txt content hello", CWD: root, Out: &out}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("written file = %q", data)
	}

	out.Reset()
	if err := app.RunFake(context.Background(), app.RunOptions{Prompt: "read note.txt", CWD: root, Out: &out}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "fake response:") {
		t.Fatalf("read output = %q", out.String())
	}

	out.Reset()
	if err := app.RunFake(context.Background(), app.RunOptions{Prompt: "edit note.txt hello bye", CWD: root, Out: &out}); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(root, "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "bye" {
		t.Fatalf("edited file = %q", data)
	}
}
