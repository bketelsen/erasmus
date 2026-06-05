// Package compact prepares and runs transcript compaction.
package compact

import (
	"context"
	"strings"

	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/provider"
)

// defaultKeepTurns is the number of trailing turns kept verbatim when neither
// KeepTurns nor KeepTail is set.
const defaultKeepTurns = 3

// defaultSummaryInstruction guides the model to produce a compaction summary
// that remains suitable for rebuilding session context and continuing the
// conversation. It preserves the durable facts and discards transient noise.
const defaultSummaryInstruction = `You are compacting an ongoing engineering conversation. Summarize the earlier messages into a compact, factual briefing that will REPLACE that older history while the conversation continues.

Preserve:
- Decisions made and the reasoning behind them.
- Concrete file paths, identifiers, commands, and configuration that remain relevant.
- Open questions and unresolved problems.
- What was tried and rejected, and why, so it is not retried.

Discard:
- Verbose tool output and logs.
- Superseded intermediate states and obsolete attempts.

Write the summary as plain prose or terse bullet points. Do not address the user or ask questions; output only the summary.`

// Options controls compaction.
type Options struct {
	// Model is the model used for the summary request when Run is given a stream.
	Model model.Model
	// KeepTurns is how many complete trailing turns to keep verbatim. When > 0
	// it takes precedence over KeepTail. Defaults to 3 when neither is set.
	KeepTurns int
	// SummaryInstruction overrides the default summarization system instruction
	// when non-empty.
	SummaryInstruction string
	// KeepTail keeps the last N messages verbatim. Retained for back-compat; used
	// only when KeepTurns is not set.
	KeepTail int
	// CustomInstructions are appended to the produced summary when non-empty.
	CustomInstructions string
	MaxTokens          int
}

// Preparation contains the messages to summarize and keep.
type Preparation struct {
	Summarize []message.Message
	Keep      []message.Message
	Options   Options
}

// Result is the outcome of compaction.
type Result struct {
	Summary      string
	TokensBefore int
	Messages     []message.Message
	Details      any
}

// Prepare splits messages into summary and tail groups.
//
// A turn is a RoleUser message and everything after it up to (but not including)
// the next RoleUser message; boundaries are role transitions only. When
// opts.KeepTurns > 0 (or by default), the last KeepTurns complete turns are kept
// verbatim and the first user message is pinned into Keep so the original
// task-framing ask is never summarized away. When opts.KeepTurns is not set but
// opts.KeepTail > 0, the last KeepTail messages are kept (back-compat).
func Prepare(messages []message.Message, opts Options) (Preparation, error) {
	if len(messages) == 0 {
		return Preparation{Options: opts}, nil
	}

	// Back-compat: explicit KeepTail with KeepTurns unset keeps N messages.
	if opts.KeepTurns <= 0 && opts.KeepTail > 0 {
		return prepareByTail(messages, opts.KeepTail, opts), nil
	}

	keepTurns := opts.KeepTurns
	if keepTurns <= 0 {
		keepTurns = defaultKeepTurns
	}

	userIdx := userIndices(messages)
	if len(userIdx) == 0 {
		// No turn boundaries to find: fall back to keeping the trailing
		// KeepTurns messages (or all of them when fewer exist).
		return prepareByTail(messages, keepTurns, opts), nil
	}

	// Fewer user turns than requested: keep everything, summarize nothing. The
	// first user message is already inside the kept tail, so no pinning needed.
	if len(userIdx) <= keepTurns {
		return Preparation{
			Summarize: nil,
			Keep:      cloneMessages(messages),
			Options:   opts,
		}, nil
	}

	// cut is the index of the KeepTurns-th-from-last user message; everything
	// from there onward is a whole-turn tail.
	cut := userIdx[len(userIdx)-keepTurns]
	first := userIdx[0]

	tail := messages[cut:]

	// Pin the first user message when it precedes the kept tail. De-duplicate
	// when it already falls within the tail.
	if first < cut {
		keep := make([]message.Message, 0, len(tail)+1)
		keep = append(keep, messages[first])
		keep = append(keep, tail...)

		summarize := make([]message.Message, 0, cut)
		for i := 0; i < cut; i++ {
			if i == first {
				continue
			}
			summarize = append(summarize, messages[i])
		}
		return Preparation{
			Summarize: summarize,
			Keep:      keep,
			Options:   opts,
		}, nil
	}

	return Preparation{
		Summarize: cloneMessages(messages[:cut]),
		Keep:      cloneMessages(tail),
		Options:   opts,
	}, nil
}

