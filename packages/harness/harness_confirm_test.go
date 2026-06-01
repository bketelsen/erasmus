package harness_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bketelsen/erasmus/packages/harness"
	"github.com/bketelsen/erasmus/packages/loop"
	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/provider"
	"github.com/bketelsen/erasmus/packages/session/memory"
	"github.com/bketelsen/erasmus/packages/tool"
)

func TestHarnessConfirmToolCallCanDeny(t *testing.T) {
	ctx := context.Background()
	args, _ := json.Marshal(map[string]string{"ok": "true"})
	calls := 0
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		calls++
		if calls == 1 {
			return streamEvents(provider.MessageStart{MessageID: "a1"}, provider.ToolCall{ID: "c1", Name: "stub", Arguments: args}, provider.MessageEnd{StopReason: "tool_use"}), nil
		}
		return streamEvents(provider.MessageStart{MessageID: "a2"}, provider.TextDelta{Text: "done"}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}
	confirmed := 0
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("test"),
		Stream:  stream,
		Model:   model.Model{Provider: "fake", ID: "test"},
		Tools:   stubRegistry{stubTool{}},
		ConfirmToolCall: func(ctx context.Context, tc loop.ToolCallContext) (bool, error) {
			confirmed++
			return false, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := h.Prompt(ctx, "hi", harness.PromptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	drain(events)
	if confirmed != 1 {
		t.Fatalf("confirmed = %d, want 1", confirmed)
	}
	built, err := h.Session().BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result := built.Messages[2].Content[0].(message.ToolResult)
	if !result.IsError {
		t.Fatal("denied result should be error")
	}
}

type stubTool struct{}

func (stubTool) Name() string            { return "stub" }
func (stubTool) Description() string     { return "stub" }
func (stubTool) Schema() json.RawMessage { return nil }
func (stubTool) Execute(context.Context, json.RawMessage, func(tool.Progress)) (tool.Result, error) {
	return tool.Result{Content: []message.Content{message.Text{Text: "ran"}}}, nil
}

type stubRegistry struct{ t stubTool }

func (r stubRegistry) Get(name string) (tool.Tool, bool) { return r.t, name == r.t.Name() }
func (r stubRegistry) List() []tool.Tool                 { return []tool.Tool{r.t} }
func (r stubRegistry) Specs() []tool.Spec                { return []tool.Spec{{Name: r.t.Name()}} }
