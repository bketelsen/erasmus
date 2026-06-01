package githubcopilot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"erasmus/packages/auth"
	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/provider"
	"erasmus/packages/tool"
)

func TestChatCompletionsStreamUsesCopilotHeaders(t *testing.T) {
	var gotReq struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Stream bool `json:"stream"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer copilot-token" {
			t.Fatalf("authorization = %q", got)
		}
		for name, want := range auth.GitHubCopilotStaticHeaders() {
			if got := r.Header.Get(name); got != want {
				t.Fatalf("%s = %q, want %q", name, got, want)
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"copilot-1\",\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client, err := NewChatCompletions(Config{AccessToken: "copilot-token", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if client.Name() != "github-copilot" {
		t.Fatalf("name = %q", client.Name())
	}
	events, err := client.Stream(context.Background(), provider.Request{
		Model:    model.Model{Provider: "github-copilot", ID: "gpt-4.1"},
		Messages: []message.Message{{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "hello"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for ev := range events {
		switch e := ev.(type) {
		case provider.TextDelta:
			text += e.Text
		case provider.Error:
			t.Fatalf("provider error: %s", e.Err)
		}
	}
	if gotReq.Model != "gpt-4.1" || !gotReq.Stream || len(gotReq.Messages) != 1 || gotReq.Messages[0].Content != "hello" || text != "ok" {
		t.Fatalf("request=%+v text=%q", gotReq, text)
	}
}

func TestDiscoverModelsUsesCopilotHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer copilot-token" {
			t.Fatalf("authorization = %q", got)
		}
		for name, want := range auth.GitHubCopilotStaticHeaders() {
			if got := r.Header.Get(name); got != want {
				t.Fatalf("%s = %q, want %q", name, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4.1"},{"id":"custom-preview"}]}`))
	}))
	defer server.Close()

	client, err := NewChatCompletions(Config{AccessToken: "copilot-token", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	models, err := client.DiscoverModels(context.Background(), "github-copilot")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].Provider != "github-copilot" || models[0].ID != "custom-preview" || models[0].Source != "live" || models[1].ID != "gpt-4.1" {
		t.Fatalf("models = %+v", models)
	}
}

func TestResponsesStreamUsesCopilotHeaders(t *testing.T) {
	var gotReq struct {
		Model        string `json:"model"`
		Instructions string `json:"instructions"`
		Stream       bool   `json:"stream"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer copilot-token" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("chatgpt-account-id"); got != "" {
			t.Fatalf("chatgpt-account-id = %q", got)
		}
		if got := r.Header.Get("openai-beta"); got != "" {
			t.Fatalf("openai-beta = %q", got)
		}
		for name, want := range auth.GitHubCopilotStaticHeaders() {
			if got := r.Header.Get(name); got != want {
				t.Fatalf("%s = %q, want %q", name, got, want)
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.added\",\"output_index\":0}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"delta\":\"hel\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"delta\":\"lo\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":4,\"output_tokens\":2}}}\n\n"))
	}))
	defer server.Close()

	client, err := NewResponses(Config{AccessToken: "copilot-token", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if client.Name() != "github-copilot" {
		t.Fatalf("name = %q", client.Name())
	}
	events, err := client.Stream(context.Background(), provider.Request{
		Model:    model.Model{Provider: "github-copilot", ID: "gpt-5.3-codex"},
		System:   "sys",
		Messages: []message.Message{{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "hello"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var usage bool
	for ev := range events {
		switch e := ev.(type) {
		case provider.TextDelta:
			text += e.Text
		case provider.Usage:
			usage = e.Usage.InputTokens == 4 && e.Usage.OutputTokens == 2
		case provider.Error:
			t.Fatalf("provider error: %s", e.Err)
		}
	}
	if gotReq.Model != "gpt-5.3-codex" || gotReq.Instructions != "sys" || !gotReq.Stream || text != "hello" || !usage {
		t.Fatalf("request=%+v text=%q usage=%v", gotReq, text, usage)
	}
}

func TestAnthropicMessagesStreamUsesCopilotHeaders(t *testing.T) {
	var gotReq struct {
		Model    string `json:"model"`
		System   string `json:"system"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer copilot-token" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("Anthropic-Version"); got == "" {
			t.Fatal("missing Anthropic-Version header")
		}
		for name, want := range auth.GitHubCopilotStaticHeaders() {
			if got := r.Header.Get(name); got != want {
				t.Fatalf("%s = %q, want %q", name, got, want)
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-1\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hel\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"lo\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"input_tokens\":4,\"output_tokens\":2}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	client, err := NewAnthropicMessages(Config{AccessToken: "copilot-token", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if client.Name() != "github-copilot" {
		t.Fatalf("name = %q", client.Name())
	}
	events, err := client.Stream(context.Background(), provider.Request{
		Model:    model.Model{Provider: "github-copilot", ID: "claude-sonnet-4.5"},
		System:   "sys",
		Messages: []message.Message{{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "hello"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var started, ended, usage bool
	for ev := range events {
		switch e := ev.(type) {
		case provider.MessageStart:
			started = e.MessageID == "msg-1"
		case provider.TextDelta:
			text += e.Text
		case provider.MessageEnd:
			ended = e.StopReason == "end_turn"
		case provider.Usage:
			usage = e.Usage.InputTokens == 4 && e.Usage.OutputTokens == 2
		case provider.Error:
			t.Fatalf("provider error: %s", e.Err)
		}
	}
	if gotReq.Model != "claude-sonnet-4.5" || gotReq.System != "sys" || !gotReq.Stream || len(gotReq.Messages) != 1 || gotReq.Messages[0].Role != "user" {
		t.Fatalf("request=%+v", gotReq)
	}
	if !started || text != "hello" || !ended || !usage {
		t.Fatalf("started=%v text=%q ended=%v usage=%v", started, text, ended, usage)
	}
}

func TestAnthropicMessagesToolCallIgnoresEmptyStartInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-1\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu-1\",\"name\":\"read\",\"input\":{}}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"path\\\":\\\"\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"README.md\\\"}\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	client, err := NewAnthropicMessages(Config{AccessToken: "copilot-token", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	events, err := client.Stream(context.Background(), provider.Request{
		Model:    model.Model{Provider: "github-copilot", ID: "claude-sonnet-4.5"},
		Messages: []message.Message{{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "read README"}}}},
		Tools:    []tool.Spec{{Name: "read", Description: "Read a file", Schema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got provider.ToolCall
	for ev := range events {
		switch e := ev.(type) {
		case provider.ToolCall:
			got = e
		case provider.Error:
			t.Fatalf("provider error: %s", e.Err)
		}
	}
	if got.ID != "toolu-1" || got.Name != "read" || string(got.Arguments) != `{"path":"README.md"}` {
		t.Fatalf("tool call = %+v", got)
	}
	if _, err := got.Arguments.MarshalJSON(); err != nil {
		t.Fatalf("invalid arguments JSON: %v", err)
	}
}

func TestNewChatCompletionsRequiresAccessToken(t *testing.T) {
	if _, err := NewChatCompletions(Config{}); err == nil {
		t.Fatal("expected access token error")
	}
}

func TestNewResponsesRequiresAccessToken(t *testing.T) {
	if _, err := NewResponses(Config{}); err == nil {
		t.Fatal("expected access token error")
	}
}

func TestNewAnthropicMessagesRequiresAccessToken(t *testing.T) {
	if _, err := NewAnthropicMessages(Config{}); err == nil {
		t.Fatal("expected access token error")
	}
}
