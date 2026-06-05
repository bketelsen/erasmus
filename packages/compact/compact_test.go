package compact_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bketelsen/erasmus/packages/compact"
	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/provider"
	"github.com/bketelsen/erasmus/packages/provider/fake"
)

func text(s string) []message.Content {
	return []message.Content{message.Text{Text: s}}
}

func firstText(msg message.Message) string {
	for _, c := range msg.Content {
		if t, ok := c.(message.Text); ok {
			return t.Text
		}
	}
	return ""
}

func TestPrepareAndRun(t *testing.T) {
	messages := []message.Message{
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "one"}}},
		{Role: message.RoleAssistant, Content: []message.Content{message.Text{Text: "two"}}},
		{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "three"}}},
	}
	prep, err := compact.Prepare(messages, compact.Options{KeepTail: 1, CustomInstructions: "be brief"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prep.Summarize) != 2 || len(prep.Keep) != 1 {
		t.Fatalf("prep = %+v", prep)
	}
	res, err := compact.Run(context.Background(), nil, prep)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Summary, "one") || !strings.Contains(res.Summary, "be brief") {
		t.Fatalf("summary = %q", res.Summary)
	}
	if len(res.Messages) != 2 || res.Messages[0].Role != message.RoleSystem {
		t.Fatalf("messages = %+v", res.Messages)
	}
}

// multiTurnTranscript builds a 4-turn transcript with interleaved assistant and
// tool messages. User messages are at indices 0, 3, 5, 8.
func multiTurnTranscript() []message.Message {
	return []message.Message{
		{Role: message.RoleUser, Content: text("task")},    // 0  turn 1
		{Role: message.RoleAssistant, Content: text("a1")}, // 1
		{Role: message.RoleTool, Content: text("t1")},      // 2
		{Role: message.RoleUser, Content: text("q2")},      // 3  turn 2
		{Role: message.RoleAssistant, Content: text("a2")}, // 4
		{Role: message.RoleUser, Content: text("q3")},      // 5  turn 3
		{Role: message.RoleAssistant, Content: text("a3")}, // 6
		{Role: message.RoleTool, Content: text("t3")},      // 7
		{Role: message.RoleUser, Content: text("q4")},      // 8  turn 4
		{Role: message.RoleAssistant, Content: text("a4")}, // 9
	}
}

