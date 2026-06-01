package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"erasmus/packages/message"
	"erasmus/packages/sandbox"
	"erasmus/packages/tool"
	"erasmus/packages/tools"
)

func TestReadToolReadsFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	policy, err := sandbox.New(root)
	if err != nil {
		t.Fatal(err)
	}
	read := tools.NewReadTool(policy)

	args, _ := json.Marshal(map[string]string{"path": "hello.txt"})
	var progress []string
	res, err := read.Execute(context.Background(), args, func(p tool.Progress) {
		progress = append(progress, p.Text)
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatal("result unexpectedly marked error")
	}
	if len(res.Content) != 1 {
		t.Fatalf("content len = %d, want 1", len(res.Content))
	}
	text, ok := res.Content[0].(message.Text)
	if !ok {
		t.Fatalf("content type = %T, want message.Text", res.Content[0])
	}
	if text.Text != "hello world" {
		t.Fatalf("text = %q", text.Text)
	}
	if len(progress) != 1 || progress[0] != "reading hello.txt" {
		t.Fatalf("progress = %v", progress)
	}
}

func TestReadToolRejectsEscape(t *testing.T) {
	root := t.TempDir()
	policy, err := sandbox.New(root)
	if err != nil {
		t.Fatal(err)
	}
	read := tools.NewReadTool(policy)

	args, _ := json.Marshal(map[string]string{"path": "../nope.txt"})
	res, err := read.Execute(context.Background(), args, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !res.IsError {
		t.Fatal("expected error result")
	}
}
