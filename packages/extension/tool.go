// Package extension hosts extension-provided tools and protocol adapters.
package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"erasmus/packages/extension/proto"
	"erasmus/packages/tool"
)

// Caller executes extension tool calls.
type Caller interface {
	CallTool(ctx context.Context, call proto.ToolCall) (proto.ToolResult, error)
}

// Tool wraps an extension-provided tool as a normal Erasmus tool.Tool.
type Tool struct {
	NameValue        string
	DescriptionValue string
	SchemaValue      json.RawMessage
	Caller           Caller
	seq              atomic.Int64
}

// NewTool creates an extension tool wrapper.
func NewTool(reg proto.RegisterTool, caller Caller) *Tool {
	return &Tool{NameValue: reg.Name, DescriptionValue: reg.Description, SchemaValue: reg.Schema, Caller: caller}
}

func (t *Tool) Name() string            { return t.NameValue }
func (t *Tool) Description() string     { return t.DescriptionValue }
func (t *Tool) Schema() json.RawMessage { return t.SchemaValue }

// Execute calls the extension and returns its result.
func (t *Tool) Execute(ctx context.Context, args json.RawMessage, progress func(tool.Progress)) (tool.Result, error) {
	if t.Caller == nil {
		return tool.Result{IsError: true}, fmt.Errorf("extension caller is nil")
	}
	id := fmt.Sprintf("%s-%d", t.NameValue, t.seq.Add(1))
	if progress != nil {
		progress(tool.Progress{Text: "calling extension tool " + t.NameValue})
	}
	res, err := t.Caller.CallTool(ctx, proto.ToolCall{ID: id, Name: t.NameValue, Args: args})
	if err != nil {
		return tool.Result{IsError: true}, err
	}
	if res.Error != "" {
		res.Result.IsError = true
		return res.Result, fmt.Errorf("%s%s", res.Error, formatDiagnosticsPath(callerLogPath(t.Caller)))
	}
	return res.Result, nil
}

type logPathProvider interface {
	LogPath() string
}

func callerLogPath(caller any) string {
	if withPath, ok := caller.(logPathProvider); ok {
		return withPath.LogPath()
	}
	return ""
}

var _ tool.Tool = (*Tool)(nil)
