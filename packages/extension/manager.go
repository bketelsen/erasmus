package extension

import (
	"context"
	"sync"

	"erasmus/packages/event"
	"erasmus/packages/extension/proto"
	"erasmus/packages/tool"
)

// Manager keeps registered extension tools.
type Manager struct {
	mu           sync.Mutex
	caller       Caller
	tools        map[string]tool.Tool
	order        []string
	interceptors []ToolInterceptor
	commands     map[string]Command
	actions      []HostAction
	subscribers  map[int]func(event.Event)
	nextSubID    int
}

// NewManager creates a manager using caller for tool execution.
func NewManager(caller Caller) *Manager {
	return &Manager{caller: caller, tools: map[string]tool.Tool{}, commands: map[string]Command{}, subscribers: map[int]func(event.Event){}}
}

// RegisterTool registers an extension tool.
func (m *Manager) RegisterTool(reg proto.RegisterTool) tool.Tool {
	m.mu.Lock()
	t := NewTool(reg, m.caller)
	if _, ok := m.tools[reg.Name]; !ok {
		m.order = append(m.order, reg.Name)
	}
	m.tools[reg.Name] = t
	update := m.extensionUpdateLocked("register_tool")
	m.mu.Unlock()
	m.publish(update)
	return t
}

// Registry returns registered extension tools as a tool registry.
func (m *Manager) Registry() tool.Registry {
	m.mu.Lock()
	defer m.mu.Unlock()
	tools := make([]tool.Tool, 0, len(m.order))
	for _, name := range m.order {
		tools = append(tools, m.tools[name])
	}
	return tool.NewRegistry(tools...)
}

// Host is the future extension host interface consumed by harness.
type Host interface {
	Tools(ctx context.Context) (tool.Registry, error)
}

// Tools implements Host.
func (m *Manager) Tools(ctx context.Context) (tool.Registry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return m.Registry(), nil
}

// Subscribe registers an extension manager event subscriber.
func (m *Manager) Subscribe(fn func(event.Event)) func() {
	m.mu.Lock()
	id := m.nextSubID
	m.nextSubID++
	m.subscribers[id] = fn
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		delete(m.subscribers, id)
		m.mu.Unlock()
	}
}

func (m *Manager) publish(ev event.Event) {
	m.mu.Lock()
	subs := make([]func(event.Event), 0, len(m.subscribers))
	for _, fn := range m.subscribers {
		subs = append(subs, fn)
	}
	m.mu.Unlock()
	for _, fn := range subs {
		fn(ev)
	}
}

func (m *Manager) extensionUpdateLocked(action string) event.ExtensionUpdate {
	tools := make([]tool.Spec, 0, len(m.order))
	for _, name := range m.order {
		t := m.tools[name]
		tools = append(tools, tool.Spec{Name: t.Name(), Description: t.Description(), Schema: t.Schema()})
	}
	commands := make([]event.ExtensionCommand, 0, len(m.commands))
	for _, cmd := range m.commands {
		commands = append(commands, event.ExtensionCommand{Name: cmd.Name(), Description: cmd.Description()})
	}
	return event.ExtensionUpdate{Action: action, Tools: tools, Commands: commands}
}

var _ Host = (*Manager)(nil)
