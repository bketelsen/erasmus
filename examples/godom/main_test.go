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

func TestAppSendChatAppendsConversationRows(t *testing.T) {
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
	app.ChatInput = "hello from chat"
	app.SendChat()

	app.mu.Lock()
	if app.ChatInput != "" {
		t.Fatalf("chat input = %q, want cleared", app.ChatInput)
	}
	if len(app.ChatMessages) < 1 {
		t.Fatalf("chat messages after submit = %d, want at least user row", len(app.ChatMessages))
	}
	if got := app.ChatMessages[0]; got.Role != "user" || got.Text != "hello from chat" {
		t.Fatalf("first chat message = %#v, want user hello row", got)
	}
	app.mu.Unlock()

	waitFor(t, 5*time.Second, func() bool {
		app.mu.Lock()
		defer app.mu.Unlock()
		return app.Status == "ready" && !app.Running && len(app.ChatMessages) >= 2
	}, func() string {
		app.mu.Lock()
		defer app.mu.Unlock()
		return "status=" + app.Status + " running=" + strconv.FormatBool(app.Running) + " messages=" + strconv.Itoa(len(app.ChatMessages)) + " error=" + app.Error
	})

	app.mu.Lock()
	defer app.mu.Unlock()
	last := app.ChatMessages[len(app.ChatMessages)-1]
	if last.Role != "assistant" {
		t.Fatalf("last chat message role = %q, want assistant", last.Role)
	}
	if !strings.Contains(last.Text, "Demo response") || !strings.Contains(last.Text, "hello from chat") {
		t.Fatalf("assistant chat text = %q", last.Text)
	}
}

func TestChatTemplateHasHistoryInputAndSend(t *testing.T) {
	data, err := ui.ReadFile("ui/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{
		`g-for="msg in ChatMessages"`,
		`g-bind="ChatInput"`,
		`g-click="SendChat"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("chat template missing %s", want)
		}
	}
}

func TestParseExampleOptionsKeepsGodomFlags(t *testing.T) {
	opts, remaining, err := parseExampleOptions([]string{
		"--live",
		"--provider", "github-copilot",
		"--model=gpt-4.1",
		"--reasoning", "low",
		"--config", "/tmp/config.json",
		"--auth-file=/tmp/auth.json",
		"--session", "/tmp/session.jsonl",
		"--no-browser",
		"--port=8080",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Live || opts.Provider != "github-copilot" || opts.ModelID != "gpt-4.1" || opts.Reasoning != "low" || opts.Config != "/tmp/config.json" || opts.Auth != "/tmp/auth.json" || opts.Session != "/tmp/session.jsonl" {
		t.Fatalf("parsed options = %#v", opts)
	}
	if got, want := strings.Join(remaining, " "), "--no-browser --port=8080"; got != want {
		t.Fatalf("remaining args = %q, want %q", got, want)
	}
}

func TestLiveModeRejectsFakeProvider(t *testing.T) {
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

	_, err = NewAppWithOptions(context.Background(), exampleOptions{
		Live:    true,
		Config:  filepath.Join(t.TempDir(), "missing-config.json"),
		Auth:    filepath.Join(t.TempDir(), "missing-auth.json"),
		Session: filepath.Join(t.TempDir(), "live.jsonl"),
	})
	if err == nil || !strings.Contains(err.Error(), "live mode requires a real provider") {
		t.Fatalf("live fake provider error = %v", err)
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
