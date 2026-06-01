package extension_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bketelsen/erasmus/packages/extension"
	"github.com/bketelsen/erasmus/packages/extension/proto"
	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/tool"
)

func TestHeadlessWeatherExtensionPath(t *testing.T) {
	caller := &weatherCaller{}
	m := extension.NewManager(caller)
	m.RegisterTool(proto.RegisterTool{Name: "weather", Description: "get weather"})
	registry := m.Registry()
	weather, ok := registry.Get("weather")
	if !ok {
		t.Fatal("weather tool missing")
	}
	args, _ := json.Marshal(map[string]string{"city": "Paris"})
	res, err := weather.Execute(context.Background(), args, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Content[0].(message.Text).Text; got != "sunny in Paris" {
		t.Fatalf("weather = %q", got)
	}
}

type weatherCaller struct{}

func (w *weatherCaller) CallTool(ctx context.Context, call proto.ToolCall) (proto.ToolResult, error) {
	var in struct {
		City string `json:"city"`
	}
	_ = json.Unmarshal(call.Args, &in)
	return proto.ToolResult{ID: call.ID, Result: tool.Result{Content: []message.Content{message.Text{Text: "sunny in " + in.City}}}}, nil
}
