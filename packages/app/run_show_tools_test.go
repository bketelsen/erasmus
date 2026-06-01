package app_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bketelsen/erasmus/packages/app"
)

func TestRunFakeShowTools(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := app.RunFake(context.Background(), app.RunOptions{Prompt: "read note.txt", CWD: root, Out: &out, ShowTools: true}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"[tool start] read", `"path":"note.txt"`, "[tool end] read done", "fake response:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRunFakeHidesToolsByDefault(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := app.RunFake(context.Background(), app.RunOptions{Prompt: "read note.txt", CWD: root, Out: &out}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "[tool start]") || strings.Contains(out.String(), "[tool end]") {
		t.Fatalf("tool markers should be hidden by default:\n%s", out.String())
	}
}
