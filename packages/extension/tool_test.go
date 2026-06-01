package extension_test

import (
	"context"
	"encoding/json"
	"testing"

	"erasmus/packages/extension"
	"erasmus/packages/extension/proto"
	"erasmus/packages/message"
	"erasmus/packages/tool"
)

func TestExtensionToolExecute(t *testing.T) {
	caller := &fakeCaller{}
	extTool := extension.NewTool(proto.RegisterTool{Name: "weather", Description: "Weather"}, caller)
	args := json.RawMessage(`{"city":"Paris"}`)
	var progress []string
	res, err := extTool.Execute(context.Background(), args, func(p tool.Progress) { progress = append(progress, p.Text) })
	if err != nil {
		t.Fatal(err)
	}
	if caller.call.Name != "weather" || string(caller.call.Args) != string(args) {
		t.Fatalf("call = %+v", caller.call)
	}
	if len(progress) != 1 {
		t.Fatalf("progress = %v", progress)
	}
	if got := res.Content[0].(message.Text).Text; got != "ok" {
		t.Fatalf("result = %q", got)
	}
}

func TestManagerRegistry(t *testing.T) {
	m := extension.NewManager(&fakeCaller{})
	m.RegisterTool(proto.RegisterTool{Name: "weather"})
	registry := m.Registry()
	if _, ok := registry.Get("weather"); !ok {
		t.Fatal("weather tool missing")
	}
}

type fakeCaller struct{ call proto.ToolCall }

func (f *fakeCaller) CallTool(ctx context.Context, call proto.ToolCall) (proto.ToolResult, error) {
	f.call = call
	return proto.ToolResult{ID: call.ID, Result: tool.Result{Content: []message.Content{message.Text{Text: "ok"}}}}, nil
}
