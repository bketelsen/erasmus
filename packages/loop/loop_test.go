package loop_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"erasmus/packages/event"
	"erasmus/packages/loop"
	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/provider"
	"erasmus/packages/provider/fake"
	"erasmus/packages/sandbox"
	"erasmus/packages/tool"
	"erasmus/packages/tools"
)

func TestRunTextOnly(t *testing.T) {
	client := &fake.Client{Script: []provider.Event{
		provider.MessageStart{MessageID: "a1"},
		provider.TextDelta{Text: "hello, "},
		provider.TextDelta{Text: "world"},
		provider.Usage{Usage: model.Usage{InputTokens: 3, OutputTokens: 2}},
		provider.MessageEnd{StopReason: "end_turn"},
	}}

	var events []string
	messages, err := loop.Run(context.Background(), []message.Message{{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "hi"}}}}, loop.Context{}, loop.Config{Model: model.Model{Provider: "fake", ID: "test"}, Stream: client.StreamFunc()}, func(ev event.Event) error {
		events = append(events, ev.Type())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	assistant := messages[1]
	if assistant.Role != message.RoleAssistant {
		t.Fatalf("assistant role = %q", assistant.Role)
	}
	if got := assistant.Content[0].(message.Text).Text; got != "hello, world" {
		t.Fatalf("assistant text = %q", got)
	}
	wantEvents := []string{"agent_start", "turn_start", "message_start", "message_delta", "message_delta", "usage", "message_end", "turn_end", "agent_end"}
	assertEvents(t, events, wantEvents)
}

func TestRunToolCallContinuesToFinalAnswer(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("file contents"), 0o644); err != nil {
		t.Fatal(err)
	}
	policy, err := sandbox.New(root)
	if err != nil {
		t.Fatal(err)
	}
	registry := tool.NewRegistry(tools.NewReadTool(policy))

	calls := 0
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		calls++
		if calls == 1 {
			args, _ := json.Marshal(map[string]string{"path": "hello.txt"})
			return streamEvents(ctx,
				provider.MessageStart{MessageID: "a1"},
				provider.ToolCall{ID: "call1", Name: "read", Arguments: args},
				provider.MessageEnd{StopReason: "tool_use"},
			), nil
		}
		if len(req.Messages) != 3 {
			t.Fatalf("second request messages len = %d, want 3", len(req.Messages))
		}
		return streamEvents(ctx,
			provider.MessageStart{MessageID: "a2"},
			provider.TextDelta{Text: "read: file contents"},
			provider.MessageEnd{StopReason: "end_turn"},
		), nil
	}

	var events []string
	messages, err := loop.Run(context.Background(), []message.Message{{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "read hello"}}}}, loop.Context{Tools: registry}, loop.Config{Model: model.Model{Provider: "fake", ID: "test"}, Stream: stream, MaxSteps: 4}, func(ev event.Event) error {
		events = append(events, ev.Type())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("stream calls = %d, want 2", calls)
	}
	if len(messages) != 4 {
		t.Fatalf("messages len = %d, want 4", len(messages))
	}
	if messages[2].Role != message.RoleTool {
		t.Fatalf("third message role = %q, want tool", messages[2].Role)
	}
	finalText := messages[3].Content[0].(message.Text).Text
	if finalText != "read: file contents" {
		t.Fatalf("final text = %q", finalText)
	}
	wantEvents := []string{"agent_start", "turn_start", "message_start", "message_end", "tool_execution_start", "tool_execution_progress", "tool_execution_end", "turn_end", "turn_start", "message_start", "message_delta", "message_end", "turn_end", "agent_end"}
	assertEvents(t, events, wantEvents)
}

func TestRunToolErrorContinuesToModel(t *testing.T) {
	root := t.TempDir()
	policy, err := sandbox.New(root)
	if err != nil {
		t.Fatal(err)
	}
	registry := tool.NewRegistry(tools.NewReadTool(policy))

	calls := 0
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		calls++
		if calls == 1 {
			args, _ := json.Marshal(map[string]string{"path": "missing.txt"})
			return streamEvents(ctx,
				provider.MessageStart{MessageID: "a1"},
				provider.ToolCall{ID: "call1", Name: "read", Arguments: args},
				provider.MessageEnd{StopReason: "tool_use"},
			), nil
		}
		if len(req.Messages) != 3 {
			t.Fatalf("second request messages len = %d, want 3", len(req.Messages))
		}
		toolResult := req.Messages[2].Content[0].(message.ToolResult)
		if !toolResult.IsError {
			t.Fatal("tool result should be marked error")
		}
		return streamEvents(ctx,
			provider.MessageStart{MessageID: "a2"},
			provider.TextDelta{Text: "README missing; summarized from directory listing instead"},
			provider.MessageEnd{StopReason: "end_turn"},
		), nil
	}

	messages, err := loop.Run(context.Background(), []message.Message{{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "summarize"}}}}, loop.Context{Tools: registry}, loop.Config{Model: model.Model{Provider: "fake", ID: "test"}, Stream: stream, MaxSteps: 4}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("stream calls = %d, want 2", calls)
	}
	if len(messages) != 4 {
		t.Fatalf("messages len = %d, want 4", len(messages))
	}
	if got := messages[3].Content[0].(message.Text).Text; got != "README missing; summarized from directory listing instead" {
		t.Fatalf("final text = %q", got)
	}
}

