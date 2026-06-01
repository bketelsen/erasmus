package extension

import (
	"context"
	"sync"

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
}

// NewManager creates a manager using caller for tool execution.
func NewManager(caller Caller) *Manager {
	return &Manager{caller: caller, tools: map[string]tool.Tool{}, commands: map[string]Command{}}
}

// RegisterTool registers an extension tool.
func (m *Manager) RegisterTool(reg proto.RegisterTool) tool.Tool {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := NewTool(reg, m.caller)
	if _, ok := m.tools[reg.Name]; !ok {
		m.order = append(m.order, reg.Name)
	}
	m.tools[reg.Name] = t
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

var _ Host = (*Manager)(nil)
