package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/bketelsen/erasmus/packages/extension/proto"
)

// Command runs extension command calls.
type Command interface {
	Name() string
	Description() string
	Execute(ctx context.Context, input json.RawMessage) (proto.CommandResult, error)
}

// CommandCaller executes extension command calls.
type CommandCaller interface {
	CallCommand(ctx context.Context, call proto.CommandCall) (proto.CommandResult, error)
}

type command struct {
	reg    proto.RegisterCommand
	caller CommandCaller
	seq    atomic.Int64
}

// NewCommand creates an extension command wrapper.
func NewCommand(reg proto.RegisterCommand, caller CommandCaller) Command {
	return &command{reg: reg, caller: caller}
}

func (c *command) Name() string        { return c.reg.Name }
func (c *command) Description() string { return c.reg.Description }
func (c *command) Execute(ctx context.Context, input json.RawMessage) (proto.CommandResult, error) {
	if c.caller == nil {
		return proto.CommandResult{}, fmt.Errorf("extension command caller is nil")
	}
	id := fmt.Sprintf("%s-%d", c.reg.Name, c.seq.Add(1))
	res, err := c.caller.CallCommand(ctx, proto.CommandCall{ID: id, Name: c.reg.Name, Input: input})
	if err != nil {
		return proto.CommandResult{}, err
	}
	if res.Error != "" {
		return res, fmt.Errorf("%s%s", res.Error, formatDiagnosticsPath(callerLogPath(c.caller)))
	}
	return res, nil
}

// RegisterCommand registers a command.
func (m *Manager) RegisterCommand(reg proto.RegisterCommand, caller CommandCaller) Command {
	m.mu.Lock()
	cmd := NewCommand(reg, caller)
	m.commands[reg.Name] = cmd
	update := m.extensionUpdateLocked("register_command")
	m.mu.Unlock()
	m.publish(update)
	return cmd
}

// Command returns a command by name.
func (m *Manager) Command(name string) (Command, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cmd, ok := m.commands[name]
	return cmd, ok
}

// Commands returns all registered commands.
func (m *Manager) Commands() []Command {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Command, 0, len(m.commands))
	for _, cmd := range m.commands {
		out = append(out, cmd)
	}
	return out
}

// HostAction is an action emitted by an extension for the host.
type HostAction = proto.HostAction

// AddHostAction queues a host action.
func (m *Manager) AddHostAction(action HostAction) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actions = append(m.actions, action)
}

// DrainHostActions drains queued host actions.
func (m *Manager) DrainHostActions() []HostAction {
	m.mu.Lock()
	defer m.mu.Unlock()
	actions := append([]HostAction(nil), m.actions...)
	m.actions = nil
	return actions
}
