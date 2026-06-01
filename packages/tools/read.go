// Package tools contains Erasmus built-in tools.
package tools

import (
	"context"
	"encoding/json"
	"os"

	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/sandbox"
	"github.com/bketelsen/erasmus/packages/tool"
)

// ReadTool reads UTF-8-ish text files from the sandbox.
type ReadTool struct {
	Policy sandbox.Policy
}

// NewReadTool creates a read tool constrained by policy.
func NewReadTool(policy sandbox.Policy) ReadTool {
	return ReadTool{Policy: policy}
}

func (ReadTool) Name() string { return "read" }

func (ReadTool) Description() string {
	return "Read a text file from the current workspace."
}

func (ReadTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "Path to the file to read, relative to the workspace."}
  },
  "required": ["path"],
  "additionalProperties": false
}`)
}

// Execute reads the requested file and returns its contents as text.
func (t ReadTool) Execute(ctx context.Context, args json.RawMessage, progress func(tool.Progress)) (tool.Result, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return tool.Result{IsError: true}, err
	}
	path, err := t.Policy.Resolve(in.Path)
	if err != nil {
		return tool.Result{IsError: true}, err
	}
	if progress != nil {
		progress(tool.Progress{Text: "reading " + in.Path})
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return tool.Result{IsError: true}, err
	}
	select {
	case <-ctx.Done():
		return tool.Result{IsError: true}, ctx.Err()
	default:
	}

	return tool.Result{
		Content: []message.Content{message.Text{Text: string(data)}},
		Details: map[string]any{
			"path":  in.Path,
			"bytes": len(data),
		},
	}, nil
}

var _ tool.Tool = ReadTool{}
