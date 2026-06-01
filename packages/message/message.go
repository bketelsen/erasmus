// Package message defines Erasmus' canonical provider-independent message types.
package message

import (
	"encoding/json"
	"fmt"
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

// UnmarshalJSON decodes canonical message content into concrete content parts.
func (m *Message) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID      string            `json:"id,omitempty"`
		Role    Role              `json:"role"`
		Content []json.RawMessage `json:"content"`
		Time    time.Time         `json:"time,omitempty"`
		Meta    map[string]string `json:"meta,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	content, err := decodeContentList(raw.Content)
	if err != nil {
		return err
	}
	*m = Message{ID: raw.ID, Role: raw.Role, Content: content, Time: raw.Time, Meta: raw.Meta}
	return nil
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

type rawContent struct {
	Type      string            `json:"type,omitempty"`
	Text      string            `json:"text,omitempty"`
	MimeType  string            `json:"mime_type,omitempty"`
	Data      json.RawMessage   `json:"data,omitempty"`
	ID        string            `json:"id,omitempty"`
	Name      string            `json:"name,omitempty"`
	Arguments json.RawMessage   `json:"arguments,omitempty"`
	CallID    string            `json:"call_id,omitempty"`
	Content   []json.RawMessage `json:"content,omitempty"`
	IsError   bool              `json:"is_error,omitempty"`
	Summary   string            `json:"summary,omitempty"`
	Encrypted string            `json:"encrypted,omitempty"`
	Kind      string            `json:"kind,omitempty"`
}

func decodeContentList(raw []json.RawMessage) ([]Content, error) {
	out := make([]Content, 0, len(raw))
	for _, part := range raw {
		content, err := decodeContent(part)
		if err != nil {
			return nil, err
		}
		out = append(out, content)
	}
	return out, nil
}

func decodeContent(data json.RawMessage) (Content, error) {
	var raw rawContent
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	switch contentType(raw) {
	case "text":
		return Text{Text: raw.Text}, nil
	case "image":
		var b []byte
		if len(raw.Data) > 0 {
			if err := json.Unmarshal(raw.Data, &b); err != nil {
				return nil, err
			}
		}
		return Image{MimeType: raw.MimeType, Data: b}, nil
	case "tool_call":
		return ToolCall{ID: raw.ID, Name: raw.Name, Arguments: raw.Arguments}, nil
	case "tool_result":
		content, err := decodeContentList(raw.Content)
		if err != nil {
			return nil, err
		}
		return ToolResult{CallID: raw.CallID, Content: content, IsError: raw.IsError}, nil
	case "reasoning":
		return Reasoning{ID: raw.ID, Summary: raw.Summary, Encrypted: raw.Encrypted}, nil
	case "custom":
		return Custom{Kind: raw.Kind, Data: raw.Data}, nil
	default:
		return nil, fmt.Errorf("unknown message content type %q", raw.Type)
	}
}

func contentType(raw rawContent) string {
	if raw.Type != "" {
		return raw.Type
	}
	switch {
	case raw.CallID != "":
		return "tool_result"
	case raw.Name != "" || len(raw.Arguments) > 0:
		return "tool_call"
	case raw.MimeType != "":
		return "image"
	case raw.Summary != "" || raw.Encrypted != "":
		return "reasoning"
	case raw.Kind != "":
		return "custom"
	default:
		return "text"
	}
}
