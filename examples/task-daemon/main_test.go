package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessPendingRunsHarnessAndWritesOutputs(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "inbox")
	out := filepath.Join(root, "out")
	state := filepath.Join(root, "state")
	repo := filepath.Join(root, "repo")
	for _, dir := range []string{inbox, out, state, repo} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(inbox, "001-demo.md"), []byte("# Demo task\n\nSay hello from the daemon."), 0o644); err != nil {
		t.Fatal(err)
	}

	processed, err := processPending(context.Background(), options{Inbox: inbox, Out: out, State: state, Repo: repo})
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}

	runDir := filepath.Join(out, "001-demo")
	summary, err := os.ReadFile(filepath.Join(runDir, "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(summary), "Processed task") || !strings.Contains(string(summary), "Demo task") {
		t.Fatalf("summary = %q", string(summary))
	}
	events, err := os.ReadFile(filepath.Join(runDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(events), `"type":"message_delta"`) || !strings.Contains(string(events), `"type":"settled"`) {
		t.Fatalf("events missing expected records:\n%s", string(events))
	}
	var status taskStatus
	data, err := os.ReadFile(filepath.Join(runDir, "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatal(err)
	}
	if status.Status != "done" || status.SessionPath == "" {
		t.Fatalf("status = %#v", status)
	}
	if _, err := os.Stat(filepath.Join(runDir, "session.jsonl")); err != nil {
		t.Fatal(err)
	}
}

func TestAlreadyDoneSkipsCompletedTask(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "inbox")
	out := filepath.Join(root, "out")
	repo := filepath.Join(root, "repo")
	for _, dir := range []string{inbox, out, repo, filepath.Join(out, "skip-me")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(inbox, "skip-me.md"), []byte("# Skip me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(out, "skip-me", "status.json"), taskStatus{ID: "skip-me", Status: "done"}); err != nil {
		t.Fatal(err)
	}
	processed, err := processPending(context.Background(), options{Inbox: inbox, Out: out, Repo: repo})
	if err != nil {
		t.Fatal(err)
	}
	if processed != 0 {
		t.Fatalf("processed = %d, want 0", processed)
	}
}

func TestTaskIDSanitizesFilename(t *testing.T) {
	if got, want := taskID("/tmp/Hello, daemon!.md"), "Hello--daemon"; got != want {
		t.Fatalf("taskID = %q, want %q", got, want)
	}
}
