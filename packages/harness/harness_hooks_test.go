package harness_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"erasmus/packages/harness"
	"erasmus/packages/loop"
	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/provider"
	"erasmus/packages/session/memory"
	"erasmus/packages/tool"
)

func TestHarnessToolCallHookPatchesArguments(t *testing.T) {
	ctx := context.Background()
	originalArgs := json.RawMessage(`{"ok":false}`)
	patchedArgs := json.RawMessage(`{"ok":true}`)
	calls := 0
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		calls++
		if calls == 1 {
			return streamEvents(provider.MessageStart{MessageID: "a1"}, provider.ToolCall{ID: "c1", Name: "stub", Arguments: originalArgs}, provider.MessageEnd{StopReason: "tool_use"}), nil
		}
		return streamEvents(provider.MessageStart{MessageID: "a2"}, provider.TextDelta{Text: "done"}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}
	recorder := &recordingTool{name: "stub"}
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("test"),
		Stream:  stream,
		Model:   model.Model{Provider: "fake", ID: "test"},
		Tools:   tool.NewRegistry(recorder),
		Hooks: harness.Hooks{
			ToolCall: func(ctx context.Context, tc harness.ToolCallContext) (harness.ToolCallDecision, error) {
				if string(tc.Call.Arguments) != string(originalArgs) {
					t.Fatalf("hook args = %s, want %s", tc.Call.Arguments, originalArgs)
				}
				return harness.ToolCallDecision{Arguments: patchedArgs}, nil
			},
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
	if string(recorder.args) != string(patchedArgs) {
		t.Fatalf("tool args = %s, want %s", recorder.args, patchedArgs)
	}
}

func TestHarnessToolCallHookRunsAfterLoopHookAndCanDeny(t *testing.T) {
	ctx := context.Background()
	originalArgs := json.RawMessage(`{"stage":"original"}`)
	loopArgs := json.RawMessage(`{"stage":"loop"}`)
	order := []string{}
	calls := 0
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		calls++
		if calls == 1 {
			return streamEvents(provider.MessageStart{MessageID: "a1"}, provider.ToolCall{ID: "c1", Name: "stub", Arguments: originalArgs}, provider.MessageEnd{StopReason: "tool_use"}), nil
		}
		return streamEvents(provider.MessageStart{MessageID: "a2"}, provider.TextDelta{Text: "done"}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("test"),
		Stream:  stream,
		Model:   model.Model{Provider: "fake", ID: "test"},
		Tools:   tool.NewRegistry(&recordingTool{name: "stub"}),
		LoopHooks: loop.Hooks{BeforeToolCall: func(ctx context.Context, tc loop.ToolCallContext) (loop.ToolDecision, error) {
			order = append(order, "loop")
			return loop.ToolDecision{Arguments: loopArgs}, nil
		}},
		Hooks: harness.Hooks{
			ToolCall: func(ctx context.Context, tc harness.ToolCallContext) (harness.ToolCallDecision, error) {
				order = append(order, "harness")
				if string(tc.Call.Arguments) != string(loopArgs) {
					t.Fatalf("hook args = %s, want %s", tc.Call.Arguments, loopArgs)
				}
				result := tool.Result{IsError: true, Content: []message.Content{message.Text{Text: "blocked by harness"}}}
				return harness.ToolCallDecision{Deny: true, Result: &result}, nil
			},
		},
		ConfirmToolCall: func(context.Context, loop.ToolCallContext) (bool, error) {
			order = append(order, "confirm")
			return true, nil
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
	if got := strings.Join(order, ","); got != "loop,harness" {
		t.Fatalf("hook order = %s, want loop,harness", got)
	}
	built, err := h.Session().BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result := built.Messages[2].Content[0].(message.ToolResult)
	if !result.IsError || result.Content[0].(message.Text).Text != "blocked by harness" {
		t.Fatalf("tool result = %+v", result)
	}
}

type recordingTool struct {
	name string
	args json.RawMessage
}

func (t *recordingTool) Name() string            { return t.name }
func (t *recordingTool) Description() string     { return t.name }
func (t *recordingTool) Schema() json.RawMessage { return nil }
func (t *recordingTool) Execute(_ context.Context, args json.RawMessage, _ func(tool.Progress)) (tool.Result, error) {
	t.args = append(json.RawMessage(nil), args...)
	return tool.Result{Content: []message.Content{message.Text{Text: "ran"}}}, nil
}
