// Package message defines Erasmus' canonical provider-independent message types.
package message

import (
	"encoding/json"
	"time"
)

// Role identifies who produced a message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
)

// Message is the canonical transcript unit used by sessions, loop, agent, and harness.
type Message struct {
	ID      string            `json:"id,omitempty"`
	Role    Role              `json:"role"`
	Content []Content         `json:"content"`
	Time    time.Time         `json:"time,omitempty"`
	Meta    map[string]string `json:"meta,omitempty"`
}

// Content is a typed message content part.
type Content interface {
	contentKind()
}

// Text is a plain text content part.
type Text struct {
	Text string `json:"text"`
}

func (Text) contentKind() {}

// Image is an inline image content part.
type Image struct {
	MimeType string `json:"mime_type"`
	Data     []byte `json:"data"`
}

func (Image) contentKind() {}

// ToolCall requests execution of a named tool.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

func (ToolCall) contentKind() {}

// ToolResult records the result of a tool call.
type ToolResult struct {
	CallID  string    `json:"call_id"`
	Content []Content `json:"content,omitempty"`
	IsError bool      `json:"is_error,omitempty"`
}

func (ToolResult) contentKind() {}

// Reasoning carries provider reasoning metadata when available.
type Reasoning struct {
	ID        string `json:"id,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Encrypted string `json:"encrypted,omitempty"`
}

func (Reasoning) contentKind() {}

// Custom is an escape hatch for future content kinds and extension-provided payloads.
type Custom struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data,omitempty"`
}

func (Custom) contentKind() {}
