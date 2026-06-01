package rpc

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"erasmus/packages/auth"
	"erasmus/packages/harness"
	"erasmus/packages/model"
	"erasmus/packages/provider"
	"erasmus/packages/session/memory"
)

func TestServerPromptStateAndEvents(t *testing.T) {
	ctx := context.Background()
	sess := memory.New("rpc-test")
	h, err := harness.New(ctx, harness.Config{
		Session: sess,
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			ch := make(chan provider.Event, 3)
			ch <- provider.MessageStart{MessageID: "m1"}
			ch <- provider.TextDelta{Text: "hello from rpc"}
			ch <- provider.MessageEnd{StopReason: "end_turn"}
			close(ch)
			return ch, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	store := auth.NewMemoryStore()
	if err := store.Set(ctx, auth.Credential{Provider: "fake", APIKey: "secret"}); err != nil {
		t.Fatal(err)
	}
	in := strings.NewReader("{" + `"id":"1","method":"state"` + "}\n" +
		"{" + `"id":"2","method":"models"` + "}\n" +
		"{" + `"id":"3","method":"auth_status"` + "}\n" +
		"{" + `"id":"4","method":"session"` + "}\n" +
		"{" + `"id":"5","method":"prompt","params":{"text":"hello"}` + "}\n" +
		"{" + `"id":"6","method":"wait"` + "}\n" +
		"{" + `"id":"7","method":"session_context"` + "}\n")
	var out bytes.Buffer
	if err := (&Server{Harness: h, Catalog: model.DefaultCatalog(), Auth: store}).Serve(ctx, in, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{`"id":"1"`, `"id":"2"`, `"Fake Echo"`, `"id":"3"`, `"fake"`, `"id":"4"`, `"rpc-test"`, `"id":"5"`, `"status":"started"`, `"method":"event"`, `"type":"message_delta"`, "hello from rpc", `"id":"6"`, `"status":"settled"`, `"id":"7"`, `"messages"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %s:\n%s", want, got)
		}
	}
}
