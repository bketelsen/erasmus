package harness_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bketelsen/erasmus/packages/compact"
	"github.com/bketelsen/erasmus/packages/event"
	"github.com/bketelsen/erasmus/packages/harness"
	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/session/memory"
)

func TestHarnessCompact(t *testing.T) {
	ctx := context.Background()
	sess := memory.New("test")
	for _, msg := range []message.Message{
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "one"}}},
		{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "two"}}},
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "three"}}},
	} {
		if _, err := sess.AppendMessage(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}
	h, err := harness.New(ctx, harness.Config{Session: sess, Stream: noopStream, Model: model.Model{Provider: "fake", ID: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	var got event.Event
	h.Subscribe(func(ev event.Event) { got = ev })
	res, err := h.Compact(ctx, compact.Options{KeepTail: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Summary, "one") {
		t.Fatalf("summary = %q", res.Summary)
	}
	if _, ok := got.(event.SessionCompact); !ok {
		t.Fatalf("event = %T, want SessionCompact", got)
	}
	st := h.State(ctx)
	if len(st.Agent.Messages) != 2 || st.Agent.Messages[0].Role != message.RoleSystem {
		t.Fatalf("messages = %+v", st.Agent.Messages)
	}
	built, err := sess.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Messages) != 2 || built.Messages[0].Role != message.RoleSystem {
		t.Fatalf("session messages = %+v", built.Messages)
	}
}

func TestHarnessBeforeCompactHookCanPatchOptions(t *testing.T) {
	ctx := context.Background()
	sess := memory.New("test")
	for _, msg := range []message.Message{
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "one"}}},
		{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "two"}}},
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "three"}}},
		{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "four"}}},
	} {
		if _, err := sess.AppendMessage(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}
	h, err := harness.New(ctx, harness.Config{
		Session: sess,
		Stream:  noopStream,
		Model:   model.Model{Provider: "fake", ID: "test"},
		Hooks: harness.Hooks{
			BeforeCompact: func(ctx context.Context, c harness.BeforeCompactContext) (harness.BeforeCompactResult, error) {
				if c.Options.KeepTail != 1 {
					t.Fatalf("keep tail = %d, want 1", c.Options.KeepTail)
				}
				opts := c.Options
				opts.KeepTail = 2
				opts.CustomInstructions = "preserve decisions"
				return harness.BeforeCompactResult{Options: &opts}, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Compact(ctx, compact.Options{KeepTail: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Summary, "Instructions: preserve decisions") {
		t.Fatalf("summary = %q", res.Summary)
	}
	st := h.State(ctx)
	if len(st.Agent.Messages) != 3 {
		t.Fatalf("messages len = %d, want 3", len(st.Agent.Messages))
	}
	if got := st.Agent.Messages[1].Content[0].(message.Text).Text; got != "three" {
		t.Fatalf("first kept message = %q, want three", got)
	}
}

func TestHarnessAfterCompactHookObservesResult(t *testing.T) {
	ctx := context.Background()
	sess := memory.New("test")
	for _, msg := range []message.Message{
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "one"}}},
		{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "two"}}},
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "three"}}},
	} {
		if _, err := sess.AppendMessage(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}
	var observed compact.Result
	h, err := harness.New(ctx, harness.Config{
		Session: sess,
		Stream:  noopStream,
		Model:   model.Model{Provider: "fake", ID: "test"},
		Hooks: harness.Hooks{
			AfterCompact: func(ctx context.Context, c harness.AfterCompactContext) error {
				observed = c.Result
				return nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Compact(ctx, compact.Options{KeepTail: 1})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Summary != res.Summary || observed.TokensBefore == 0 {
		t.Fatalf("observed = %+v, result = %+v", observed, res)
	}
}
