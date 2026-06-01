package fake_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/provider"
	"github.com/bketelsen/erasmus/packages/provider/fake"
)

func TestClientStreamsScript(t *testing.T) {
	client := &fake.Client{Script: []provider.Event{
		provider.MessageStart{MessageID: "m1"},
		provider.TextDelta{Text: "hello"},
		provider.MessageEnd{StopReason: "end_turn"},
	}}

	ch, err := client.Stream(context.Background(), provider.Request{Model: model.Model{Provider: "fake", ID: "test"}})
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for ev := range ch {
		got = append(got, ev.ProviderEventType())
	}

	want := []string{"message_start", "text_delta", "message_end"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}

	if len(client.Requests) != 1 {
		t.Fatalf("recorded %d requests, want 1", len(client.Requests))
	}
}

func TestClientStreamsToolCallErrorAndUsageEvents(t *testing.T) {
	args := json.RawMessage(`{"path":"README.md"}`)
	client := &fake.Client{Script: []provider.Event{
		provider.ToolCall{ID: "call-1", Name: "read", Arguments: args},
		provider.Usage{Usage: model.Usage{InputTokens: 3, OutputTokens: 2}},
		provider.Error{Err: "provider failed"},
	}}

	ch, err := client.Stream(context.Background(), provider.Request{Model: model.Model{Provider: "fake", ID: "test"}})
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(ch)
	if len(events) != 3 {
		t.Fatalf("events = %#v, want 3 events", events)
	}
	call, ok := events[0].(provider.ToolCall)
	if !ok {
		t.Fatalf("event[0] = %T, want provider.ToolCall", events[0])
	}
	if call.ID != "call-1" || call.Name != "read" || string(call.Arguments) != string(args) {
		t.Fatalf("tool call = %#v, want call-1/read/%s", call, args)
	}
	usage, ok := events[1].(provider.Usage)
	if !ok {
		t.Fatalf("event[1] = %T, want provider.Usage", events[1])
	}
	if usage.Usage.InputTokens != 3 || usage.Usage.OutputTokens != 2 {
		t.Fatalf("usage = %+v, want input=3 output=2", usage.Usage)
	}
	providerErr, ok := events[2].(provider.Error)
	if !ok {
		t.Fatalf("event[2] = %T, want provider.Error", events[2])
	}
	if providerErr.Err != "provider failed" {
		t.Fatalf("provider error = %q, want provider failed", providerErr.Err)
	}
}

func TestClientStopsDelayedStreamOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &fake.Client{
		Delay: 50 * time.Millisecond,
		Script: []provider.Event{
			provider.TextDelta{Text: "too late"},
		},
	}

	ch, err := client.Stream(ctx, provider.Request{Model: model.Model{Provider: "fake", ID: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	select {
	case ev, ok := <-ch:
		if ok {
			t.Fatalf("received event after cancellation: %#v", ev)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("stream did not close after cancellation")
	}
}

func TestClientRecordsRequestsAcrossStreams(t *testing.T) {
	client := &fake.Client{}

	for _, id := range []string{"first", "second"} {
		ch, err := client.Stream(context.Background(), provider.Request{Model: model.Model{Provider: "fake", ID: id}})
		if err != nil {
			t.Fatal(err)
		}
		for range ch {
		}
	}

	if len(client.Requests) != 2 {
		t.Fatalf("recorded %d requests, want 2", len(client.Requests))
	}
	if got := client.Requests[0].Model.ID; got != "first" {
		t.Fatalf("first request model = %q, want first", got)
	}
	if got := client.Requests[1].Model.ID; got != "second" {
		t.Fatalf("second request model = %q, want second", got)
	}
}

func collectEvents(ch <-chan provider.Event) []provider.Event {
	var events []provider.Event
	for ev := range ch {
		events = append(events, ev)
	}
	return events
}
