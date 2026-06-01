package extension

import (
	"context"

	"github.com/bketelsen/erasmus/packages/loop"
)

// ToolInterceptor can inspect or modify tool calls/results.
type ToolInterceptor interface {
	BeforeToolCall(context.Context, loop.ToolCallContext) (loop.ToolDecision, error)
	AfterToolCall(context.Context, loop.ToolResultContext) (loop.ToolResultPatch, error)
}

// AddInterceptor registers a tool interceptor.
func (m *Manager) AddInterceptor(i ToolInterceptor) {
	if i == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interceptors = append(m.interceptors, i)
}

// Hooks returns loop hooks backed by registered interceptors.
func (m *Manager) Hooks() loop.Hooks {
	return loop.Hooks{
		BeforeToolCall: func(ctx context.Context, tc loop.ToolCallContext) (loop.ToolDecision, error) {
			m.mu.Lock()
			interceptors := append([]ToolInterceptor(nil), m.interceptors...)
			m.mu.Unlock()
			var decision loop.ToolDecision
			for _, i := range interceptors {
				next, err := i.BeforeToolCall(ctx, tc)
				if err != nil {
					return loop.ToolDecision{}, err
				}
				if len(next.Arguments) > 0 {
					decision.Arguments = next.Arguments
					tc.Call.Arguments = next.Arguments
				}
				if next.Result != nil {
					decision.Result = next.Result
				}
				if next.Deny {
					decision.Deny = true
					return decision, nil
				}
			}
			return decision, nil
		},
		AfterToolCall: func(ctx context.Context, tr loop.ToolResultContext) (loop.ToolResultPatch, error) {
			m.mu.Lock()
			interceptors := append([]ToolInterceptor(nil), m.interceptors...)
			m.mu.Unlock()
			var patch loop.ToolResultPatch
			for _, i := range interceptors {
				next, err := i.AfterToolCall(ctx, tr)
				if err != nil {
					return loop.ToolResultPatch{}, err
				}
				if next.Result != nil {
					patch.Result = next.Result
					tr.Result = *next.Result
				}
			}
			return patch, nil
		},
	}
}
