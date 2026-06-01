package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"erasmus/packages/agent"
	"erasmus/packages/event"
	"erasmus/packages/loop"
	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/provider"
	"erasmus/packages/provider/fake"
	"erasmus/packages/tool"
)

func TestAgentPromptUpdatesStateAndPublishesEvents(t *testing.T) {
	client := &fake.Client{Script: []provider.Event{
		provider.MessageStart{MessageID: "a1"},
		provider.TextDelta{Text: "hello"},
		provider.MessageEnd{StopReason: "end_turn"},
	}}
	a := agent.New(agent.Config{LoopConfig: loopConfig(client.StreamFunc())})

	var events []string
	a.Subscribe(func(ev event.Event) { events = append(events, ev.Type()) })
	if err := a.Prompt(context.Background(), "hi", nil); err != nil {
		t.Fatal(err)
	}
	if err := a.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}

	st := a.State()
	if st.IsStreaming {
		t.Fatal("agent still streaming")
	}
	if len(st.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(st.Messages))
	}
	if got := st.Messages[1].Content[0].(message.Text).Text; got != "hello" {
		t.Fatalf("assistant text = %q", got)
	}
	if len(events) < 2 || events[0] != "agent_start" || events[len(events)-2] != "agent_end" || events[len(events)-1] != "settled" {
		t.Fatalf("events = %v", events)
	}
}

func TestAgentRejectsConcurrentPrompt(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		ch := make(chan provider.Event)
		go func() {
			close(started)
			<-release
			close(ch)
		}()
		return ch, nil
	}
	a := agent.New(agent.Config{LoopConfig: loopConfig(stream)})
	if err := a.Prompt(context.Background(), "one", nil); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := a.Prompt(context.Background(), "two", nil); err == nil {
		t.Fatal("expected concurrent prompt error")
	}
	close(release)
	if err := a.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAgentAbort(t *testing.T) {
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		ch := make(chan provider.Event)
		return ch, nil
	}
	a := agent.New(agent.Config{LoopConfig: loopConfig(stream)})
	if err := a.Prompt(context.Background(), "hi", nil); err != nil {
		t.Fatal(err)
	}
	a.Abort()
	err := a.Wait(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if !waitFor(func() bool { return !a.State().IsStreaming }) {
		t.Fatal("agent did not stop streaming")
	}
}

func TestAgentFollowUpQueuesAnotherTurn(t *testing.T) {
	calls := 0
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		calls++
		if calls == 1 {
			return streamEvents(provider.MessageStart{MessageID: "a1"}, provider.TextDelta{Text: "first"}, provider.MessageEnd{StopReason: "end_turn"}), nil
		}
		if len(req.Messages) != 3 {
			t.Fatalf("second request messages len = %d, want 3", len(req.Messages))
		}
		if got := req.Messages[2].Content[0].(message.Text).Text; got != "again" {
			t.Fatalf("follow-up text = %q", got)
		}
		return streamEvents(provider.MessageStart{MessageID: "a2"}, provider.TextDelta{Text: "second"}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}
	a := agent.New(agent.Config{LoopConfig: loop.Config{Model: model.Model{Provider: "fake", ID: "test"}, Stream: stream, MaxSteps: 3}})
	a.FollowUp(message.Message{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "again"}}})
	if err := a.Prompt(context.Background(), "start", nil); err != nil {
		t.Fatal(err)
	}
	if err := a.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("stream calls = %d, want 2", calls)
	}
	if len(a.Messages()) != 4 {
		t.Fatalf("messages len = %d, want 4", len(a.Messages()))
	}
}

func TestAgentSteerQueuesBeforeNextToolContinuation(t *testing.T) {
	calls := 0
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		calls++
		if calls == 1 {
			return streamEvents(provider.MessageStart{MessageID: "a1"}, provider.ToolCall{ID: "missing", Name: "missing"}, provider.MessageEnd{StopReason: "tool_use"}), nil
		}
		if got := req.Messages[len(req.Messages)-1].Content[0].(message.Text).Text; got != "steer" {
			t.Fatalf("last message = %q, want steer", got)
		}
		return streamEvents(provider.MessageStart{MessageID: "a2"}, provider.TextDelta{Text: "ok"}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}
	stub := toolStub{name: "missing"}
	a := agent.New(agent.Config{InitialState: agent.State{Tools: stubRegistry{stub}}, LoopConfig: loop.Config{Model: model.Model{Provider: "fake", ID: "test"}, Stream: stream, MaxSteps: 3}})
	a.Steer(message.Message{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "steer"}}})
	if err := a.Prompt(context.Background(), "start", nil); err != nil {
		t.Fatal(err)
	}
	if err := a.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("stream calls = %d, want 2", calls)
	}
}

func TestAgentPublishesErrorAndSettledOnRunError(t *testing.T) {
	stream := func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		return streamEvents(provider.Error{Err: "provider failed"}), nil
	}
	a := agent.New(agent.Config{LoopConfig: loopConfig(stream)})

	var events []event.Event
	var settledStreaming bool
	a.Subscribe(func(ev event.Event) {
		events = append(events, ev)
		if _, ok := ev.(event.Settled); ok {
			settledStreaming = a.State().IsStreaming
		}
	})
	if err := a.Prompt(context.Background(), "hi", nil); err != nil {
		t.Fatal(err)
	}
	if err := a.Wait(context.Background()); err == nil || err.Error() != "provider failed" {
		t.Fatalf("wait error = %v, want provider failed", err)
	}
	if len(events) < 2 {
		t.Fatalf("events = %+v", events)
	}
	errEvent, ok := events[len(events)-2].(event.Error)
	if !ok {
		t.Fatalf("penultimate event = %T, want event.Error", events[len(events)-2])
	}
	if errEvent.Err != "provider failed" {
		t.Fatalf("error event = %+v", errEvent)
	}
	settled, ok := events[len(events)-1].(event.Settled)
	if !ok {
		t.Fatalf("last event = %T, want event.Settled", events[len(events)-1])
	}
	if settled.Err != "provider failed" {
		t.Fatalf("settled event = %+v", settled)
	}
	if settledStreaming {
		t.Fatal("agent was still streaming when settled was published")
	}
}

func loopConfig(stream provider.StreamFunc) loop.Config {
	return loop.Config{Model: model.Model{Provider: "fake", ID: "test"}, Stream: stream}
}

func streamEvents(events ...provider.Event) <-chan provider.Event {
	ch := make(chan provider.Event, len(events))
	for _, ev := range events {
		ch <- ev
	}
	close(ch)
	return ch
}

type toolStub struct{ name string }

func (t toolStub) Name() string            { return t.name }
func (t toolStub) Description() string     { return "stub" }
func (t toolStub) Schema() json.RawMessage { return nil }
func (t toolStub) Execute(context.Context, json.RawMessage, func(tool.Progress)) (tool.Result, error) {
	return tool.Result{Content: []message.Content{message.Text{Text: "stub"}}}, nil
}

type stubRegistry struct{ t toolStub }

func (r stubRegistry) Get(name string) (tool.Tool, bool) { return r.t, name == r.t.Name() }
func (r stubRegistry) List() []tool.Tool                 { return []tool.Tool{r.t} }
func (r stubRegistry) Specs() []tool.Spec                { return []tool.Spec{{Name: r.t.Name()}} }

func waitFor(fn func() bool) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}
