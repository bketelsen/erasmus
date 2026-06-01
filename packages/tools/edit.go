package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"erasmus/packages/message"
	"erasmus/packages/sandbox"
	"erasmus/packages/tool"
)

// EditTool performs exact-match text replacements inside sandboxed files.
type EditTool struct {
	Policy sandbox.Policy
}

// NewEditTool creates an edit tool constrained by policy.
func NewEditTool(policy sandbox.Policy) EditTool { return EditTool{Policy: policy} }

func (EditTool) Name() string { return "edit" }

func (EditTool) Description() string {
	return "Edit a text file by replacing exact, unique text occurrences."
}

func (EditTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "Path to edit, relative to the workspace."},
    "old_text": {"type": "string", "description": "Exact text to replace. Must occur exactly once."},
    "new_text": {"type": "string", "description": "Replacement text."}
  },
  "required": ["path", "old_text", "new_text"],
  "additionalProperties": false
}`)
}

// Execute performs one exact replacement.
func (t EditTool) Execute(ctx context.Context, args json.RawMessage, progress func(tool.Progress)) (tool.Result, error) {
	var in struct {
		Path    string `json:"path"`
		OldText string `json:"old_text"`
		NewText string `json:"new_text"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return tool.Result{IsError: true}, err
	}
	if in.OldText == "" {
		return tool.Result{IsError: true}, fmt.Errorf("old_text is required")
	}
	path, err := t.Policy.Resolve(in.Path)
	if err != nil {
		return tool.Result{IsError: true}, err
	}
	if err := ctx.Err(); err != nil {
		return tool.Result{IsError: true}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return tool.Result{IsError: true}, err
	}
	text := string(data)
	count := strings.Count(text, in.OldText)
	if count != 1 {
		return tool.Result{IsError: true}, fmt.Errorf("old_text occurs %d times, want exactly 1", count)
	}
	if progress != nil {
		progress(tool.Progress{Text: "editing " + in.Path})
	}
	updated := strings.Replace(text, in.OldText, in.NewText, 1)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return tool.Result{IsError: true}, err
	}
	return tool.Result{
		Content: []message.Content{message.Text{Text: "edited " + in.Path}},
		Details: map[string]any{"path": in.Path, "replacements": 1},
	}, nil
}

var _ tool.Tool = EditTool{}
