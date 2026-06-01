package harness_test

import (
	"context"
	"path/filepath"
	"testing"

	"erasmus/packages/event"
	"erasmus/packages/harness"
	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/provider"
	"erasmus/packages/session"
	"erasmus/packages/session/jsonl"
	"erasmus/packages/session/memory"
)

func TestHarnessPersistsMessagesAndReopenContinue(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	s, err := jsonl.Open(path, session.Metadata{ID: "s1"})
	if err != nil {
		t.Fatal(err)
	}

	calls := 0
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		calls++
		switch calls {
		case 1:
			return streamEvents(provider.MessageStart{MessageID: "a1"}, provider.TextDelta{Text: "first"}, provider.MessageEnd{StopReason: "end_turn"}), nil
		case 2:
			if len(req.Messages) != 2 {
				t.Fatalf("continue request messages len = %d, want 2", len(req.Messages))
			}
			return streamEvents(provider.MessageStart{MessageID: "a2"}, provider.TextDelta{Text: "second"}, provider.MessageEnd{StopReason: "end_turn"}), nil
		default:
			t.Fatalf("unexpected stream call %d", calls)
			return nil, nil
		}
	}

	h, err := harness.New(ctx, harness.Config{Session: s, Stream: stream, Model: model.Model{Provider: "fake", ID: "test"}})
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
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}

	reopened, err := jsonl.Open(path, session.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := reopened.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rebuilt.Messages) != 2 {
		t.Fatalf("reopened messages len = %d, want 2", len(rebuilt.Messages))
	}
	if got := rebuilt.Messages[1].Content[0].(message.Text).Text; got != "first" {
		t.Fatalf("persisted assistant = %q", got)
	}

	h2, err := harness.New(ctx, harness.Config{Session: reopened, Stream: stream, Model: model.Model{Provider: "fake", ID: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	events, err = h2.Continue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := h2.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	drain(events)
	rebuilt, err = reopened.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rebuilt.Messages) != 3 {
		t.Fatalf("after continue messages len = %d, want 3", len(rebuilt.Messages))
	}
	if got := rebuilt.Messages[2].Content[0].(message.Text).Text; got != "second" {
		t.Fatalf("continued assistant = %q", got)
	}
}

func TestHarnessSetModelAndStreamUsesNewStreamForNextPrompt(t *testing.T) {
	ctx := context.Background()
	oldCalls := 0
	newCalls := 0
	oldStream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		oldCalls++
		return streamEvents(provider.MessageStart{MessageID: "old"}, provider.TextDelta{Text: "old"}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}
	newStream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		newCalls++
		if req.Model.Provider != "openai-codex" || req.Model.ID != "gpt-5.3-codex" {
			t.Fatalf("request model = %s/%s, want openai-codex/gpt-5.3-codex", req.Model.Provider, req.Model.ID)
		}
		return streamEvents(provider.MessageStart{MessageID: "new"}, provider.TextDelta{Text: "new"}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}
	h, err := harness.New(ctx, harness.Config{Session: memory.New("switch"), Stream: oldStream, Model: model.Model{Provider: "fake", ID: "echo"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.SetModelAndStream(ctx, model.Model{Provider: "openai-codex", ID: "gpt-5.3-codex"}, newStream); err != nil {
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
	if oldCalls != 0 {
		t.Fatalf("old stream calls = %d, want 0", oldCalls)
	}
	if newCalls != 1 {
		t.Fatalf("new stream calls = %d, want 1", newCalls)
	}
}

func streamEvents(events ...provider.Event) <-chan provider.Event {
	ch := make(chan provider.Event, len(events))
	for _, ev := range events {
		ch <- ev
	}
	close(ch)
	return ch
}

func drain(ch <-chan event.Event) {
	for range ch {
	}
}
