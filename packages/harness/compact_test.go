package harness_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bketelsen/erasmus/packages/compact"
	"github.com/bketelsen/erasmus/packages/event"
	"github.com/bketelsen/erasmus/packages/harness"
	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/provider"
	"github.com/bketelsen/erasmus/packages/provider/fake"
	"github.com/bketelsen/erasmus/packages/session/memory"
)

func TestHarnessCompact(t *testing.T) {
	ctx := context.Background()
	sess := memory.New("test")
	for _, msg := range []message.Message{
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "one"}}},
		{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "two"}}},
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "three"}}},
	} {
		if _, err := sess.AppendMessage(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}
	h, err := harness.New(ctx, harness.Config{Session: sess, Stream: noopStream, Model: model.Model{Provider: "fake", ID: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	var got event.Event
	h.Subscribe(func(ev event.Event) { got = ev })
	res, err := h.Compact(ctx, compact.Options{KeepTail: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Summary, "one") {
		t.Fatalf("summary = %q", res.Summary)
	}
	if _, ok := got.(event.SessionCompact); !ok {
		t.Fatalf("event = %T, want SessionCompact", got)
	}
	st := h.State(ctx)
	if len(st.Agent.Messages) != 2 || st.Agent.Messages[0].Role != message.RoleSystem {
		t.Fatalf("messages = %+v", st.Agent.Messages)
	}
	built, err := sess.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Messages) != 2 || built.Messages[0].Role != message.RoleSystem {
		t.Fatalf("session messages = %+v", built.Messages)
	}
}

func TestHarnessBeforeCompactHookCanPatchOptions(t *testing.T) {
	ctx := context.Background()
	sess := memory.New("test")
	for _, msg := range []message.Message{
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "one"}}},
		{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "two"}}},
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "three"}}},
		{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "four"}}},
	} {
		if _, err := sess.AppendMessage(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}
	h, err := harness.New(ctx, harness.Config{
		Session: sess,
		Stream:  noopStream,
		Model:   model.Model{Provider: "fake", ID: "test"},
		Hooks: harness.Hooks{
			BeforeCompact: func(ctx context.Context, c harness.BeforeCompactContext) (harness.BeforeCompactResult, error) {
				if c.Options.KeepTail != 1 {
					t.Fatalf("keep tail = %d, want 1", c.Options.KeepTail)
				}
				opts := c.Options
				opts.KeepTail = 2
				opts.CustomInstructions = "preserve decisions"
				return harness.BeforeCompactResult{Options: &opts}, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Compact(ctx, compact.Options{KeepTail: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Summary, "Instructions: preserve decisions") {
		t.Fatalf("summary = %q", res.Summary)
	}
	st := h.State(ctx)
	if len(st.Agent.Messages) != 3 {
		t.Fatalf("messages len = %d, want 3", len(st.Agent.Messages))
	}
	if got := st.Agent.Messages[1].Content[0].(message.Text).Text; got != "three" {
		t.Fatalf("first kept message = %q, want three", got)
	}
}

func TestHarnessAfterCompactHookObservesResult(t *testing.T) {
	ctx := context.Background()
	sess := memory.New("test")
	for _, msg := range []message.Message{
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "one"}}},
		{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "two"}}},
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "three"}}},
	} {
		if _, err := sess.AppendMessage(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}
	var observed compact.Result
	h, err := harness.New(ctx, harness.Config{
		Session: sess,
		Stream:  noopStream,
		Model:   model.Model{Provider: "fake", ID: "test"},
		Hooks: harness.Hooks{
			AfterCompact: func(ctx context.Context, c harness.AfterCompactContext) error {
				observed = c.Result
				return nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Compact(ctx, compact.Options{KeepTail: 1})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Summary != res.Summary || observed.TokensBefore == 0 {
		t.Fatalf("observed = %+v, result = %+v", observed, res)
	}
}

// compactTestSession builds a memory session with a multi-turn transcript that
// has more than the default kept turns, so compaction has earlier messages to
// summarize (and thus issues a model request).
func compactTestSession(t *testing.T) *memory.Session {
	t.Helper()
	ctx := context.Background()
	sess := memory.New("compact-model")
	for _, msg := range []message.Message{
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "u1"}}},
		{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "a1"}}},
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "u2"}}},
		{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "a2"}}},
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "u3"}}},
		{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "a3"}}},
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "u4"}}},
		{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "a4"}}},
	} {
		if _, err := sess.AppendMessage(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}
	return sess
}

