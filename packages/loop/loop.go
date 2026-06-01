// Package loop implements the low-level provider/tool cycle.
package loop

import (
	"context"
	"errors"
	"fmt"
	"time"

	"erasmus/packages/event"
	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/provider"
	"erasmus/packages/tool"
)

// Context is the immutable-ish context for a loop run.
type Context struct {
	SystemPrompt string
	Messages     []message.Message
	Tools        tool.Registry
}

// Config controls loop execution.
type Config struct {
	Model         model.Model
	Reasoning     string
	ToolExecution tool.ExecutionMode
	MaxSteps      int
	Stream        provider.StreamFunc
	Hooks         Hooks
	GetSteering   func(context.Context) ([]message.Message, error)
	GetFollowUp   func(context.Context) ([]message.Message, error)
	MaxTokens     int
	SessionID     string
}

// Hooks customizes low-level loop behavior.
type Hooks struct {
	TransformContext      func(context.Context, []message.Message) ([]message.Message, error)
	BeforeProviderRequest func(context.Context, *provider.Request) error
	BeforeToolCall        func(context.Context, ToolCallContext) (ToolDecision, error)
	AfterToolCall         func(context.Context, ToolResultContext) (ToolResultPatch, error)
	BeforeAssistantCommit func(context.Context, message.Message) (AssistantDecision, error)
}

// ToolCallContext describes a pending tool call.
type ToolCallContext struct {
	Call message.ToolCall
	Tool tool.Tool
}

// ToolDecision may allow, deny, or patch a tool call before execution.
type ToolDecision struct {
	Deny      bool
	Result    *tool.Result
	Arguments []byte
}

// ToolResultContext describes a completed tool call.
type ToolResultContext struct {
	Call   message.ToolCall
	Tool   tool.Tool
	Result tool.Result
	Err    error
}

// ToolResultPatch may replace a tool result after execution.
type ToolResultPatch struct {
	Result *tool.Result
}

// AssistantDecision may patch an assistant message before it is committed.
type AssistantDecision struct {
	Message *message.Message
}

// Run appends prompts to context and runs provider/tool turns until completion.
func Run(ctx context.Context, prompts []message.Message, c Context, cfg Config, emit func(event.Event) error) ([]message.Message, error) {
	messages := append(copyMessages(c.Messages), prompts...)
	return run(ctx, messages, c, cfg, emit)
}

// Continue runs provider/tool turns with the supplied context messages.
func Continue(ctx context.Context, c Context, cfg Config, emit func(event.Event) error) ([]message.Message, error) {
	return run(ctx, copyMessages(c.Messages), c, cfg, emit)
}

