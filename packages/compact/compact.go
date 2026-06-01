// Package compact prepares and runs transcript compaction.
package compact

import (
	"context"
	"strings"

	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/provider"
)

// Options controls compaction.
type Options struct {
	KeepTail           int
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
func Prepare(messages []message.Message, opts Options) (Preparation, error) {
	keepTail := opts.KeepTail
	if keepTail <= 0 {
		keepTail = 4
	}
	if keepTail > len(messages) {
		keepTail = len(messages)
	}
	cut := len(messages) - keepTail
	return Preparation{
		Summarize: append([]message.Message(nil), messages[:cut]...),
		Keep:      append([]message.Message(nil), messages[cut:]...),
		Options:   opts,
	}, nil
}

// Run summarizes a preparation. If stream is nil, it uses a deterministic local summary.
func Run(ctx context.Context, stream provider.StreamFunc, prep Preparation) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	summary := localSummary(prep.Summarize)
	if strings.TrimSpace(prep.Options.CustomInstructions) != "" {
		summary += "\n\nInstructions: " + strings.TrimSpace(prep.Options.CustomInstructions)
	}
	messages := append([]message.Message{{Role: message.RoleSystem, Content: []message.Content{message.Text{Text: summary}}}}, prep.Keep...)
	return Result{Summary: summary, TokensBefore: estimateTokens(append(prep.Summarize, prep.Keep...)), Messages: messages}, nil
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
