package tui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"erasmus/packages/harness"
	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/provider"
	"erasmus/packages/session/memory"

	tea "charm.land/bubbletea/v2"
)

func TestAppRunPromptStateQuit(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			ch := make(chan provider.Event, 3)
			ch <- provider.MessageStart{MessageID: "m1"}
			ch <- provider.TextDelta{Text: "tui response: " + lastUserText(req)}
			ch <- provider.MessageEnd{StopReason: "end_turn"}
			close(ch)
			return ch, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	app := &App{Harness: h, In: strings.NewReader("hello\n/help\n/state\n/model\n/tree\n/move 1 back\n/branch 1\n/compact\n/quit\n"), Out: &out}
	if err := app.Run(ctx); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"Erasmus TUI MVP", "you: hello", "assistant: tui response: hello", "commands:", "session: tui-test", "messages: 2", "model=fake/echo", "model: fake/echo", "leaf=", "entries=2", "branch session=", "compacted:", "bye"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestModelDialogTabsBetweenProviders(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-model-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := newBubbleModel(ctx, &App{Harness: h})
	opened, _ := m.openModelDialog()
	if len(opened.models) == 0 || opened.models[opened.selectedModel].Provider != "fake" {
		t.Fatalf("opened provider = %q, want fake", opened.models[opened.selectedModel].Provider)
	}
	updated, _ := opened.updateModelDialog(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	afterTab := updated.(bubbleModel)
	if len(afterTab.models) == 0 || afterTab.models[afterTab.selectedModel].Provider != "openai" {
		t.Fatalf("tab provider = %q, want openai", afterTab.models[afterTab.selectedModel].Provider)
	}
	updated, _ = afterTab.updateModelDialog(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
	afterShiftTab := updated.(bubbleModel)
	if len(afterShiftTab.models) == 0 || afterShiftTab.models[afterShiftTab.selectedModel].Provider != "fake" {
		t.Fatalf("shift+tab provider = %q, want fake", afterShiftTab.models[afterShiftTab.selectedModel].Provider)
	}
}

func TestModelDialogApplyUsesAppCallback(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-model-apply-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var applied model.Model
	var appliedReasoning string
	app := &App{
		Harness: h,
		ApplyModel: func(ctx context.Context, selected model.Model, reasoning string) error {
			applied = selected
			appliedReasoning = reasoning
			return nil
		},
	}
	m := newBubbleModel(ctx, app)
	opened, _ := m.openModelDialog()
	updated, _ := opened.updateModelDialog(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	afterTab := updated.(bubbleModel)
	updated, _ = afterTab.updateModelDialog(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	afterReasoning := updated.(bubbleModel)
	_, cmd := afterReasoning.updateModelDialog(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("expected apply command")
	}
	if msg := cmd(); msg != (modelAppliedMsg{}) {
		t.Fatalf("apply msg = %#v, want empty modelAppliedMsg", msg)
	}
	if applied.Provider != "openai" || applied.ID != "gpt-4o-mini" {
		t.Fatalf("applied model = %s/%s, want openai/gpt-4o-mini", applied.Provider, applied.ID)
	}
	if appliedReasoning != "low" {
		t.Fatalf("applied reasoning = %q, want low", appliedReasoning)
	}
}

func streamText(text string) <-chan provider.Event {
	ch := make(chan provider.Event, 3)
	ch <- provider.MessageStart{MessageID: "m1"}
	ch <- provider.TextDelta{Text: text}
	ch <- provider.MessageEnd{StopReason: "end_turn"}
	close(ch)
	return ch
}

func lastUserText(req provider.Request) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role != message.RoleUser {
			continue
		}
		for _, c := range req.Messages[i].Content {
			if text, ok := c.(message.Text); ok {
				return text.Text
			}
		}
	}
	return ""
}
