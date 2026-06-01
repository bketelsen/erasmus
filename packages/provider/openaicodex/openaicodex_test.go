package openaicodex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/provider"
)

func TestStreamCodexSSE(t *testing.T) {
	var auth, account, beta, originator string
	var got codexRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("authorization")
		account = r.Header.Get("chatgpt-account-id")
		beta = r.Header.Get("openai-beta")
		originator = r.Header.Get("originator")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.added\",\"output_index\":0}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"delta\":\"hel\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"delta\":\"lo\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":4,\"output_tokens\":2}}}\n\n"))
	}))
	defer server.Close()
	client, err := New(Config{AccessToken: "tok", AccountID: "acct", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	events, err := client.Stream(context.Background(), provider.Request{Model: model.Model{Provider: "openai-codex", ID: "gpt-5.3-codex"}, System: "sys", Messages: []message.Message{{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "hi"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var started, ended, usage bool
	for ev := range events {
		switch e := ev.(type) {
		case provider.MessageStart:
			started = true
		case provider.TextDelta:
			text += e.Text
		case provider.MessageEnd:
			ended = e.StopReason == "stop"
		case provider.Usage:
			usage = e.Usage.InputTokens == 4 && e.Usage.OutputTokens == 2
		case provider.Error:
			t.Fatalf("provider error: %s", e.Err)
		}
	}
	if auth != "Bearer tok" || account != "acct" || beta != "responses=experimental" || originator != "erasmus" {
		t.Fatalf("bad headers auth=%q account=%q beta=%q originator=%q", auth, account, beta, originator)
	}
	if got.Model != "gpt-5.3-codex" || !got.Stream || got.Instructions != "sys" || len(got.Input) != 1 {
		t.Fatalf("unexpected body: %+v", got)
	}
	if !started || text != "hello" || !ended || !usage {
		t.Fatalf("started=%v text=%q ended=%v usage=%v", started, text, ended, usage)
	}
}

func TestBuildRequestUsesOutputTextForAssistantHistory(t *testing.T) {
	req := buildRequest(provider.Request{Model: model.Model{ID: "m"}, Messages: []message.Message{
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "remember papaya"}}},
		{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "Papaya noted."}}},
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "what word?"}}},
	}})
	if len(req.Input) != 3 {
		t.Fatalf("input len = %d", len(req.Input))
	}
	assistant, ok := req.Input[1].(inputMessage)
	if !ok {
		t.Fatalf("assistant input type %T", req.Input[1])
	}
	if assistant.Role != "assistant" || len(assistant.Content) != 1 || assistant.Content[0].Type != "output_text" {
		t.Fatalf("assistant input = %+v", assistant)
	}
}

func TestNewRequiresTokenAndAccount(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected token error")
	}
	if _, err := New(Config{AccessToken: "tok"}); err == nil {
		t.Fatal("expected account error")
	}
}
