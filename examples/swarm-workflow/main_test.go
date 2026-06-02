package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWorkflowUsesNamedAgents(t *testing.T) {
	root := t.TempDir()
	result, err := runWorkflow(context.Background(), options{
		Task:     "Add transcript export support",
		StateDir: filepath.Join(root, "state"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Agents) != 3 {
		t.Fatalf("agents = %d, want 3", len(result.Agents))
	}
	for _, want := range []string{"planner", "reviewer", "executor"} {
		if _, ok := result.Agents[want]; !ok {
			t.Fatalf("missing named agent %q in %#v", want, result.Agents)
		}
		logPath := filepath.Join(root, "state", "events", want+".events.jsonl")
		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read event log for %s: %v", want, err)
		}
		if !strings.Contains(string(data), `"type":"message_delta"`) {
			t.Fatalf("%s event log missing message delta:\n%s", want, data)
		}
	}

	for _, want := range []string{
		"## Plan",
		"planner: Plan",
		"## Review",
		"reviewer: Review",
		"## Execution",
		"executor: Execution",
		"Add transcript export support",
	} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, result.Summary)
		}
	}
}
