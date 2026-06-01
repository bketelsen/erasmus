package compact_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bketelsen/erasmus/packages/compact"
	"github.com/bketelsen/erasmus/packages/message"
)

func TestPrepareAndRun(t *testing.T) {
	messages := []message.Message{
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "one"}}},
		{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "two"}}},
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "three"}}},
	}
	prep, err := compact.Prepare(messages, compact.Options{KeepTail: 1, CustomInstructions: "be brief"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prep.Summarize) != 2 || len(prep.Keep) != 1 {
		t.Fatalf("prep = %+v", prep)
	}
	res, err := compact.Run(context.Background(), nil, prep)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Summary, "one") || !strings.Contains(res.Summary, "be brief") {
		t.Fatalf("summary = %q", res.Summary)
	}
	if len(res.Messages) != 2 || res.Messages[0].Role != message.RoleSystem {
		t.Fatalf("messages = %+v", res.Messages)
	}
}