// prepareByTail keeps the last keep messages verbatim and summarizes the rest.
func prepareByTail(messages []message.Message, keep int, opts Options) Preparation {
	if keep > len(messages) {
		keep = len(messages)
	}
	cut := len(messages) - keep
	return Preparation{
		Summarize: cloneMessages(messages[:cut]),
		Keep:      cloneMessages(messages[cut:]),
		Options:   opts,
	}
}

// userIndices returns the indices of RoleUser messages in order.
func userIndices(messages []message.Message) []int {
	var idx []int
	for i, m := range messages {
		if m.Role == message.RoleUser {
			idx = append(idx, i)
		}
	}
	return idx
}

func cloneMessages(messages []message.Message) []message.Message {
	if len(messages) == 0 {
		return nil
	}
	return append([]message.Message(nil), messages...)
}

// Run summarizes a preparation. When stream is non-nil it generates the summary
// by calling the model; on any failure (stream error, provider error event, or
// empty output) it falls back to a deterministic local summary. A nil stream
// always uses the local summary.
func Run(ctx context.Context, stream provider.StreamFunc, prep Preparation) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	summary := ""
	if stream != nil {
		summary = modelSummary(ctx, stream, prep)
	}
	if strings.TrimSpace(summary) == "" {
		summary = localSummary(prep.Summarize)
	}

	if strings.TrimSpace(prep.Options.CustomInstructions) != "" {
		summary += "\n\nInstructions: " + strings.TrimSpace(prep.Options.CustomInstructions)
	}
	messages := append([]message.Message{{Role: message.RoleSystem, Content: []message.Content{message.Text{Text: summary}}}}, prep.Keep...)
	return Result{Summary: summary, TokensBefore: estimateTokens(append(prep.Summarize, prep.Keep...)), Messages: messages}, nil
}

// modelSummary runs the summary request through the provider stream and returns
// the accumulated text, or "" on any failure so the caller can fall back.
func modelSummary(ctx context.Context, stream provider.StreamFunc, prep Preparation) string {
	instruction := defaultSummaryInstruction
	if strings.TrimSpace(prep.Options.SummaryInstruction) != "" {
		instruction = prep.Options.SummaryInstruction
	}

	req := provider.Request{
		Model:     prep.Options.Model,
		System:    instruction,
		Messages:  prep.Summarize,
		MaxTokens: prep.Options.MaxTokens,
	}

	events, err := stream(ctx, req)
	if err != nil {
		return ""
	}

	var builder strings.Builder
	for {
		select {
		case <-ctx.Done():
			// The provider terminates its stream on context cancellation, so we
			// intentionally do not drain events here (real providers use
			// context-bound HTTP; the fake selects on ctx.Done()).
			return ""
		case ev, ok := <-events:
			if !ok {
				// A clean channel close with accumulated text is success: do not
				// "fix" a partial-then-clean-close into a fallback.
				return builder.String()
			}
			switch e := ev.(type) {
			case provider.TextDelta:
				builder.WriteString(e.Text)
			case provider.Error:
				return ""
			}
		}
	}
}

func localSummary(messages []message.Message) string {
	if len(messages) == 0 {
		return "No earlier conversation to summarize."
	}
	var parts []string
	for _, msg := range messages {
		var text []string
		for _, c := range msg.Content {
			if t, ok := c.(message.Text); ok && strings.TrimSpace(t.Text) != "" {
				text = append(text, strings.TrimSpace(t.Text))
			}
		}
		if len(text) > 0 {
			parts = append(parts, string(msg.Role)+": "+strings.Join(text, " "))
		}
	}
	if len(parts) == 0 {
		return "Earlier conversation contained non-text content."
	}
	return "Summary of earlier conversation:\n" + strings.Join(parts, "\n")
}

func estimateTokens(messages []message.Message) int {
	words := 0
	for _, msg := range messages {
		for _, c := range msg.Content {
			if t, ok := c.(message.Text); ok {
				words += len(strings.Fields(t.Text))
			}
		}
	}
	return words
}
