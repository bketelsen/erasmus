package jsonl

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/session"
)

func TestTreeMoveBranchAndReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	s, err := Open(path, session.Metadata{ID: "json-tree"})
	if err != nil {
		t.Fatal(err)
	}
	one, err := s.AppendMessage(ctx, message.Message{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "one"}}})
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.AppendMessage(ctx, message.Message{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "two"}}})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := s.LeafID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if leaf != two {
		t.Fatalf("leaf = %q, want %q", leaf, two)
	}
	entries, err := s.Entries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[1].Parent != one {
		t.Fatalf("unexpected entries: %+v", entries)
	}

	if err := s.MoveTo(ctx, one, nil); err != nil {
		t.Fatal(err)
	}
	three, err := s.AppendMessage(ctx, message.Message{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "three"}}})
	if err != nil {
		t.Fatal(err)
	}
	if three == two {
		t.Fatal("expected branch entry")
	}
	built, err := s.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Messages) != 2 || textOf(built.Messages[1]) != "three" {
		t.Fatalf("unexpected active context: %+v", built.Messages)
	}
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path, session.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close(ctx)
	reopenedCtx, err := reopened.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopenedCtx.Messages) != 2 || textOf(reopenedCtx.Messages[1]) != "three" {
		t.Fatalf("unexpected reopened context: %+v", reopenedCtx.Messages)
	}

	branch, err := reopened.Branch(ctx, two)
	if err != nil {
		t.Fatal(err)
	}
	defer branch.Close(ctx)
	branchCtx, err := branch.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(branchCtx.Messages) != 2 || textOf(branchCtx.Messages[1]) != "two" {
		t.Fatalf("unexpected branch context: %+v", branchCtx.Messages)
	}

	if err := reopened.MoveTo(ctx, one, &session.BranchSummary{Summary: "switched"}); err != nil {
		t.Fatal(err)
	}
	leaf, err = reopened.LeafID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if leaf == one {
		t.Fatal("expected summary marker leaf")
	}
}

func textOf(msg message.Message) string {
	for _, c := range msg.Content {
		if text, ok := c.(message.Text); ok {
			return text.Text
		}
	}
	return ""
}
