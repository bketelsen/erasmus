// Package proto defines the Erasmus extension JSON-line protocol frames.
package proto

import (
	"encoding/json"

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
