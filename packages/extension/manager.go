package extension

import (
	"context"
	"sync"

	"github.com/bketelsen/erasmus/packages/event"
	"github.com/bketelsen/erasmus/packages/extension/proto"
	"github.com/bketelsen/erasmus/packages/skill"
	"github.com/bketelsen/erasmus/packages/tool"
)

// Manager keeps registered extension tools.
type Manager struct {
	mu           sync.Mutex
	caller       Caller
	tools        map[string]tool.Tool
	order        []string
	interceptors []ToolInterceptor
	commands     map[string]Command
	skills       map[string]skill.Skill
	skillOrder   []string
	actions      []HostAction
	subscribers  map[int]func(event.Event)
	nextSubID    int
}

// NewManager creates a manager using caller for tool execution.
func NewManager(caller Caller) *Manager {
	return &Manager{caller: caller, tools: map[string]tool.Tool{}, commands: map[string]Command{}, skills: map[string]skill.Skill{}, subscribers: map[int]func(event.Event){}}
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

// RegisterSkill registers an extension prompt skill.
func (m *Manager) RegisterSkill(reg proto.RegisterSkill) skill.Skill {
	m.mu.Lock()
	s := skill.Skill{Name: reg.Name, Description: reg.Description, Body: reg.Body, Source: reg.Source}
	if s.Source == "" {
		s.Source = "extension:" + reg.Name
	}
	if _, ok := m.skills[reg.Name]; !ok {
		m.skillOrder = append(m.skillOrder, reg.Name)
	}
	m.skills[reg.Name] = s
	update := m.extensionUpdateLocked("register_skill")
	m.mu.Unlock()
	m.publish(update)
	return s
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

// Skills returns registered extension prompt skills.
func (m *Manager) Skills() []skill.Skill {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]skill.Skill, 0, len(m.skillOrder))
	for _, name := range m.skillOrder {
		out = append(out, m.skills[name])
	}
	return out
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
	skills := make([]skill.Skill, 0, len(m.skillOrder))
	for _, name := range m.skillOrder {
		skills = append(skills, m.skills[name])
	}
	return event.ExtensionUpdate{Action: action, Tools: tools, Commands: commands, Skills: skills}
}

var _ Host = (*Manager)(nil)