func run(ctx context.Context, messages []message.Message, c Context, cfg Config, emit func(event.Event) error) ([]message.Message, error) {
	if cfg.Stream == nil {
		return nil, errors.New("loop stream function is required")
	}
	maxSteps := cfg.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 8
	}

	var cumulative model.Usage
	if err := emitEvent(emit, event.AgentStart{}); err != nil {
		return messages, err
	}

	for step := 1; step <= maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return messages, err
		}
		if err := emitEvent(emit, event.TurnStart{Step: step}); err != nil {
			return messages, err
		}

		requestMessages := copyMessages(messages)
		if cfg.Hooks.TransformContext != nil {
			next, err := cfg.Hooks.TransformContext(ctx, requestMessages)
			if err != nil {
				emitErr := emitEvent(emit, event.TurnEnd{Step: step, Err: err.Error()})
				if emitErr != nil {
					return messages, emitErr
				}
				return messages, err
			}
			requestMessages = copyMessages(next)
		}

		req := provider.Request{
			Model:     cfg.Model,
			System:    c.SystemPrompt,
			Messages:  requestMessages,
			Reasoning: cfg.Reasoning,
			MaxTokens: cfg.MaxTokens,
			SessionID: cfg.SessionID,
		}
		if c.Tools != nil {
			req.Tools = c.Tools.Specs()
		}
		if cfg.Hooks.BeforeProviderRequest != nil {
			if err := cfg.Hooks.BeforeProviderRequest(ctx, &req); err != nil {
				emitErr := emitEvent(emit, event.TurnEnd{Step: step, Err: err.Error()})
				if emitErr != nil {
					return messages, emitErr
				}
				return messages, err
			}
		}

		stream, err := cfg.Stream(ctx, req)
		if err != nil {
			emitErr := emitEvent(emit, event.TurnEnd{Step: step, Err: err.Error()})
			if emitErr != nil {
				return messages, emitErr
			}
			return messages, err
		}

		assistant, toolCalls, stop, err := consumeProviderStream(ctx, stream, emit, &cumulative)
		if err != nil {
			emitErr := emitEvent(emit, event.TurnEnd{Step: step, Stop: stop, Err: err.Error()})
			if emitErr != nil {
				return messages, emitErr
			}
			return messages, err
		}
		if len(assistant.Content) > 0 {
			if cfg.Hooks.BeforeAssistantCommit != nil {
				decision, err := cfg.Hooks.BeforeAssistantCommit(ctx, assistant)
				if err != nil {
					emitErr := emitEvent(emit, event.TurnEnd{Step: step, Stop: stop, Err: err.Error()})
					if emitErr != nil {
						return messages, emitErr
					}
					return messages, err
				}
				if decision.Message != nil {
					assistant = *decision.Message
				}
			}
			messages = append(messages, assistant)
			if err := emitEvent(emit, event.MessageEnd{Message: assistant}); err != nil {
				return messages, err
			}
		}

		if len(toolCalls) == 0 {
			if err := emitEvent(emit, event.TurnEnd{Step: step, Stop: stop}); err != nil {
				return messages, err
			}
			queued, err := pollQueuedMessages(ctx, cfg.GetSteering, cfg.GetFollowUp)
			if err != nil {
				return messages, err
			}
			if len(queued) == 0 {
				if err := emitEvent(emit, event.AgentEnd{Messages: messages}); err != nil {
					return messages, err
				}
				return messages, nil
			}
			messages = append(messages, queued...)
			continue
		}

		toolMessages, err := executeToolCalls(ctx, c.Tools, toolCalls, cfg.Hooks, emit)
		messages = append(messages, toolMessages...)
		if err != nil {
			emitErr := emitEvent(emit, event.TurnEnd{Step: step, Stop: "tool_error", Err: err.Error()})
			if emitErr != nil {
				return messages, emitErr
			}
			return messages, err
		}
		if err := emitEvent(emit, event.TurnEnd{Step: step, Stop: "tool_use"}); err != nil {
			return messages, err
		}
		queued, err := pollQueuedMessages(ctx, cfg.GetSteering, nil)
		if err != nil {
			return messages, err
		}
		messages = append(messages, queued...)
	}

	err := fmt.Errorf("loop exceeded max steps %d", maxSteps)
	_ = emitEvent(emit, event.AgentEnd{Messages: messages})
	return messages, err
}

func pollQueuedMessages(ctx context.Context, pollers ...func(context.Context) ([]message.Message, error)) ([]message.Message, error) {
	var queued []message.Message
	for _, poll := range pollers {
		if poll == nil {
			continue
		}
		msgs, err := poll(ctx)
		if err != nil {
			return nil, err
		}
		queued = append(queued, msgs...)
	}
	return queued, nil
}

func consumeProviderStream(ctx context.Context, stream <-chan provider.Event, emit func(event.Event) error, cumulative *model.Usage) (message.Message, []message.ToolCall, string, error) {
	assistant := message.Message{Role: message.RoleAssistant, Time: time.Now()}
	var toolCalls []message.ToolCall
	stop := ""
	started := false

	for {
		select {
		case <-ctx.Done():
			return assistant, toolCalls, stop, ctx.Err()
		case ev, ok := <-stream:
			if !ok {
				if err := ctx.Err(); err != nil {
					return assistant, toolCalls, stop, err
				}
				return assistant, toolCalls, stop, nil
			}
			switch e := ev.(type) {
			case provider.MessageStart:
				started = true
				assistant.ID = e.MessageID
				if err := emitEvent(emit, event.MessageStart{Message: assistant}); err != nil {
					return assistant, toolCalls, stop, err
				}
			case provider.TextDelta:
				if !started {
					started = true
					if err := emitEvent(emit, event.MessageStart{Message: assistant}); err != nil {
						return assistant, toolCalls, stop, err
					}
				}
				assistant.Content = appendText(assistant.Content, e.Text)
				if err := emitEvent(emit, event.MessageDelta{MessageID: assistant.ID, Text: e.Text}); err != nil {
					return assistant, toolCalls, stop, err
				}
			case provider.ToolCall:
				call := message.ToolCall{ID: e.ID, Name: e.Name, Arguments: e.Arguments}
				assistant.Content = append(assistant.Content, call)
				toolCalls = append(toolCalls, call)
			case provider.Usage:
				*cumulative = addUsage(*cumulative, e.Usage)
				if err := emitEvent(emit, event.Usage{Usage: e.Usage, Cumulative: *cumulative}); err != nil {
					return assistant, toolCalls, stop, err
				}
			case provider.MessageEnd:
				stop = e.StopReason
			case provider.Error:
				return assistant, toolCalls, stop, errors.New(e.Err)
			default:
				return assistant, toolCalls, stop, fmt.Errorf("unknown provider event %T", ev)
			}
		}
	}
}

