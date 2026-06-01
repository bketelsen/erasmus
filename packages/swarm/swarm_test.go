package swarm

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/bketelsen/erasmus/packages/harness"
	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/provider"
	"github.com/bketelsen/erasmus/packages/session/memory"
)

func TestSwarmSpawnListSendResumeStop(t *testing.T) {
	ctx := context.Background()
	eventLogDir := t.TempDir()
	s, err := New(Config{EventLogDir: eventLogDir, Factory: func(ctx context.Context, req SpawnRequest) (*harness.Harness, error) {
		return harness.New(ctx, harness.Config{
			Session: memory.New(req.ID),
			Model:   model.Model{Provider: "fake", ID: "echo"},
			Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
				ch := make(chan provider.Event, 3)
				ch <- provider.MessageStart{MessageID: "m1"}
				ch <- provider.TextDelta{Text: "swarm response: " + lastUserText(req)}
				ch <- provider.MessageEnd{StopReason: "end_turn"}
				close(ch)
				return ch, nil
			},
		})
	}})
	if err != nil {
		t.Fatal(err)
	}

	a, err := s.Spawn(ctx, SpawnRequest{ID: "worker", Task: "first task", CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if len(a.Events()) == 0 {
		t.Fatal("expected event log")
	}

	list, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "worker" || list[0].SessionID != "worker" || list[0].Running || list[0].State != "settled" || list[0].EventLog == "" {
		t.Fatalf("unexpected list: %+v", list)
	}
	if list[0].Provider != "fake" || list[0].Model != "echo" || list[0].Messages == 0 || list[0].LastEventType == "" || list[0].LastEventAt.IsZero() {
		t.Fatalf("expected supervision metadata, got %+v", list[0])
	}
	data, err := os.ReadFile(list[0].EventLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"type":"message_delta"`) || !strings.Contains(string(data), "swarm response: first task") {
		t.Fatalf("unexpected event log:\n%s", string(data))
	}

	resumed, err := s.Resume(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ID() != "worker" {
		t.Fatalf("unexpected resumed id %q", resumed.ID())
	}

	if err := s.Send(ctx, "worker", "second task"); err != nil {
		t.Fatal(err)
	}
	if err := resumed.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	state := resumed.Harness().State(ctx)
	if len(state.Agent.Messages) < 4 {
		t.Fatalf("expected messages after send, got %d", len(state.Agent.Messages))
	}

	if err := s.Stop(ctx, "worker"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resume(ctx, "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing error, got %v", err)
	}
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
