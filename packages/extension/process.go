package extension

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"erasmus/packages/event"
	"erasmus/packages/extension/proto"
	"erasmus/packages/message"
	"erasmus/packages/tool"
)

// Process hosts one extension subprocess speaking JSON-line proto.Frame values.
type Process struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	frames  chan proto.Frame
	manager *Manager

	mu       sync.Mutex
	pendingT map[string]chan proto.ToolResult
	pendingC map[string]chan proto.CommandResult
	subs     map[string]bool
	logs     *ringLog
	writeMu  sync.Mutex
	done     chan error
}

// ProcessOptions configures an extension subprocess host.
type ProcessOptions struct {
	LogPath string
}

// StartProcess starts an extension subprocess and waits briefly for registrations.
func StartProcess(ctx context.Context, command string, args ...string) (*Process, error) {
	return StartProcessWithOptions(ctx, command, ProcessOptions{}, args...)
}

// StartProcessWithOptions starts an extension subprocess with host options.
func StartProcessWithOptions(ctx context.Context, command string, opts ProcessOptions, args ...string) (*Process, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	logs, err := newPersistentRingLog(80, opts.LogPath)
	if err != nil {
		return nil, err
	}
	p := &Process{cmd: cmd, stdin: stdin, frames: make(chan proto.Frame, 32), pendingT: map[string]chan proto.ToolResult{}, pendingC: map[string]chan proto.CommandResult{}, subs: map[string]bool{}, logs: logs, done: make(chan error, 1)}
	p.manager = NewManager(p)
	if err := cmd.Start(); err != nil {
		_ = p.logs.Close()
		return nil, err
	}
	go p.read(stdout)
	go p.readStderr(stderr)
	go func() {
		p.done <- cmd.Wait()
		close(p.done)
	}()
	if err := p.collectStartup(ctx, 200*time.Millisecond); err != nil {
		diag := p.Diagnostics()
		logPath := p.LogPath()
		_ = p.Close()
		return nil, fmt.Errorf("start extension %q: %w%s%s", command, err, formatDiagnostics(diag), formatDiagnosticsPath(logPath))
	}
	return p, nil
}

// Manager returns the subprocess-backed manager.
func (p *Process) Manager() *Manager { return p.manager }

// Diagnostics returns recent stderr and host-side protocol diagnostics.
func (p *Process) Diagnostics() []string {
	if p == nil || p.logs == nil {
		return nil
	}
	return p.logs.Lines()
}

// LogPath returns the persistent diagnostics log path, when configured.
func (p *Process) LogPath() string {
	if p == nil || p.logs == nil {
		return ""
	}
	return p.logs.Path()
}