func TestRunUnknownToolContinuesToModel(t *testing.T) {
	calls := 0
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		calls++
		if calls == 1 {
			return streamEvents(ctx,
				provider.MessageStart{MessageID: "a1"},
				provider.ToolCall{ID: "call1", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`)},
				provider.MessageEnd{StopReason: "tool_use"},
			), nil
		}
		toolResult := req.Messages[2].Content[0].(message.ToolResult)
		if !toolResult.IsError {
			t.Fatal("unknown tool result should be marked error")
		}
		return streamEvents(ctx, provider.MessageStart{MessageID: "a2"}, provider.TextDelta{Text: "used available tools instead"}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}

	messages, err := loop.Run(context.Background(), []message.Message{{Role: message.RoleUser}}, loop.Context{Tools: tool.NewRegistry()}, loop.Config{Model: model.Model{Provider: "fake", ID: "test"}, Stream: stream, MaxSteps: 4}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || messages[3].Content[0].(message.Text).Text != "used available tools instead" {
		t.Fatalf("calls=%d messages=%v", calls, messages)
	}
}

func TestRunToolHooksPatchArgumentsAndResult(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}
	policy, err := sandbox.New(root)
	if err != nil {
		t.Fatal(err)
	}
	registry := tool.NewRegistry(tools.NewReadTool(policy))

	calls := 0
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		calls++
		if calls == 1 {
			args, _ := json.Marshal(map[string]string{"path": "a.txt"})
			return streamEvents(ctx, provider.MessageStart{MessageID: "a1"}, provider.ToolCall{ID: "call1", Name: "read", Arguments: args}, provider.MessageEnd{StopReason: "tool_use"}), nil
		}
		return streamEvents(ctx, provider.MessageStart{MessageID: "a2"}, provider.TextDelta{Text: "done"}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}

	patchedArgs, _ := json.Marshal(map[string]string{"path": "b.txt"})
	patchedResult := tool.Result{Content: []message.Content{message.Text{Text: "patched"}}}
	messages, err := loop.Run(context.Background(), []message.Message{{Role: message.RoleUser}}, loop.Context{Tools: registry}, loop.Config{
		Model:    model.Model{Provider: "fake", ID: "test"},
		Stream:   stream,
		MaxSteps: 3,
		Hooks: loop.Hooks{
			BeforeToolCall: func(ctx context.Context, tc loop.ToolCallContext) (loop.ToolDecision, error) {
				if tc.Call.Name != "read" {
					t.Fatalf("hook call name = %q", tc.Call.Name)
				}
				return loop.ToolDecision{Arguments: patchedArgs}, nil
			},
			AfterToolCall: func(ctx context.Context, rc loop.ToolResultContext) (loop.ToolResultPatch, error) {
				if got := rc.Result.Content[0].(message.Text).Text; got != "B" {
					t.Fatalf("before patch result = %q, want B", got)
				}
				return loop.ToolResultPatch{Result: &patchedResult}, nil
			},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	toolResult := messages[2].Content[0].(message.ToolResult)
	if got := toolResult.Content[0].(message.Text).Text; got != "patched" {
		t.Fatalf("tool result = %q, want patched", got)
	}
}

func TestRunToolHookCanDeny(t *testing.T) {
	root := t.TempDir()
	policy, err := sandbox.New(root)
	if err != nil {
		t.Fatal(err)
	}
	registry := tool.NewRegistry(tools.NewReadTool(policy))
	args, _ := json.Marshal(map[string]string{"path": "missing.txt"})

	calls := 0
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		calls++
		if calls == 1 {
			return streamEvents(ctx, provider.MessageStart{MessageID: "a1"}, provider.ToolCall{ID: "call1", Name: "read", Arguments: args}, provider.MessageEnd{StopReason: "tool_use"}), nil
		}
		return streamEvents(ctx, provider.MessageStart{MessageID: "a2"}, provider.TextDelta{Text: "done"}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}

	denyResult := tool.Result{IsError: true, Content: []message.Content{message.Text{Text: "denied by test"}}}
	messages, err := loop.Run(context.Background(), []message.Message{{Role: message.RoleUser}}, loop.Context{Tools: registry}, loop.Config{
		Model:    model.Model{Provider: "fake", ID: "test"},
		Stream:   stream,
		MaxSteps: 3,
		Hooks: loop.Hooks{BeforeToolCall: func(ctx context.Context, tc loop.ToolCallContext) (loop.ToolDecision, error) {
			return loop.ToolDecision{Deny: true, Result: &denyResult}, nil
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	toolResult := messages[2].Content[0].(message.ToolResult)
	if !toolResult.IsError {
		t.Fatal("expected denied tool result to be an error")
	}
	if got := toolResult.Content[0].(message.Text).Text; got != "denied by test" {
		t.Fatalf("tool result = %q", got)
	}
}

func TestRunStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		ch := make(chan provider.Event)
		go func() {
			defer close(ch)
			ch <- provider.MessageStart{MessageID: "a1"}
			cancel()
			<-ctx.Done()
		}()
		return ch, nil
	}

	_, err := loop.Run(ctx, []message.Message{{Role: message.RoleUser}}, loop.Context{}, loop.Config{Model: model.Model{Provider: "fake", ID: "test"}, Stream: stream}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestRunStopsWhenProviderStreamBlocksAndContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		ch := make(chan provider.Event)
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()
		return ch, nil
	}

	_, err := loop.Run(ctx, []message.Message{{Role: message.RoleUser}}, loop.Context{}, loop.Config{Model: model.Model{Provider: "fake", ID: "test"}, Stream: stream}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func streamEvents(ctx context.Context, events ...provider.Event) <-chan provider.Event {
	ch := make(chan provider.Event, len(events))
	for _, ev := range events {
		ch <- ev
	}
	close(ch)
	return ch
}

func assertEvents(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}
}
