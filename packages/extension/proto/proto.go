// Package proto defines the Erasmus extension JSON-line protocol frames.
package proto

import (
	"encoding/json"

	"erasmus/packages/event"
	"erasmus/packages/provider"
	"erasmus/packages/tool"
)

// Frame is a generic JSON-line protocol frame.
type Frame struct {
	Type string          `json:"type"`
	ID   string          `json:"id,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Hello is sent by an extension at startup.
type Hello struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// RegisterTool registers a tool provided by an extension.
type RegisterTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
}

// RegisterCommand registers a command provided by an extension.
type RegisterCommand struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Subscribe requests runtime events from the host.
type Subscribe struct {
	Events []string `json:"events,omitempty"`
}

// Event carries a runtime event from the host to an extension.
type Event struct {
	Type  string          `json:"type"`
	Event event.Event     `json:"-"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// SubscribeHooks requests runtime hook calls from the host.
type SubscribeHooks struct {
	Hooks []string `json:"hooks,omitempty"`
}

// HookCall carries a runtime hook request from the host to an extension.
type HookCall struct {
	ID      string           `json:"id"`
	Hook    string           `json:"hook"`
	Request provider.Request `json:"request,omitempty"`
}

// HookResult returns a runtime hook decision to the host.
type HookResult struct {
	ID      string            `json:"id"`
	Deny    bool              `json:"deny,omitempty"`
	Error   string            `json:"error,omitempty"`
	Request *provider.Request `json:"request,omitempty"`
}

// CommandCall requests extension command execution.
type CommandCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input,omitempty"`
}

// CommandResult returns extension command output.
type CommandResult struct {
	ID      string       `json:"id"`
	Actions []HostAction `json:"actions,omitempty"`
	Error   string       `json:"error,omitempty"`
}

// HostAction asks the host UI/runtime to do something.
type HostAction struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// ToolCall requests an extension tool execution.
type ToolCall struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// ToolResult returns an extension tool result.
type ToolResult struct {
	ID     string      `json:"id"`
	Result tool.Result `json:"result"`
	Error  string      `json:"error,omitempty"`
}

// EncodeFrame wraps v in a typed frame.
func EncodeFrame(typ, id string, v any) (Frame, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return Frame{}, err
	}
	return Frame{Type: typ, ID: id, Data: data}, nil
}

// DecodeData decodes frame data into out.
func DecodeData(f Frame, out any) error {
	return json.Unmarshal(f.Data, out)
}
