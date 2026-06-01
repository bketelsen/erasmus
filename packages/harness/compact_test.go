package harness_test

import (
	"context"
	"strings"
	"testing"

	"erasmus/packages/compact"
	"erasmus/packages/event"
	"erasmus/packages/harness"
	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/session/memory"
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