// TestHarnessCompactDefaultsModelAndPublishesModelMethod is the high-value
// integration check for both changes: Compact with empty Options defaults the
// summary model to the resolved runtime model (so the model path fires) and the
// published SessionCompact carries Method "model".
func TestHarnessCompactDefaultsModelAndPublishesModelMethod(t *testing.T) {
	ctx := context.Background()
	sess := compactTestSession(t)
	client := &fake.Client{
		Script: []provider.Event{
			provider.TextDelta{Text: "model-generated compaction summary"},
			provider.MessageEnd{StopReason: "stop"},
		},
	}
	h, err := harness.New(ctx, harness.Config{
		Session: sess,
		Stream:  client.StreamFunc(),
		Model:   model.Model{Provider: "fake", ID: "test-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got event.SessionCompact
	var sawCompact bool
	h.Subscribe(func(ev event.Event) {
		if sc, ok := ev.(event.SessionCompact); ok {
			got = sc
			sawCompact = true
		}
	})

	res, err := h.Compact(ctx, compact.Options{})
	if err != nil {
		t.Fatal(err)
	}

	// The harness defaulted opts.Model, so the model path fired (not the
	// zero-model fallback): the request carries the resolved model.
	if len(client.Requests) != 1 {
		t.Fatalf("recorded %d provider requests, want 1", len(client.Requests))
	}
	if got := client.Requests[0].Model.ID; got != "test-model" {
		t.Fatalf("request Model.ID = %q, want test-model (harness must default the model)", got)
	}
	// The model produced the summary, so Result.Method and the published event
	// both report the model rung.
	if res.Method != compact.SummaryModel {
		t.Fatalf("Result.Method = %q, want %q", res.Method, compact.SummaryModel)
	}
	if !strings.Contains(res.Summary, "model-generated compaction summary") {
		t.Fatalf("summary = %q, want model-generated text", res.Summary)
	}
	if !sawCompact {
		t.Fatal("no SessionCompact event published")
	}
	if got.Method != "model" {
		t.Fatalf("published SessionCompact.Method = %q, want model", got.Method)
	}
}

// TestHarnessCompactPreservesCallerModel verifies a caller-supplied model is not
// overwritten by the runtime-model default.
func TestHarnessCompactPreservesCallerModel(t *testing.T) {
	ctx := context.Background()
	sess := compactTestSession(t)
	client := &fake.Client{
		Script: []provider.Event{
			provider.TextDelta{Text: "summary"},
			provider.MessageEnd{StopReason: "stop"},
		},
	}
	h, err := harness.New(ctx, harness.Config{
		Session: sess,
		Stream:  client.StreamFunc(),
		Model:   model.Model{Provider: "fake", ID: "test-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Compact(ctx, compact.Options{Model: model.Model{ID: "override"}}); err != nil {
		t.Fatal(err)
	}
	if len(client.Requests) != 1 {
		t.Fatalf("recorded %d provider requests, want 1", len(client.Requests))
	}
	if got := client.Requests[0].Model.ID; got != "override" {
		t.Fatalf("request Model.ID = %q, want override (caller model must be preserved)", got)
	}
}

// TestHarnessCompactPublishesFallbackMethod verifies the published method is
// "fallback" when the model produces no usable output.
func TestHarnessCompactPublishesFallbackMethod(t *testing.T) {
	ctx := context.Background()
	sess := compactTestSession(t)
	// Only a MessageEnd, no TextDelta: empty model output -> fallback rung.
	client := &fake.Client{
		Script: []provider.Event{
			provider.MessageEnd{StopReason: "stop"},
		},
	}
	h, err := harness.New(ctx, harness.Config{
		Session: sess,
		Stream:  client.StreamFunc(),
		Model:   model.Model{Provider: "fake", ID: "test-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got event.SessionCompact
	h.Subscribe(func(ev event.Event) {
		if sc, ok := ev.(event.SessionCompact); ok {
			got = sc
		}
	})
	res, err := h.Compact(ctx, compact.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Method != compact.SummaryFallback {
		t.Fatalf("Result.Method = %q, want %q", res.Method, compact.SummaryFallback)
	}
	if got.Method != "fallback" {
		t.Fatalf("published SessionCompact.Method = %q, want fallback", got.Method)
	}
}