func TestPrepareKeepsLastThreeTurns(t *testing.T) {
	messages := multiTurnTranscript()
	prep, err := compact.Prepare(messages, compact.Options{KeepTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	// Keep = pinned first user (idx 0) + turns 2,3,4 (idx 3..9) = 1 + 7 = 8.
	if len(prep.Keep) != 8 {
		t.Fatalf("Keep len = %d, want 8: %+v", len(prep.Keep), prep.Keep)
	}
	// Summarize = idx 1,2 (inside turn 1, after the pinned first user message).
	if len(prep.Summarize) != 2 {
		t.Fatalf("Summarize len = %d, want 2: %+v", len(prep.Summarize), prep.Summarize)
	}
	// The cut to the trailing turns must land on a user boundary: the first
	// element of the kept *tail* (after the pinned message) is a user message.
	if prep.Keep[1].Role != message.RoleUser || firstText(prep.Keep[1]) != "q2" {
		t.Fatalf("kept tail does not start on user boundary q2: %+v", prep.Keep[1])
	}
	// No turn bisected: the kept tail contains whole turns 2,3,4.
	wantTail := []string{"task", "q2", "a2", "q3", "a3", "t3", "q4", "a4"}
	for i, want := range wantTail {
		if got := firstText(prep.Keep[i]); got != want {
			t.Fatalf("Keep[%d] = %q, want %q", i, got, want)
		}
	}
}

func TestPreparePinsFirstUserMessage(t *testing.T) {
	messages := multiTurnTranscript()
	prep, err := compact.Prepare(messages, compact.Options{KeepTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(prep.Keep) == 0 {
		t.Fatal("Keep is empty")
	}
	// The first kept element is the pinned first user message.
	if prep.Keep[0].Role != message.RoleUser || firstText(prep.Keep[0]) != "task" {
		t.Fatalf("first kept element is not the pinned first user message: %+v", prep.Keep[0])
	}
	// The pinned message is older than the kept tail and must not be in Summarize.
	for _, m := range prep.Summarize {
		if firstText(m) == "task" {
			t.Fatalf("pinned first user message leaked into Summarize: %+v", prep.Summarize)
		}
	}
}

func TestPrepareDedupFirstUserWithinTail(t *testing.T) {
	// Two user turns, KeepTurns=3 covers everything; first user is in the tail.
	messages := []message.Message{
		{Role: message.RoleUser, Content: text("task")},
		{Role: message.RoleAssistant, Content: text("a1")},
		{Role: message.RoleUser, Content: text("q2")},
		{Role: message.RoleAssistant, Content: text("a2")},
	}
	prep, err := compact.Prepare(messages, compact.Options{KeepTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	// Fewer user turns than KeepTurns -> keep everything, nothing to summarize.
	if len(prep.Summarize) != 0 {
		t.Fatalf("Summarize len = %d, want 0: %+v", len(prep.Summarize), prep.Summarize)
	}
	if len(prep.Keep) != 4 {
		t.Fatalf("Keep len = %d, want 4: %+v", len(prep.Keep), prep.Keep)
	}
	// The first user message must appear exactly once (no duplication).
	count := 0
	for _, m := range prep.Keep {
		if m.Role == message.RoleUser && firstText(m) == "task" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("first user message appears %d times, want 1: %+v", count, prep.Keep)
	}
}

func TestPrepareBackCompatKeepTail(t *testing.T) {
	messages := []message.Message{
		{Role: message.RoleUser, Content: text("u1")},
		{Role: message.RoleAssistant, Content: text("a1")},
		{Role: message.RoleUser, Content: text("u2")},
		{Role: message.RoleAssistant, Content: text("a2")},
		{Role: message.RoleUser, Content: text("u3")},
	}
	prep, err := compact.Prepare(messages, compact.Options{KeepTail: 2})
	if err != nil {
		t.Fatal(err)
	}
	// Back-compat: keep last 2 messages, no pinning.
	if len(prep.Keep) != 2 {
		t.Fatalf("Keep len = %d, want 2: %+v", len(prep.Keep), prep.Keep)
	}
	if len(prep.Summarize) != 3 {
		t.Fatalf("Summarize len = %d, want 3: %+v", len(prep.Summarize), prep.Summarize)
	}
	if firstText(prep.Keep[0]) != "a2" || firstText(prep.Keep[1]) != "u3" {
		t.Fatalf("kept tail = %+v", prep.Keep)
	}
}

func TestPrepareEmpty(t *testing.T) {
	prep, err := compact.Prepare(nil, compact.Options{KeepTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(prep.Summarize) != 0 || len(prep.Keep) != 0 {
		t.Fatalf("expected empty preparation, got %+v", prep)
	}
}

func TestPrepareNoUserMessages(t *testing.T) {
	// No user messages at all: fall back to keeping the trailing KeepTurns
	// messages (or all if fewer). With KeepTurns=2 and 3 messages, keep last 2.
	messages := []message.Message{
		{Role: message.RoleSystem, Content: text("s")},
		{Role: message.RoleAssistant, Content: text("a1")},
		{Role: message.RoleAssistant, Content: text("a2")},
	}
	prep, err := compact.Prepare(messages, compact.Options{KeepTurns: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(prep.Keep) != 2 {
		t.Fatalf("Keep len = %d, want 2: %+v", len(prep.Keep), prep.Keep)
	}
	if len(prep.Summarize) != 1 {
		t.Fatalf("Summarize len = %d, want 1: %+v", len(prep.Summarize), prep.Summarize)
	}
	if firstText(prep.Keep[0]) != "a1" || firstText(prep.Keep[1]) != "a2" {
		t.Fatalf("kept tail = %+v", prep.Keep)
	}
}

func TestPrepareSingleTurn(t *testing.T) {
	// A single turn: one user message followed by assistant/tool messages.
	messages := []message.Message{
		{Role: message.RoleUser, Content: text("only")},
		{Role: message.RoleAssistant, Content: text("a1")},
		{Role: message.RoleTool, Content: text("t1")},
	}
	prep, err := compact.Prepare(messages, compact.Options{KeepTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	// Fewer turns than KeepTurns -> keep all, summarize nothing.
	if len(prep.Summarize) != 0 {
		t.Fatalf("Summarize len = %d, want 0: %+v", len(prep.Summarize), prep.Summarize)
	}
	if len(prep.Keep) != 3 {
		t.Fatalf("Keep len = %d, want 3: %+v", len(prep.Keep), prep.Keep)
	}
}

func TestRunModelDrivenSuccess(t *testing.T) {
	messages := multiTurnTranscript()
	mdl := model.Model{ID: "test-model", Provider: "fake"}
	prep, err := compact.Prepare(messages, compact.Options{KeepTurns: 3, Model: mdl})
	if err != nil {
		t.Fatal(err)
	}
	client := &fake.Client{
		Script: []provider.Event{
			provider.MessageStart{MessageID: "m1"},
			provider.TextDelta{Text: "Decisions: chose X. "},
			provider.TextDelta{Text: "Files: a.go. Open: none."},
			provider.MessageEnd{StopReason: "stop"},
		},
	}
	res, err := compact.Run(context.Background(), client.StreamFunc(), prep)
	if err != nil {
		t.Fatal(err)
	}
	wantSummary := "Decisions: chose X. Files: a.go. Open: none."
	if res.Summary != wantSummary {
		t.Fatalf("summary = %q, want %q (must be streamed text, not localSummary)", res.Summary, wantSummary)
	}
	// The system message in the result carries the model summary.
	if len(res.Messages) == 0 || res.Messages[0].Role != message.RoleSystem {
		t.Fatalf("expected leading system message, got %+v", res.Messages)
	}
	if firstText(res.Messages[0]) != wantSummary {
		t.Fatalf("system message = %q, want %q", firstText(res.Messages[0]), wantSummary)
	}
	// Assert the request sent to the provider.
	if len(client.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(client.Requests))
	}
	req := client.Requests[0]
	if strings.TrimSpace(req.System) == "" {
		t.Fatalf("request System (instruction) was empty")
	}
	if req.Model.ID != "test-model" {
		t.Fatalf("request Model = %+v, want test-model", req.Model)
	}
	if len(req.Tools) != 0 {
		t.Fatalf("request carried Tools, want none: %+v", req.Tools)
	}
	if len(req.Messages) != len(prep.Summarize) {
		t.Fatalf("request Messages len = %d, want %d", len(req.Messages), len(prep.Summarize))
	}
	for i := range prep.Summarize {
		if firstText(req.Messages[i]) != firstText(prep.Summarize[i]) {
			t.Fatalf("request Messages[%d] = %q, want %q", i, firstText(req.Messages[i]), firstText(prep.Summarize[i]))
		}
	}
}

func TestRunModelDrivenForwardsMaxTokens(t *testing.T) {
	messages := multiTurnTranscript()
	mdl := model.Model{ID: "test-model", Provider: "fake"}
	prep, err := compact.Prepare(messages, compact.Options{KeepTurns: 3, Model: mdl, MaxTokens: 512})
	if err != nil {
		t.Fatal(err)
	}
	client := &fake.Client{
		Script: []provider.Event{
			provider.TextDelta{Text: "ok"},
			provider.MessageEnd{},
		},
	}
	if _, err := compact.Run(context.Background(), client.StreamFunc(), prep); err != nil {
		t.Fatal(err)
	}
	if len(client.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(client.Requests))
	}
	if client.Requests[0].MaxTokens != 512 {
		t.Fatalf("request MaxTokens = %d, want 512 (Options.MaxTokens must be forwarded)", client.Requests[0].MaxTokens)
	}
}

func TestRunModelDrivenCustomInstructionAppended(t *testing.T) {
	messages := multiTurnTranscript()
	prep, err := compact.Prepare(messages, compact.Options{KeepTurns: 3, CustomInstructions: "stay terse"})
	if err != nil {
		t.Fatal(err)
	}
	client := &fake.Client{
		Script: []provider.Event{
			provider.TextDelta{Text: "model summary"},
			provider.MessageEnd{},
		},
	}
	res, err := compact.Run(context.Background(), client.StreamFunc(), prep)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Summary, "model summary") {
		t.Fatalf("summary missing model text: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "stay terse") {
		t.Fatalf("summary missing custom instructions: %q", res.Summary)
	}
}

func TestRunSummaryInstructionOverridesSystem(t *testing.T) {
	messages := multiTurnTranscript()
	prep, err := compact.Prepare(messages, compact.Options{KeepTurns: 3, SummaryInstruction: "CUSTOM-SYS"})
	if err != nil {
		t.Fatal(err)
	}
	client := &fake.Client{
		Script: []provider.Event{
			provider.TextDelta{Text: "ok"},
			provider.MessageEnd{},
		},
	}
	if _, err := compact.Run(context.Background(), client.StreamFunc(), prep); err != nil {
		t.Fatal(err)
	}
	if len(client.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(client.Requests))
	}
	if client.Requests[0].System != "CUSTOM-SYS" {
		t.Fatalf("request System = %q, want CUSTOM-SYS", client.Requests[0].System)
	}
}

func TestRunFallsBackOnProviderError(t *testing.T) {
	messages := multiTurnTranscript()
	prep, err := compact.Prepare(messages, compact.Options{KeepTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	client := &fake.Client{
		Script: []provider.Event{
			provider.MessageStart{MessageID: "m1"},
			provider.TextDelta{Text: "partial that should be discarded"},
			provider.Error{Err: "boom"},
		},
	}
	res, err := compact.Run(context.Background(), client.StreamFunc(), prep)
	if err != nil {
		t.Fatalf("Run should not error on provider failure (it falls back): %v", err)
	}
	// Fallback rung: deterministic localSummary of prep.Summarize.
	wantPrefix := "Summary of earlier conversation:"
	if !strings.HasPrefix(res.Summary, wantPrefix) {
		t.Fatalf("summary = %q, want localSummary fallback starting %q", res.Summary, wantPrefix)
	}
	if strings.Contains(res.Summary, "partial that should be discarded") {
		t.Fatalf("partial model text leaked into fallback summary: %q", res.Summary)
	}
}

type errStreamClient struct {
	requests []provider.Request
}

func (c *errStreamClient) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	c.requests = append(c.requests, req)
	return nil, context.DeadlineExceeded
}

func TestRunFallsBackWhenStreamReturnsError(t *testing.T) {
	messages := multiTurnTranscript()
	prep, err := compact.Prepare(messages, compact.Options{KeepTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	client := &errStreamClient{}
	res, err := compact.Run(context.Background(), client.Stream, prep)
	if err != nil {
		t.Fatalf("Run should fall back, not error: %v", err)
	}
	if !strings.HasPrefix(res.Summary, "Summary of earlier conversation:") {
		t.Fatalf("expected localSummary fallback, got %q", res.Summary)
	}
}

func TestRunFallsBackOnEmptyModelSummary(t *testing.T) {
	messages := multiTurnTranscript()
	prep, err := compact.Prepare(messages, compact.Options{KeepTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	client := &fake.Client{
		Script: []provider.Event{
			provider.MessageStart{MessageID: "m1"},
			provider.TextDelta{Text: "   \n  "},
			provider.MessageEnd{},
		},
	}
	res, err := compact.Run(context.Background(), client.StreamFunc(), prep)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Summary, "Summary of earlier conversation:") {
		t.Fatalf("expected localSummary fallback on whitespace-only model output, got %q", res.Summary)
	}
}

func TestRunNilStreamUsesLocalSummary(t *testing.T) {
	messages := multiTurnTranscript()
	prep, err := compact.Prepare(messages, compact.Options{KeepTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	res, err := compact.Run(context.Background(), nil, prep)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Summary, "Summary of earlier conversation:") {
		t.Fatalf("nil stream should use localSummary, got %q", res.Summary)
	}
}

func TestRunCanceledContext(t *testing.T) {
	prep, err := compact.Prepare(multiTurnTranscript(), compact.Options{KeepTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := compact.Run(ctx, nil, prep); err == nil {
		t.Fatal("expected error from canceled context")
	}
}

func TestRunMethodModelOnSuccess(t *testing.T) {
	prep, err := compact.Prepare(multiTurnTranscript(), compact.Options{KeepTurns: 3, Model: model.Model{ID: "test-model", Provider: "fake"}})
	if err != nil {
		t.Fatal(err)
	}
	client := &fake.Client{
		Script: []provider.Event{
			provider.TextDelta{Text: "model-generated summary"},
			provider.MessageEnd{StopReason: "stop"},
		},
	}
	res, err := compact.Run(context.Background(), client.StreamFunc(), prep)
	if err != nil {
		t.Fatal(err)
	}
	if res.Method != compact.SummaryModel {
		t.Fatalf("Method = %q, want %q", res.Method, compact.SummaryModel)
	}
}

func TestRunMethodFallbackOnNilStream(t *testing.T) {
	prep, err := compact.Prepare(multiTurnTranscript(), compact.Options{KeepTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	res, err := compact.Run(context.Background(), nil, prep)
	if err != nil {
		t.Fatal(err)
	}
	if res.Method != compact.SummaryFallback {
		t.Fatalf("Method = %q, want %q", res.Method, compact.SummaryFallback)
	}
}

func TestRunMethodFallbackOnStreamConstructError(t *testing.T) {
	prep, err := compact.Prepare(multiTurnTranscript(), compact.Options{KeepTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	// errStreamClient.Stream returns (nil, err): a stream-construct failure.
	res, err := compact.Run(context.Background(), (&errStreamClient{}).Stream, prep)
	if err != nil {
		t.Fatalf("Run should fall back, not error: %v", err)
	}
	if res.Method != compact.SummaryFallback {
		t.Fatalf("Method = %q, want %q", res.Method, compact.SummaryFallback)
	}
}

func TestRunMethodFallbackOnProviderError(t *testing.T) {
	prep, err := compact.Prepare(multiTurnTranscript(), compact.Options{KeepTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	client := &fake.Client{
		Script: []provider.Event{
			provider.TextDelta{Text: "partial that should be discarded"},
			provider.Error{Err: "boom"},
		},
	}
	res, err := compact.Run(context.Background(), client.StreamFunc(), prep)
	if err != nil {
		t.Fatal(err)
	}
	if res.Method != compact.SummaryFallback {
		t.Fatalf("Method = %q, want %q", res.Method, compact.SummaryFallback)
	}
}

func TestRunMethodFallbackOnEmptyModelOutput(t *testing.T) {
	prep, err := compact.Prepare(multiTurnTranscript(), compact.Options{KeepTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	// Only a MessageEnd, no TextDelta: the model produced no usable output.
	client := &fake.Client{
		Script: []provider.Event{
			provider.MessageEnd{StopReason: "stop"},
		},
	}
	res, err := compact.Run(context.Background(), client.StreamFunc(), prep)
	if err != nil {
		t.Fatal(err)
	}
	if res.Method != compact.SummaryFallback {
		t.Fatalf("Method = %q, want %q", res.Method, compact.SummaryFallback)
	}
}
