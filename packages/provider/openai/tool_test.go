package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/provider"
	"erasmus/packages/tool"
)

func TestStreamChatCompletionToolCall(t *testing.T) {
	var gotReq chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-tool\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\\\"\"}}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-tool\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"README.md\\\"}\"}}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-tool\",\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client, err := New(Config{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	events, err := client.Stream(context.Background(), provider.Request{
		Model:    model.Model{Provider: "openai", ID: "gpt-test"},
		Messages: []message.Message{{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "read README"}}}},
		Tools:    []tool.Spec{{Name: "read", Description: "Read a file", Schema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var gotTool provider.ToolCall
	var ended bool
	for ev := range events {
		switch e := ev.(type) {
		case provider.ToolCall:
			gotTool = e
		case provider.MessageEnd:
			ended = e.StopReason == "tool_calls"
		case provider.Error:
			t.Fatalf("provider error: %s", e.Err)
		}
	}
	if len(gotReq.Tools) != 1 || gotReq.Tools[0].Type != "function" || gotReq.Tools[0].Function.Name != "read" || gotReq.ToolChoice != "auto" {
		t.Fatalf("tools not encoded: %+v", gotReq)
	}
	if gotTool.ID != "call-1" || gotTool.Name != "read" || string(gotTool.Arguments) != `{"path":"README.md"}` || !ended {
		t.Fatalf("tool=%+v ended=%v", gotTool, ended)
	}
}

func TestRequestBodyEncodesToolHistory(t *testing.T) {
	client := &Client{}
	body, err := client.requestBody(provider.Request{
		Model: model.Model{ID: "gpt-test"},
		Messages: []message.Message{
			{Role: message.RoleAssistant, Content: []message.Content{message.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}}},
			{Role: message.RoleTool, Content: []message.Content{message.ToolResult{CallID: "call-1", Content: []message.Content{message.Text{Text: "contents"}}}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got chatRequest
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 2 || got.Messages[0].Role != "assistant" || got.Messages[0].ToolCalls[0].ID != "call-1" || got.Messages[1].Role != "tool" || got.Messages[1].ToolCallID != "call-1" || got.Messages[1].Content != "contents" {
		t.Fatalf("unexpected body: %s", string(body))
	}
}
