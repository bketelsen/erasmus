package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/provider"
)

func TestStreamChatCompletionSSE(t *testing.T) {
	var gotAuth string
	var gotReq chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"delta\":{\"content\":\"lo\"},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client, err := New(Config{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	events, err := client.Stream(context.Background(), provider.Request{
		Model:    model.Model{Provider: "openai", ID: "gpt-test"},
		System:   "be helpful",
		Messages: []message.Message{{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "hello?"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var started, ended, usage bool
	for ev := range events {
		switch e := ev.(type) {
		case provider.MessageStart:
			started = e.MessageID == "chatcmpl-1"
		case provider.TextDelta:
			text += e.Text
		case provider.MessageEnd:
			ended = e.StopReason == "stop"
		case provider.Usage:
			usage = e.Usage.InputTokens == 3 && e.Usage.OutputTokens == 2
		case provider.Error:
			t.Fatalf("provider error: %s", e.Err)
		}
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if gotReq.Model != "gpt-test" || !gotReq.Stream || len(gotReq.Messages) != 2 || gotReq.Messages[0].Role != "system" || gotReq.Messages[1].Content != "hello?" {
		t.Fatalf("unexpected request: %+v", gotReq)
	}
	if !started || text != "hello" || !ended || !usage {
		t.Fatalf("started=%v text=%q ended=%v usage=%v", started, text, ended, usage)
	}
}

func TestNewRequiresAPIKey(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected api key error")
	}
}

func TestStreamHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	defer server.Close()
	client, err := New(Config{APIKey: "bad", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Stream(context.Background(), provider.Request{Model: model.Model{ID: "gpt-test"}})
	if err == nil {
		t.Fatal("expected http error")
	}
}

func TestDiscoverModels(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-test"},{"id":"gpt-next"}]}`))
	}))
	defer server.Close()

	client, err := New(Config{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	models, err := client.DiscoverModels(context.Background(), "openai")
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if len(models) != 2 || models[0].Provider != "openai" || models[0].ID != "gpt-next" || models[0].Source != "live" || models[1].ID != "gpt-test" {
		t.Fatalf("models = %+v", models)
	}
}

func TestDiscoverModelsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	defer server.Close()
	client, err := New(Config{APIKey: "bad", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.DiscoverModels(context.Background(), "openai"); err == nil {
		t.Fatal("expected http error")
	}
}
