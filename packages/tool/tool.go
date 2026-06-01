// Package tool defines the common interface for built-in, SDK, and extension tools.
package tool

import (
	"context"
	"encoding/json"

	"github.com/bketelsen/erasmus/packages/message"
)

// ExecutionMode controls whether tool batches may run concurrently.
type ExecutionMode string

const (
	ToolParallel   ExecutionMode = "parallel"
	ToolSequential ExecutionMode = "sequential"
)

// Progress describes an incremental tool execution update.
type Progress struct {
	Text string `json:"text,omitempty"`
	Data any    `json:"data,omitempty"`
}

// Result is the canonical result returned by tools.
type Result struct {
	Content   []message.Content `json:"content,omitempty"`
	IsError   bool              `json:"is_error,omitempty"`
	Details   any               `json:"details,omitempty"`
	Terminate bool              `json:"terminate,omitempty"`
}

// Tool is implemented by every executable tool.
type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	Execute(ctx context.Context, args json.RawMessage, progress func(Progress)) (Result, error)
}

// Spec is the provider-facing tool description.
type Spec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
}

// Registry resolves tools and exposes provider-facing specs.
type Registry interface {
	Get(name string) (Tool, bool)
	List() []Tool
	Specs() []Spec
}

type registry struct {
	tools map[string]Tool
	order []string
}

// NewRegistry creates a registry from tools. Later tools with duplicate names replace earlier ones.
func NewRegistry(tools ...Tool) Registry {
	r := &registry{tools: map[string]Tool{}}
	for _, t := range tools {
		if t == nil {
			continue
		}
		name := t.Name()
		if _, exists := r.tools[name]; !exists {
			r.order = append(r.order, name)
		}
		r.tools[name] = t
	}
	return r
}

func (r *registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *registry) List() []Tool {
	out := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.tools[name])
	}
	return out
}

func (r *registry) Specs() []Spec {
	tools := r.List()
	out := make([]Spec, 0, len(tools))
	for _, t := range tools {
		out = append(out, Spec{Name: t.Name(), Description: t.Description(), Schema: t.Schema()})
	}
	return out
}

// Select returns a registry containing only named tools, preserving names order.
func Select(source Registry, names []string) Registry {
	if source == nil {
		return NewRegistry()
	}
	if len(names) == 0 {
		return source
	}
	selected := make([]Tool, 0, len(names))
	for _, name := range names {
		if t, ok := source.Get(name); ok {
			selected = append(selected, t)
		}
	}
	return NewRegistry(selected...)
}
