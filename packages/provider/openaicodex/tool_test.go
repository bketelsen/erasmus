package openaicodex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/provider"
	"github.com/bketelsen/erasmus/packages/tool"
)

func TestStreamCodexToolCall(t *testing.T) {
	var got codexRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"id\":\"fc-1\",\"type\":\"function_call\",\"call_id\":\"call-1\",\"name\":\"read\",\"arguments\":\"\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.function_call_arguments.delta\",\"output_index\":0,\"item_id\":\"fc-1\",\"delta\":\"{\\\"path\\\":\\\"\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.function_call_arguments.delta\",\"output_index\":0,\"item_id\":\"fc-1\",\"delta\":\"README.md\\\"}\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"id\":\"fc-1\",\"type\":\"function_call\",\"call_id\":\"call-1\",\"name\":\"read\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":4,\"output_tokens\":2}}}\n\n"))
	}))
	defer server.Close()
	client, err := New(Config{AccessToken: "tok", AccountID: "acct", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	events, err := client.Stream(context.Background(), provider.Request{
		Model:    model.Model{Provider: "openai-codex", ID: "gpt-5.3-codex"},
		Messages: []message.Message{{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "read README"}}}},
		Tools:    []tool.Spec{{Name: "read", Description: "Read a file", Schema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var gotTool provider.ToolCall
	var ended, usage bool
	for ev := range events {
		switch e := ev.(type) {
		case provider.ToolCall:
			gotTool = e
		case provider.MessageEnd:
			ended = e.StopReason == "tool_calls"
		case provider.Usage:
			usage = e.Usage.InputTokens == 4 && e.Usage.OutputTokens == 2
		case provider.Error:
			t.Fatalf("provider error: %s", e.Err)
		}
	}
	if len(got.Tools) != 1 || got.Tools[0].Type != "function" || got.Tools[0].Name != "read" {
		t.Fatalf("tools not encoded: %+v", got.Tools)
	}
	if gotTool.ID != "call-1" || gotTool.Name != "read" || string(gotTool.Arguments) != `{"path":"README.md"}` || !ended || !usage {
		t.Fatalf("tool=%+v ended=%v usage=%v", gotTool, ended, usage)
	}
}

func TestBuildRequestEncodesCodexToolHistory(t *testing.T) {
	req := buildRequest(provider.Request{Model: model.Model{ID: "m"}, Messages: []message.Message{
		{Role: message.RoleAssistant, Content: []message.Content{message.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}}},
		{Role: message.RoleTool, Content: []message.Content{message.ToolResult{CallID: "call-1", Content: []message.Content{message.Text{Text: "contents"}}}}},
	}})
	if len(req.Input) != 2 {
		t.Fatalf("input len = %d", len(req.Input))
	}
	call, ok := req.Input[0].(functionCallInput)
	if !ok || call.Type != "function_call" || call.CallID != "call-1" || call.Name != "read" || call.Arguments != `{"path":"README.md"}` {
		t.Fatalf("call = %#v", req.Input[0])
	}
	out, ok := req.Input[1].(functionCallOutput)
	if !ok || out.Type != "function_call_output" || out.CallID != "call-1" || out.Output != "contents" {
		t.Fatalf("output = %#v", req.Input[1])
	}
}
