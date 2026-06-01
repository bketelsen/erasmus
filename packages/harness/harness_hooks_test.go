package harness_test

import (
	"context"
	"encoding/json"
	"errors"
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

func TestHarnessToolResultHookPatchesResult(t *testing.T) {
	ctx := context.Background()
	calls := 0
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		calls++
		if calls == 1 {
			return streamEvents(provider.MessageStart{MessageID: "a1"}, provider.ToolCall{ID: "c1", Name: "stub", Arguments: json.RawMessage(`{}`)}, provider.MessageEnd{StopReason: "tool_use"}), nil
		}
		return streamEvents(provider.MessageStart{MessageID: "a2"}, provider.TextDelta{Text: "done"}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("test"),
		Stream:  stream,
		Model:   model.Model{Provider: "fake", ID: "test"},
		Tools:   tool.NewRegistry(&recordingTool{name: "stub"}),
		Hooks: harness.Hooks{
			ToolResult: func(ctx context.Context, tr harness.ToolResultContext) (harness.ToolResultPatch, error) {
				if got := tr.Result.Content[0].(message.Text).Text; got != "ran" {
					t.Fatalf("hook result = %q, want ran", got)
				}
				result := tool.Result{Content: []message.Content{message.Text{Text: "patched by harness"}}}
				return harness.ToolResultPatch{Result: &result}, nil
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
	built, err := h.Session().BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result := built.Messages[2].Content[0].(message.ToolResult)
	if got := result.Content[0].(message.Text).Text; got != "patched by harness" {
		t.Fatalf("tool result = %q, want patched by harness", got)
	}
}

func TestHarnessToolResultHookRunsAfterLoopHook(t *testing.T) {
	ctx := context.Background()
	order := []string{}
	calls := 0
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		calls++
		if calls == 1 {
			return streamEvents(provider.MessageStart{MessageID: "a1"}, provider.ToolCall{ID: "c1", Name: "stub", Arguments: json.RawMessage(`{}`)}, provider.MessageEnd{StopReason: "tool_use"}), nil
		}
		return streamEvents(provider.MessageStart{MessageID: "a2"}, provider.TextDelta{Text: "done"}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("test"),
		Stream:  stream,
		Model:   model.Model{Provider: "fake", ID: "test"},
		Tools:   tool.NewRegistry(&recordingTool{name: "stub"}),
		LoopHooks: loop.Hooks{AfterToolCall: func(ctx context.Context, tr loop.ToolResultContext) (loop.ToolResultPatch, error) {
			order = append(order, "loop")
			result := tool.Result{Content: []message.Content{message.Text{Text: "patched by loop"}}}
			return loop.ToolResultPatch{Result: &result}, nil
		}},
		Hooks: harness.Hooks{
			ToolResult: func(ctx context.Context, tr harness.ToolResultContext) (harness.ToolResultPatch, error) {
				order = append(order, "harness")
				if got := tr.Result.Content[0].(message.Text).Text; got != "patched by loop" {
					t.Fatalf("hook result = %q, want patched by loop", got)
				}
				result := tool.Result{Content: []message.Content{message.Text{Text: "patched by harness"}}}
				return harness.ToolResultPatch{Result: &result}, nil
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
	if got := strings.Join(order, ","); got != "loop,harness" {
		t.Fatalf("hook order = %s, want loop,harness", got)
	}
	built, err := h.Session().BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result := built.Messages[2].Content[0].(message.ToolResult)
	if got := result.Content[0].(message.Text).Text; got != "patched by harness" {
		t.Fatalf("tool result = %q, want patched by harness", got)
	}
}

func TestHarnessBeforeProviderRequestCanMutateRequest(t *testing.T) {
	ctx := context.Background()
	seen := false
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		seen = true
		if req.MaxTokens != 123 {
			t.Fatalf("max tokens = %d, want 123", req.MaxTokens)
		}
		if req.Meta["source"] != "harness-hook" {
			t.Fatalf("meta = %+v, want source harness-hook", req.Meta)
		}
		return streamEvents(provider.MessageStart{MessageID: "a1"}, provider.TextDelta{Text: "done"}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("test"),
		Stream:  stream,
		Model:   model.Model{Provider: "fake", ID: "test"},
		Hooks: harness.Hooks{
			BeforeProviderRequest: func(ctx context.Context, req *provider.Request) error {
				req.MaxTokens = 123
				req.Meta = map[string]string{"source": "harness-hook"}
				return nil
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
	if !seen {
		t.Fatal("provider stream was not called")
	}
}

func TestHarnessBeforeProviderRequestErrorStopsRun(t *testing.T) {
	ctx := context.Background()
	hookErr := errors.New("request blocked")
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		t.Fatal("provider stream should not be called")
		return nil, nil
	}
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("test"),
		Stream:  stream,
		Model:   model.Model{Provider: "fake", ID: "test"},
		Hooks: harness.Hooks{
			BeforeProviderRequest: func(context.Context, *provider.Request) error {
				return hookErr
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
	if err := h.Wait(ctx); !errors.Is(err, hookErr) {
		t.Fatalf("wait error = %v, want %v", err, hookErr)
	}
	drain(events)
}

func TestHarnessAfterProviderResponseObservesEvents(t *testing.T) {
	ctx := context.Background()
	var observed harness.ProviderResponseContext
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		return streamEvents(provider.MessageStart{MessageID: "a1"}, provider.TextDelta{Text: "done"}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("test"),
		Stream:  stream,
		Model:   model.Model{Provider: "fake", ID: "test"},
		Hooks: harness.Hooks{
			AfterProviderResponse: func(ctx context.Context, response harness.ProviderResponseContext) error {
				observed = response
				return nil
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
	if observed.Request.Model.ID != "test" {
		t.Fatalf("observed request model = %+v", observed.Request.Model)
	}
	if len(observed.Events) != 3 {
		t.Fatalf("events len = %d, want 3", len(observed.Events))
	}
	if delta, ok := observed.Events[1].(provider.TextDelta); !ok || delta.Text != "done" {
		t.Fatalf("event[1] = %#v, want text delta done", observed.Events[1])
	}
}

func TestHarnessAfterProviderResponseErrorStopsRun(t *testing.T) {
	ctx := context.Background()
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		return streamEvents(provider.MessageStart{MessageID: "a1"}, provider.TextDelta{Text: "done"}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("test"),
		Stream:  stream,
		Model:   model.Model{Provider: "fake", ID: "test"},
		Hooks: harness.Hooks{
			AfterProviderResponse: func(context.Context, harness.ProviderResponseContext) error {
				return errors.New("response blocked")
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
	if err := h.Wait(ctx); err == nil || err.Error() != "response blocked" {
		t.Fatalf("wait error = %v, want response blocked", err)
	}
	drain(events)
}

func TestHarnessBeforeAgentStartCanPatchPrompt(t *testing.T) {
	ctx := context.Background()
	seen := false
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		seen = true
		if len(req.Messages) != 1 {
			t.Fatalf("messages len = %d, want 1", len(req.Messages))
		}
		if got := req.Messages[0].Content[0].(message.Text).Text; got != "patched prompt" {
			t.Fatalf("prompt = %q, want patched prompt", got)
		}
		return streamEvents(provider.MessageStart{MessageID: "a1"}, provider.TextDelta{Text: "done"}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("test"),
		Stream:  stream,
		Model:   model.Model{Provider: "fake", ID: "test"},
		Hooks: harness.Hooks{
			BeforeAgentStart: func(ctx context.Context, start harness.BeforeAgentStartContext) (harness.BeforeAgentStartResult, error) {
				if start.Action != "prompt" {
					t.Fatalf("action = %q, want prompt", start.Action)
				}
				if start.Prompt != "original prompt" {
					t.Fatalf("prompt = %q, want original prompt", start.Prompt)
				}
				prompt := "patched prompt"
				return harness.BeforeAgentStartResult{Prompt: &prompt}, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := h.Prompt(ctx, "original prompt", harness.PromptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	drain(events)
	if !seen {
		t.Fatal("provider stream was not called")
	}
}

func TestHarnessBeforeAgentStartCanRejectContinue(t *testing.T) {
	ctx := context.Background()
	hookErr := errors.New("continue blocked")
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		t.Fatal("provider stream should not be called")
		return nil, nil
	}
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("test"),
		Stream:  stream,
		Model:   model.Model{Provider: "fake", ID: "test"},
		Hooks: harness.Hooks{
			BeforeAgentStart: func(ctx context.Context, start harness.BeforeAgentStartContext) (harness.BeforeAgentStartResult, error) {
				if start.Action != "continue" {
					t.Fatalf("action = %q, want continue", start.Action)
				}
				return harness.BeforeAgentStartResult{}, hookErr
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Continue(ctx); !errors.Is(err, hookErr) {
		t.Fatalf("continue error = %v, want %v", err, hookErr)
	}
	if err := h.Wait(ctx); err != nil {
		t.Fatalf("wait error = %v, want nil", err)
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