// Close terminates the subprocess.
func (p *Process) Close() error {
	if p.stdin != nil {
		_ = p.stdin.Close()
	}
	select {
	case <-p.done:
	case <-time.After(200 * time.Millisecond):
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	select {
	case <-p.done:
	case <-time.After(time.Second):
	}
	if p.logs != nil {
		return p.logs.Close()
	}
	return nil
}

// PublishEvent forwards a runtime event to the subprocess when subscribed.
func (p *Process) PublishEvent(ctx context.Context, ev event.Event) error {
	if p == nil || ev == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	typ := ev.Type()
	p.mu.Lock()
	subscribed := p.subs["*"] || p.subs[typ]
	p.mu.Unlock()
	if !subscribed {
		return nil
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return p.write("event", typ, proto.Event{Type: typ, Data: data})
}

// CallTool implements Caller.
func (p *Process) CallTool(ctx context.Context, call proto.ToolCall) (proto.ToolResult, error) {
	ch := make(chan proto.ToolResult, 1)
	p.mu.Lock()
	p.pendingT[call.ID] = ch
	p.mu.Unlock()
	if err := p.write("tool_call", call.ID, call); err != nil {
		return proto.ToolResult{}, err
	}
	select {
	case res := <-ch:
		return res, nil
	case <-ctx.Done():
		return proto.ToolResult{}, ctx.Err()
	case err := <-p.done:
		return proto.ToolResult{}, fmt.Errorf("extension process exited: %v%s%s", err, formatDiagnostics(p.Diagnostics()), formatDiagnosticsPath(p.LogPath()))
	}
}

// CallCommand implements CommandCaller.
func (p *Process) CallCommand(ctx context.Context, call proto.CommandCall) (proto.CommandResult, error) {
	ch := make(chan proto.CommandResult, 1)
	p.mu.Lock()
	p.pendingC[call.ID] = ch
	p.mu.Unlock()
	if err := p.write("command_call", call.ID, call); err != nil {
		return proto.CommandResult{}, err
	}
	select {
	case res := <-ch:
		return res, nil
	case <-ctx.Done():
		return proto.CommandResult{}, ctx.Err()
	case err := <-p.done:
		return proto.CommandResult{}, fmt.Errorf("extension process exited: %v%s%s", err, formatDiagnostics(p.Diagnostics()), formatDiagnosticsPath(p.LogPath()))
	}
}

func (p *Process) read(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Bytes()
		var frame proto.Frame
		if err := json.Unmarshal(line, &frame); err == nil {
			p.frames <- frame
		} else if p.logs != nil {
			p.logs.AddSource("stdout", "stdout: invalid JSON frame: "+err.Error()+": "+string(line))
		}
	}
	if err := scanner.Err(); err != nil && p.logs != nil {
		p.logs.AddSource("stdout", "stdout: read error: "+err.Error())
	}
	close(p.frames)
}

func (p *Process) readStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if p.logs != nil {
			p.logs.AddSource("stderr", "stderr: "+scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil && p.logs != nil {
		p.logs.AddSource("stderr", "stderr: read error: "+err.Error())
	}
}

func (p *Process) collectStartup(ctx context.Context, quiet time.Duration) error {
	quietTimer := time.NewTimer(time.Hour)
	if !quietTimer.Stop() {
		<-quietTimer.C
	}
	defer quietTimer.Stop()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	seenFrame := false
	for {
		select {
		case frame, ok := <-p.frames:
			if !ok {
				select {
				case err := <-p.done:
					if err != nil {
						return err
					}
				default:
				}
				return nil
			}
			seenFrame = true
			if err := p.handle(frame); err != nil {
				return err
			}
			if !quietTimer.Stop() {
				select {
				case <-quietTimer.C:
				default:
				}
			}
			quietTimer.Reset(quiet)
		case <-quietTimer.C:
			go p.dispatch()
			return nil
		case <-deadline.C:
			go p.dispatch()
			if seenFrame {
				return nil
			}
			return fmt.Errorf("extension produced no startup frames before deadline")
		case <-ctx.Done():
			return ctx.Err()
		case err := <-p.done:
			if err == nil && !seenFrame {
				return fmt.Errorf("extension exited before producing startup frames")
			}
			return err
		}
	}
}

func (p *Process) dispatch() {
	for frame := range p.frames {
		_ = p.handle(frame)
	}
}

func (p *Process) handle(frame proto.Frame) error {
	switch frame.Type {
	case "hello":
		return nil
	case "register_tool":
		var reg proto.RegisterTool
		if err := proto.DecodeData(frame, &reg); err != nil {
			return err
		}
		p.manager.RegisterTool(reg)
	case "register_command":
		var reg proto.RegisterCommand
		if err := proto.DecodeData(frame, &reg); err != nil {
			return err
		}
		p.manager.RegisterCommand(reg, p)
	case "subscribe":
		var sub proto.Subscribe
		if err := proto.DecodeData(frame, &sub); err != nil {
			return err
		}
		p.mu.Lock()
		for _, typ := range sub.Events {
			if typ != "" {
				p.subs[typ] = true
			}
		}
		p.mu.Unlock()
	case "tool_result":
		res, err := decodeProcessToolResult(frame)
		if err != nil {
			return err
		}
		if res.ID == "" {
			res.ID = frame.ID
		}
		if p.logs != nil && (res.Error != "" || res.Result.IsError) {
			msg := res.Error
			if msg == "" {
				msg = "tool result marked as error"
			}
			p.logs.AddSource("tool", "tool_result "+res.ID+": "+msg)
		}
		p.mu.Lock()
		ch := p.pendingT[res.ID]
		delete(p.pendingT, res.ID)
		p.mu.Unlock()
		if ch != nil {
			ch <- res
		}
	case "command_result":
		var res proto.CommandResult
		if err := proto.DecodeData(frame, &res); err != nil {
			return err
		}
		if res.ID == "" {
			res.ID = frame.ID
		}
		if p.logs != nil && res.Error != "" {
			p.logs.AddSource("command", "command_result "+res.ID+": "+res.Error)
		}
		p.mu.Lock()
		ch := p.pendingC[res.ID]
		delete(p.pendingC, res.ID)
		p.mu.Unlock()
		if ch != nil {
			ch <- res
		}
	case "host_action":
		var action proto.HostAction
		if err := proto.DecodeData(frame, &action); err != nil {
			return err
		}
		p.manager.AddHostAction(action)
	}
	return nil
}

func decodeProcessToolResult(frame proto.Frame) (proto.ToolResult, error) {
	var raw struct {
		ID     string `json:"id"`
		Error  string `json:"error,omitempty"`
		Result struct {
			IsError bool `json:"is_error,omitempty"`
			Content []struct {
				Text      string `json:"text,omitempty"`
				TextUpper string `json:"Text,omitempty"`
			} `json:"content,omitempty"`
		} `json:"result"`
	}
	if err := json.Unmarshal(frame.Data, &raw); err != nil {
		return proto.ToolResult{}, err
	}
	res := proto.ToolResult{ID: raw.ID, Error: raw.Error, Result: tool.Result{IsError: raw.Result.IsError}}
	for _, c := range raw.Result.Content {
		text := c.Text
		if text == "" {
			text = c.TextUpper
		}
		if text != "" {
			res.Result.Content = append(res.Result.Content, message.Text{Text: text})
		}
	}
	return res, nil
}

func (p *Process) write(typ, id string, v any) error {
	frame, err := proto.EncodeFrame(typ, id, v)
	if err != nil {
		return err
	}
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	_, err = p.stdin.Write(append(data, '\n'))
	return err
}

var _ Caller = (*Process)(nil)
var _ CommandCaller = (*Process)(nil)
