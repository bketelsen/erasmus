// Package provider defines the boundary between Erasmus runtime code and model providers.
package provider

import (
	"context"
	"encoding/json"

	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/tool"
)

// Client streams normalized provider events for a request.
type Client interface {
	Name() string
	Stream(ctx context.Context, req Request) (<-chan Event, error)
}

// StreamFunc adapts either a provider client or test fake into the loop.
type StreamFunc func(context.Context, Request) (<-chan Event, error)

// StreamOptions carries provider-specific streaming options.
type StreamOptions struct {
	Raw map[string]any `json:"raw,omitempty"`
}

// Request is the provider-facing prompt request built by the loop.
type Request struct {
	Model     model.Model       `json:"model"`
	System    string            `json:"system,omitempty"`
	Messages  []message.Message `json:"messages,omitempty"`
	Tools     []tool.Spec       `json:"tools,omitempty"`
	Reasoning string            `json:"reasoning,omitempty"`
	MaxTokens int               `json:"max_tokens,omitempty"`
	SessionID string            `json:"session_id,omitempty"`
	Options   StreamOptions     `json:"options,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
}

// Event is implemented by normalized provider stream events.
type Event interface {
	ProviderEventType() string
}

// MessageStart begins a provider assistant message.
type MessageStart struct {
	MessageID string `json:"message_id,omitempty"`
}

func (MessageStart) ProviderEventType() string { return "message_start" }

// TextDelta appends text to the active assistant message.
type TextDelta struct {
	Text string `json:"text"`
}

func (TextDelta) ProviderEventType() string { return "text_delta" }

// ToolCall emits a complete tool call request.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

func (ToolCall) ProviderEventType() string { return "tool_call" }

// Usage reports token usage.
type Usage struct {
	Usage model.Usage `json:"usage"`
}

func (Usage) ProviderEventType() string { return "usage" }

// MessageEnd ends the active assistant message.
type MessageEnd struct {
	StopReason string `json:"stop_reason,omitempty"`
}

func (MessageEnd) ProviderEventType() string { return "message_end" }

// Error reports a provider-side failure.
type Error struct {
	Err string `json:"error"`
}

func (Error) ProviderEventType() string { return "error" }
