package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAppSubmitPromptRunsToolBackedConversation(t *testing.T) {
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalWD); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	app, err := NewApp(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	app.PromptText = "write a demo note"
	app.SubmitPrompt()

	waitFor(t, 5*time.Second, func() bool {
		app.mu.Lock()
		defer app.mu.Unlock()
		return app.Status == "ready" && !app.Running
	}, func() string {
		app.mu.Lock()
		defer app.mu.Unlock()
		return "status=" + app.Status + " running=" + strconv.FormatBool(app.Running) + " error=" + app.Error
	})

	note, err := os.ReadFile(filepath.Join(".erasmus", "examples", "godom", "notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(note), "hello from the Erasmus godom example\n"; got != want {
		t.Fatalf("note = %q, want %q", got, want)
	}
	app.mu.Lock()
	transcript := app.Transcript
	tools := len(app.Tools)
	skills := len(app.Skills)
	events := len(app.Events)
	app.mu.Unlock()
	if !strings.Contains(transcript, "Tool result observed") {
		t.Fatalf("transcript does not include final tool result response:\n%s", transcript)
	}
	if tools == 0 || skills == 0 || events == 0 {
		t.Fatalf("app rows not populated: tools=%d skills=%d events=%d", tools, skills, events)
	}
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool, describe func() string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition was not satisfied before timeout: %s", describe())
}
