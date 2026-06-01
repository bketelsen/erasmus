package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"erasmus/packages/message"
	"erasmus/packages/sandbox"
	"erasmus/packages/tool"
)

// WriteTool writes text files inside the sandbox.
type WriteTool struct {
	Policy sandbox.Policy
}

// NewWriteTool creates a write tool constrained by policy.
func NewWriteTool(policy sandbox.Policy) WriteTool { return WriteTool{Policy: policy} }

func (WriteTool) Name() string { return "write" }

func (WriteTool) Description() string {
	return "Write text content to a file in the current workspace."
}

func (WriteTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "Path to write, relative to the workspace."},
    "content": {"type": "string", "description": "Complete file content to write."}
  },
  "required": ["path", "content"],
  "additionalProperties": false
}`)
}

// Execute writes complete file content, creating parent directories inside the sandbox as needed.
func (t WriteTool) Execute(ctx context.Context, args json.RawMessage, progress func(tool.Progress)) (tool.Result, error) {
	var in struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return tool.Result{IsError: true}, err
	}
	path, err := t.Policy.Resolve(in.Path)
	if err != nil {
		return tool.Result{IsError: true}, err
	}
	if err := ctx.Err(); err != nil {
		return tool.Result{IsError: true}, err
	}
	if progress != nil {
		progress(tool.Progress{Text: "writing " + in.Path})
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return tool.Result{IsError: true}, err
	}
	if err := os.WriteFile(path, []byte(in.Content), 0o644); err != nil {
		return tool.Result{IsError: true}, err
	}
	return tool.Result{
		Content: []message.Content{message.Text{Text: "wrote " + in.Path}},
		Details: map[string]any{"path": in.Path, "bytes": len([]byte(in.Content))},
	}, nil
}

var _ tool.Tool = WriteTool{}
