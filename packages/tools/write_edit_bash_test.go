package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/sandbox"
	"github.com/bketelsen/erasmus/packages/tools"
)

func TestWriteToolWritesFile(t *testing.T) {
	root := t.TempDir()
	policy, err := sandbox.New(root)
	if err != nil {
		t.Fatal(err)
	}
	write := tools.NewWriteTool(policy)
	args, _ := json.Marshal(map[string]string{"path": "dir/file.txt", "content": "hello"})
	res, err := write.Execute(context.Background(), args, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatal("unexpected error result")
	}
	data, err := os.ReadFile(filepath.Join(root, "dir/file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("file = %q", data)
	}
}

func TestWriteToolRejectsEscape(t *testing.T) {
	root := t.TempDir()
	policy, err := sandbox.New(root)
	if err != nil {
		t.Fatal(err)
	}
	write := tools.NewWriteTool(policy)
	args, _ := json.Marshal(map[string]string{"path": "../file.txt", "content": "hello"})
	res, err := write.Execute(context.Background(), args, nil)
	if err == nil || !res.IsError {
		t.Fatalf("err = %v, result = %+v; want error", err, res)
	}
}

func TestEditToolEditsUniqueMatch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	policy, err := sandbox.New(root)
	if err != nil {
		t.Fatal(err)
	}
	edit := tools.NewEditTool(policy)
	args, _ := json.Marshal(map[string]string{"path": "file.txt", "old_text": "world", "new_text": "erasmus"})
	res, err := edit.Execute(context.Background(), args, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatal("unexpected error result")
	}
	data, err := os.ReadFile(filepath.Join(root, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello erasmus" {
		t.Fatalf("file = %q", data)
	}
}

func TestEditToolRejectsAmbiguousMatch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("x x"), 0o644); err != nil {
		t.Fatal(err)
	}
	policy, err := sandbox.New(root)
	if err != nil {
		t.Fatal(err)
	}
	edit := tools.NewEditTool(policy)
	args, _ := json.Marshal(map[string]string{"path": "file.txt", "old_text": "x", "new_text": "y"})
	res, err := edit.Execute(context.Background(), args, nil)
	if err == nil || !res.IsError {
		t.Fatalf("err = %v, result = %+v; want error", err, res)
	}
}

func TestBashToolRunsInSandboxRoot(t *testing.T) {
	root := t.TempDir()
	policy, err := sandbox.New(root)
	if err != nil {
		t.Fatal(err)
	}
	bash := tools.NewBashTool(policy)
	args, _ := json.Marshal(map[string]any{"command": "pwd && printf hi"})
	res, err := bash.Execute(context.Background(), args, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatal("unexpected error result")
	}
	text := res.Content[0].(message.Text).Text
	if want := policy.Root + "\nhi"; text != want {
		t.Fatalf("output = %q, want %q", text, want)
	}
}

func TestBashToolTimeout(t *testing.T) {
	root := t.TempDir()
	policy, err := sandbox.New(root)
	if err != nil {
		t.Fatal(err)
	}
	bash := tools.NewBashTool(policy)
	bash.DefaultTimeout = time.Second
	args, _ := json.Marshal(map[string]any{"command": "sleep 1", "timeout_ms": 1})
	res, err := bash.Execute(context.Background(), args, nil)
	if err == nil || !res.IsError {
		t.Fatalf("err = %v, result = %+v; want timeout error", err, res)
	}
}
