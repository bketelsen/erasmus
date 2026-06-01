package fake_test

import (
	"context"
	"testing"

	"erasmus/packages/model"
	"erasmus/packages/provider"
	"erasmus/packages/provider/fake"
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
