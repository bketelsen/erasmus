// Package sdk provides small helpers for authoring Erasmus extension subprocesses.
package sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"erasmus/packages/extension/proto"
	"erasmus/packages/message"
	"erasmus/packages/tool"
)

// ToolResult is the canonical extension tool result shape.
type ToolResult = tool.Result

// ToolHandler executes an extension tool call.
type ToolHandler func(context.Context, json.RawMessage) (ToolResult, error)

// CommandHandler executes an extension command call.
type CommandHandler func(context.Context, json.RawMessage) ([]proto.HostAction, error)

// EventHandler handles runtime events forwarded by the host.
type EventHandler func(context.Context, proto.Event) ([]proto.HostAction, error)

// Tool describes a tool exposed by an extension subprocess.
type Tool struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Handler     ToolHandler
}

// Command describes a command exposed by an extension subprocess.
type Command struct {
	Name        string
	Description string
	Handler     CommandHandler
}

// Extension describes the startup registrations and handlers for one extension.
type Extension struct {
	Name     string
	Version  string
	Events   []string
	OnEvent  EventHandler
	Tools    []Tool
	Commands []Command
}

// Run starts an extension loop over stdin/stdout.
func Run(ctx context.Context, ext Extension) error {
	return RunWithIO(ctx, ext, os.Stdin, os.Stdout)
}

// RunWithIO starts an extension loop over caller-provided streams.
func RunWithIO(ctx context.Context, ext Extension, in io.Reader, out io.Writer) error {
	r := runner{ctx: ctx, ext: ext, in: in, out: out}
	if err := r.writeStartup(); err != nil {
		return err
	}
	return r.readLoop()
}

// TextResult returns a successful tool result with one text content part.
func TextResult(text string) ToolResult {
	return ToolResult{Content: []message.Content{message.Text{Text: text}}}
}

// ErrorResult returns a failed tool result with one text content part.
func ErrorResult(text string) ToolResult {
	return ToolResult{Content: []message.Content{message.Text{Text: text}}, IsError: true}
}

// PrintAction asks the host to print text.
func PrintAction(text string) proto.HostAction {
	data, _ := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: text})
	return proto.HostAction{Type: "print", Data: data}
}

// SetActiveToolsAction asks the host to replace the active tool selection.
func SetActiveToolsAction(names ...string) proto.HostAction {
	data, _ := json.Marshal(struct {
		Names []string `json:"names"`
	}{Names: append([]string(nil), names...)})
	return proto.HostAction{Type: "set_active_tools", Data: data}
}

// SavePointAction asks the host to persist a checkpoint marker.
func SavePointAction(label string, value any) proto.HostAction {
	data, _ := json.Marshal(struct {
		Label string `json:"label"`
		Data  any    `json:"data,omitempty"`
	}{Label: label, Data: value})
	return proto.HostAction{Type: "save_point", Data: data}
}

type runner struct {
	ctx context.Context
	ext Extension
	in  io.Reader
	out io.Writer
	mu  sync.Mutex
}

func (r *runner) writeStartup() error {
	name := r.ext.Name
	if name == "" {
		name = "extension"
	}
	if err := r.write("hello", "", proto.Hello{Name: name, Version: r.ext.Version}); err != nil {
		return err
	}
	for _, t := range r.ext.Tools {
		if err := r.write("register_tool", "", proto.RegisterTool{Name: t.Name, Description: t.Description, Schema: t.Schema}); err != nil {
			return err
		}
	}
	for _, c := range r.ext.Commands {
		if err := r.write("register_command", "", proto.RegisterCommand{Name: c.Name, Description: c.Description}); err != nil {
			return err
		}
	}
	if len(r.ext.Events) > 0 {
		if err := r.write("subscribe", "", proto.Subscribe{Events: append([]string(nil), r.ext.Events...)}); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) readLoop() error {
	scanner := bufio.NewScanner(r.in)
	for scanner.Scan() {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		var frame proto.Frame
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			return err
		}
		if err := r.handle(frame); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return r.ctx.Err()
}

func (r *runner) handle(frame proto.Frame) error {
	switch frame.Type {
	case "tool_call":
		var call proto.ToolCall
		if err := proto.DecodeData(frame, &call); err != nil {
			return err
		}
		if call.ID == "" {
			call.ID = frame.ID
		}
		return r.handleToolCall(call)
	case "command_call":
		var call proto.CommandCall
		if err := proto.DecodeData(frame, &call); err != nil {
			return err
		}
		if call.ID == "" {
			call.ID = frame.ID
		}
		return r.handleCommandCall(call)
	case "event":
		var ev proto.Event
		if err := proto.DecodeData(frame, &ev); err != nil {
			return err
		}
		if ev.Type == "" {
			ev.Type = frame.ID
		}
		return r.handleEvent(ev)
	default:
		return nil
	}
}

func (r *runner) handleToolCall(call proto.ToolCall) error {
	handler := r.toolHandler(call.Name)
	if handler == nil {
		return r.write("tool_result", call.ID, proto.ToolResult{ID: call.ID, Error: fmt.Sprintf("unknown tool %q", call.Name), Result: ErrorResult("unknown tool " + call.Name)})
	}
	result, err := handler(r.ctx, call.Args)
	if err != nil {
		return r.write("tool_result", call.ID, proto.ToolResult{ID: call.ID, Error: err.Error(), Result: ErrorResult(err.Error())})
	}
	return r.write("tool_result", call.ID, proto.ToolResult{ID: call.ID, Result: result})
}

func (r *runner) handleCommandCall(call proto.CommandCall) error {
	handler := r.commandHandler(call.Name)
	if handler == nil {
		return r.write("command_result", call.ID, proto.CommandResult{ID: call.ID, Error: fmt.Sprintf("unknown command %q", call.Name)})
	}
	actions, err := handler(r.ctx, call.Input)
	if err != nil {
		return r.write("command_result", call.ID, proto.CommandResult{ID: call.ID, Error: err.Error()})
	}
	return r.write("command_result", call.ID, proto.CommandResult{ID: call.ID, Actions: actions})
}

func (r *runner) handleEvent(ev proto.Event) error {
	if r.ext.OnEvent == nil {
		return nil
	}
	actions, err := r.ext.OnEvent(r.ctx, ev)
	if err != nil {
		return err
	}
	for _, action := range actions {
		if err := r.write("host_action", "", action); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) toolHandler(name string) ToolHandler {
	for _, t := range r.ext.Tools {
		if t.Name == name {
			return t.Handler
		}
	}
	return nil
}

func (r *runner) commandHandler(name string) CommandHandler {
	for _, c := range r.ext.Commands {
		if c.Name == name {
			return c.Handler
		}
	}
	return nil
}

func (r *runner) write(typ, id string, v any) error {
	frame, err := proto.EncodeFrame(typ, id, v)
	if err != nil {
		return err
	}
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err = r.out.Write(append(data, '\n'))
	return err
}
