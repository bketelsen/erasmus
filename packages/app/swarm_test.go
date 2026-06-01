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

func TestRunSwarmConfiguredPersistsSessionPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "swarm.jsonl")
	var out bytes.Buffer
	if err := RunSwarmConfigured(context.Background(), SwarmRunOptions{Task: "hello", Out: &out, SessionPath: path}, config.Config{Provider: "fake", Model: "echo"}, auth.NewMemoryStore()); err != nil {
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

func TestRunSwarmConfiguredPersistsSessionDir(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if err := RunSwarmConfigured(context.Background(), SwarmRunOptions{Task: "hello", Out: &out, SessionDir: dir}, config.Config{Provider: "fake", Model: "echo"}, auth.NewMemoryStore()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "main.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hello") || !strings.Contains(string(data), "fake response: hello") {
		t.Fatalf("session did not persist transcript:\n%s", string(data))
	}
}

func TestRunSwarmFake(t *testing.T) {
	var out bytes.Buffer
	logDir := t.TempDir()
	if err := RunSwarmFake(context.Background(), SwarmRunOptions{Task: "hello", Out: &out, EventLogDir: logDir}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "swarm agent main settled") || !strings.Contains(got, "event log:") || !strings.Contains(got, "fake response: hello") {
		t.Fatalf("unexpected output:\n%s", got)
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected event log file")
	}
}
