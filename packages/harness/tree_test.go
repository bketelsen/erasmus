package harness

import (
	"context"
	"testing"

	"erasmus/packages/event"
	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/provider"
	"erasmus/packages/session"
	"erasmus/packages/session/memory"
)

func TestTreeMoveToAndBranch(t *testing.T) {
	ctx := context.Background()
	sess := memory.New("tree-harness")
	one, err := sess.AppendMessage(ctx, message.Message{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "one"}}})
	if err != nil {
		t.Fatal(err)
	}
	two, err := sess.AppendMessage(ctx, message.Message{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "two"}}})
	if err != nil {
		t.Fatal(err)
	}
	h, err := New(ctx, Config{
		Session: sess,
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			ch := make(chan provider.Event)
			close(ch)
			return ch, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var treeEvents []event.SessionTree
	h.Subscribe(func(ev event.Event) {
		if tree, ok := ev.(event.SessionTree); ok {
			treeEvents = append(treeEvents, tree)
		}
	})

	tree, err := h.Tree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tree.LeafID != two || len(tree.Entries) != 2 {
		t.Fatalf("unexpected tree: %+v", tree)
	}
	if err := h.MoveTo(ctx, one, &session.BranchSummary{Summary: "switch"}); err != nil {
		t.Fatal(err)
	}
	state := h.State(ctx)
	if len(state.Agent.Messages) != 1 || textOf(state.Agent.Messages[0]) != "one" {
		t.Fatalf("unexpected moved state: %+v", state.Agent.Messages)
	}
	if len(treeEvents) != 1 || treeEvents[0].Action != "move_to" {
		t.Fatalf("expected move event, got %+v", treeEvents)
	}

	branched, err := h.Branch(ctx, two)
	if err != nil {
		t.Fatal(err)
	}
	if branched.ID() == sess.ID() {
		t.Fatal("expected branch session id to differ")
	}
	if len(treeEvents) != 2 || treeEvents[1].Action != "branch" {
		t.Fatalf("expected branch event, got %+v", treeEvents)
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
