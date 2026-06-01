package jsonl_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/session"
	"github.com/bketelsen/erasmus/packages/session/jsonl"
)

func TestRoundTripReopenBuildContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	ctx := context.Background()

	s, err := jsonl.Open(path, session.Metadata{ID: "s1", CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	args := json.RawMessage(`{"path":"hello.txt"}`)
	if _, err := s.AppendMessage(ctx, message.Message{ID: "u1", Role: message.RoleUser, Content: []message.Content{message.Text{Text: "hi"}}, Time: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendMessage(ctx, message.Message{ID: "a1", Role: message.RoleAssistant, Content: []message.Content{message.ToolCall{ID: "c1", Name: "read", Arguments: args}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendMessage(ctx, message.Message{Role: message.RoleTool, Content: []message.Content{message.ToolResult{CallID: "c1", Content: []message.Content{message.Text{Text: "contents"}}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendUsage(ctx, model.Usage{InputTokens: 1}, model.Usage{InputTokens: 3, OutputTokens: 2}); err != nil {
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
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}

	reopened, err := jsonl.Open(path, session.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close(ctx)
	meta, err := reopened.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ID != "s1" || meta.CWD != "/tmp" {
		t.Fatalf("metadata = %+v", meta)
	}
	built, err := reopened.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Messages) != 3 {
		t.Fatalf("messages len = %d, want 3", len(built.Messages))
	}
	call := built.Messages[1].Content[0].(message.ToolCall)
	if call.ID != "c1" || call.Name != "read" || string(call.Arguments) != string(args) {
		t.Fatalf("tool call = %+v", call)
	}
	result := built.Messages[2].Content[0].(message.ToolResult)
	if got := result.Content[0].(message.Text).Text; got != "contents" {
		t.Fatalf("tool result text = %q", got)
	}
	if built.Usage.InputTokens != 3 || built.Usage.OutputTokens != 2 {
		t.Fatalf("usage = %+v", built.Usage)
	}
	if built.Model.Provider != "fake" || built.Model.ID != "test" {
		t.Fatalf("model = %+v", built.Model)
	}
	if built.Reasoning != "low" {
		t.Fatalf("reasoning = %q", built.Reasoning)
	}
	if len(built.ActiveTools) != 1 || built.ActiveTools[0] != "read" {
		t.Fatalf("tools = %v", built.ActiveTools)
	}
}

func TestCompactionReplacesPriorContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	ctx := context.Background()
	s, err := jsonl.Open(path, session.Metadata{ID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendMessage(ctx, message.Message{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "old"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendCompaction(ctx, session.Compaction{Summary: "summary"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendMessage(ctx, message.Message{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "new"}}}); err != nil {
		t.Fatal(err)
	}
	built, err := s.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(built.Messages))
	}
	if got := built.Messages[0].Content[0].(message.Text).Text; got != "summary" {
		t.Fatalf("first message = %q", got)
	}
	if got := built.Messages[1].Content[0].(message.Text).Text; got != "new" {
		t.Fatalf("second message = %q", got)
	}
}
