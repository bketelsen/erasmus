package message_test

import (
	"encoding/json"
	"testing"

	"github.com/bketelsen/erasmus/packages/message"
)

func TestMessageUnmarshalJSONDecodesTextContent(t *testing.T) {
	var msg message.Message
	if err := json.Unmarshal([]byte(`{"role":"user","content":[{"text":"hello"}]}`), &msg); err != nil {
		t.Fatal(err)
	}
	text, ok := msg.Content[0].(message.Text)
	if !ok {
		t.Fatalf("content type = %T", msg.Content[0])
	}
	if text.Text != "hello" {
		t.Fatalf("text = %q", text.Text)
	}
}

func TestMessageUnmarshalJSONDecodesTypedToolResultContent(t *testing.T) {
	var msg message.Message
	data := []byte(`{"role":"tool","content":[{"type":"tool_result","call_id":"call-1","content":[{"type":"text","text":"ok"}]}]}`)
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	result, ok := msg.Content[0].(message.ToolResult)
	if !ok {
		t.Fatalf("content type = %T", msg.Content[0])
	}
	if result.CallID != "call-1" {
		t.Fatalf("call id = %q", result.CallID)
	}
	text, ok := result.Content[0].(message.Text)
	if !ok || text.Text != "ok" {
		t.Fatalf("nested content = %#v", result.Content)
	}
}
