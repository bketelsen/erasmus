package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/sandbox"
	"github.com/bketelsen/erasmus/packages/tool"
)

// BashTool runs shell commands from the sandbox root.
type BashTool struct {
	Policy         sandbox.Policy
	DefaultTimeout time.Duration
	MaxOutputBytes int
}

// NewBashTool creates a bash tool constrained to policy.Root as working directory.
func NewBashTool(policy sandbox.Policy) BashTool {
	return BashTool{Policy: policy, DefaultTimeout: 30 * time.Second, MaxOutputBytes: 64 * 1024}
}

func (BashTool) Name() string { return "bash" }

func (BashTool) Description() string {
	return "Run a shell command in the current workspace and return combined stdout/stderr."
}

func (BashTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "command": {"type": "string", "description": "Shell command to run."},
    "timeout_ms": {"type": "integer", "description": "Optional timeout in milliseconds."}
  },
  "required": ["command"],
  "additionalProperties": false
}`)
}

// Execute runs the command through /bin/sh -c with cwd set to the sandbox root.
func (t BashTool) Execute(ctx context.Context, args json.RawMessage, progress func(tool.Progress)) (tool.Result, error) {
	var in struct {
		Command   string `json:"command"`
		TimeoutMS int    `json:"timeout_ms"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return tool.Result{IsError: true}, err
	}
	if in.Command == "" {
		return tool.Result{IsError: true}, fmt.Errorf("command is required")
	}
	timeout := t.DefaultTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if in.TimeoutMS > 0 {
		timeout = time.Duration(in.TimeoutMS) * time.Millisecond
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if progress != nil {
		progress(tool.Progress{Text: "running " + in.Command})
	}
	cmd := exec.CommandContext(cmdCtx, "/bin/sh", "-c", in.Command)
	cmd.Dir = t.Policy.Root
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	limit := t.MaxOutputBytes
	if limit <= 0 {
		limit = 64 * 1024
	}
	truncated := false
	if len(out) > limit {
		out = out[:limit] + "\n[output truncated]"
		truncated = true
	}
	isError := err != nil
	if cmdCtx.Err() == context.DeadlineExceeded {
		isError = true
		if out != "" {
			out += "\n"
		}
		out += "command timed out"
		err = cmdCtx.Err()
	}
	return tool.Result{
		Content: []message.Content{message.Text{Text: out}},
		IsError: isError,
		Details: map[string]any{"command": in.Command, "truncated": truncated},
	}, err
}

var _ tool.Tool = BashTool{}
