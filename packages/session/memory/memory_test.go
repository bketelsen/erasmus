package memory_test

import (
	"context"
	"testing"

	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/session/memory"
)

func TestSessionBuildContext(t *testing.T) {
	s := memory.New("test")
	ctx := context.Background()
	if s.ID() != "test" {
		t.Fatalf("id = %q", s.ID())
	}
	if _, err := s.AppendMessage(ctx, message.Message{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendUsage(ctx, model.Usage{InputTokens: 1}, model.Usage{InputTokens: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendModelChange(ctx, "fake", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendReasoningChange(ctx, "low"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendActiveToolsChange(ctx, []string{"read"}); err != nil {
		t.Fatal(err)
	}

	built, err := s.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(built.Messages))
	}
	if built.Usage.InputTokens != 3 {
		t.Fatalf("usage = %+v", built.Usage)
	}
	if built.Model.Provider != "fake" || built.Model.ID != "test" {
		t.Fatalf("model = %+v", built.Model)
	}
	if built.Reasoning != "low" {
		t.Fatalf("reasoning = %q", built.Reasoning)
	}
	if len(built.ActiveTools) != 1 || built.ActiveTools[0] != "read" {
		t.Fatalf("active tools = %v", built.ActiveTools)
	}
}

func TestSessionClose(t *testing.T) {
	s := memory.New("test")
	if err := s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendMessage(context.Background(), message.Message{Role: message.RoleUser}); err == nil {
		t.Fatal("expected append after close to fail")
	}
}