func executeToolCalls(ctx context.Context, registry tool.Registry, calls []message.ToolCall, hooks Hooks, emit func(event.Event) error) ([]message.Message, error) {
	messages := make([]message.Message, 0, len(calls))
	for _, call := range calls {
		if registry == nil {
			result := errorToolResult("tool registry is not configured")
			if err := emitEvent(emit, event.ToolExecutionEnd{ID: call.ID, Name: call.Name, Result: result, IsError: true}); err != nil {
				return messages, err
			}
			messages = append(messages, toolResultMessage(call.ID, result))
			continue
		}
		t, ok := registry.Get(call.Name)
		if !ok {
			result := errorToolResult(fmt.Sprintf("tool %q not found", call.Name))
			if err := emitEvent(emit, event.ToolExecutionEnd{ID: call.ID, Name: call.Name, Result: result, IsError: true}); err != nil {
				return messages, err
			}
			messages = append(messages, toolResultMessage(call.ID, result))
			continue
		}
		if hooks.BeforeToolCall != nil {
			decision, err := hooks.BeforeToolCall(ctx, ToolCallContext{Call: call, Tool: t})
			if err != nil {
				return messages, err
			}
			if len(decision.Arguments) > 0 {
				call.Arguments = decision.Arguments
			}
			if decision.Deny {
				result := tool.Result{IsError: true, Content: []message.Content{message.Text{Text: "tool call denied"}}}
				if decision.Result != nil {
					result = *decision.Result
				}
				if err := emitEvent(emit, event.ToolExecutionEnd{ID: call.ID, Name: call.Name, Result: result, IsError: result.IsError}); err != nil {
					return messages, err
				}
				messages = append(messages, toolResultMessage(call.ID, result))
				continue
			}
		}
		if err := emitEvent(emit, event.ToolExecutionStart{ID: call.ID, Name: call.Name, Args: call.Arguments}); err != nil {
			return messages, err
		}
		result, err := t.Execute(ctx, call.Arguments, func(p tool.Progress) {
			_ = emitEvent(emit, event.ToolExecutionProgress{ID: call.ID, Text: p.Text})
		})
		if err != nil {
			result.IsError = true
			if len(result.Content) == 0 {
				result.Content = []message.Content{message.Text{Text: err.Error()}}
			}
		}
		if hooks.AfterToolCall != nil {
			patch, hookErr := hooks.AfterToolCall(ctx, ToolResultContext{Call: call, Tool: t, Result: result, Err: err})
			if hookErr != nil {
				return messages, hookErr
			}
			if patch.Result != nil {
				result = *patch.Result
			}
		}
		if err := emitEvent(emit, event.ToolExecutionEnd{ID: call.ID, Name: call.Name, Result: result, IsError: result.IsError}); err != nil {
			return messages, err
		}
		messages = append(messages, toolResultMessage(call.ID, result))
	}
	return messages, nil
}

func errorToolResult(text string) tool.Result {
	return tool.Result{IsError: true, Content: []message.Content{message.Text{Text: text}}}
}

func toolResultMessage(callID string, result tool.Result) message.Message {
	return message.Message{
		Role: message.RoleTool,
		Time: time.Now(),
		Content: []message.Content{message.ToolResult{
			CallID:  callID,
			Content: result.Content,
			IsError: result.IsError,
		}},
	}
}

func emitEvent(emit func(event.Event) error, ev event.Event) error {
	if emit == nil {
		return nil
	}
	return emit(ev)
}

func appendText(content []message.Content, text string) []message.Content {
	if len(content) > 0 {
		if last, ok := content[len(content)-1].(message.Text); ok {
			last.Text += text
			content[len(content)-1] = last
			return content
		}
	}
	return append(content, message.Text{Text: text})
}

func addUsage(a, b model.Usage) model.Usage {
	return model.Usage{
		InputTokens:      a.InputTokens + b.InputTokens,
		OutputTokens:     a.OutputTokens + b.OutputTokens,
		ReasoningTokens:  a.ReasoningTokens + b.ReasoningTokens,
		CacheReadTokens:  a.CacheReadTokens + b.CacheReadTokens,
		CacheWriteTokens: a.CacheWriteTokens + b.CacheWriteTokens,
	}
}

func copyMessages(in []message.Message) []message.Message {
	out := make([]message.Message, len(in))
	copy(out, in)
	return out
}
