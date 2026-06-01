package extension_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bketelsen/erasmus/packages/extension"
	"github.com/bketelsen/erasmus/packages/extension/proto"
	"github.com/bketelsen/erasmus/packages/loop"
	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/tool"
)

func TestManagerHooksInterceptors(t *testing.T) {
	m := extension.NewManager(nil)
	patched := tool.Result{Content: []message.Content{message.Text{Text: "patched"}}}
	m.AddInterceptor(interceptor{result: &patched})
	hooks := m.Hooks()
	decision, err := hooks.BeforeToolCall(context.Background(), loop.ToolCallContext{Call: message.ToolCall{Name: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Deny {
		t.Fatal("unexpected deny")
	}
	patch, err := hooks.AfterToolCall(context.Background(), loop.ToolResultContext{Result: tool.Result{}})
	if err != nil {
		t.Fatal(err)
	}
	if patch.Result == nil || patch.Result.Content[0].(message.Text).Text != "patched" {
		t.Fatalf("patch = %+v", patch)
	}
}

func TestManagerCommandsAndHostActions(t *testing.T) {
	caller := &fakeCommandCaller{}
	m := extension.NewManager(nil)
	m.RegisterCommand(extensionProtoCommand("hello"), caller)
	cmd, ok := m.Command("hello")
	if !ok {
		t.Fatal("command missing")
	}
	res, err := cmd.Execute(context.Background(), json.RawMessage(`{"name":"Ada"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Actions) != 1 || res.Actions[0].Type != "notify" {
		t.Fatalf("result = %+v", res)
	}
	m.AddHostAction(res.Actions[0])
	if len(m.DrainHostActions()) != 1 || len(m.DrainHostActions()) != 0 {
		t.Fatal("host action drain failed")
	}
}

func extensionProtoCommand(name string) proto.RegisterCommand {
	return proto.RegisterCommand{Name: name, Description: "test command"}
}

type fakeCommandCaller struct{}

func (f *fakeCommandCaller) CallCommand(ctx context.Context, call proto.CommandCall) (proto.CommandResult, error) {
	data, _ := json.Marshal(map[string]string{"message": "hello"})
	return proto.CommandResult{ID: call.ID, Actions: []proto.HostAction{{Type: "notify", Data: data}}}, nil
}

type interceptor struct{ result *tool.Result }

func (i interceptor) BeforeToolCall(context.Context, loop.ToolCallContext) (loop.ToolDecision, error) {
	return loop.ToolDecision{}, nil
}
func (i interceptor) AfterToolCall(context.Context, loop.ToolResultContext) (loop.ToolResultPatch, error) {
	return loop.ToolResultPatch{Result: i.result}, nil
}
