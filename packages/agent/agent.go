// Package agent wraps the low-level loop with in-memory state and lifecycle control.
package agent

import (
	"context"
	"errors"
	"sync"
	"time"

	"erasmus/packages/event"
	"erasmus/packages/loop"
	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/provider"
	"erasmus/packages/tool"
)

// QueueMode controls future steering/follow-up queue behavior.
type QueueMode string

const (
	QueueFIFO QueueMode = "fifo"
)

// Config configures an Agent.
type Config struct {
	InitialState State
	LoopConfig   loop.Config
	SteeringMode QueueMode
	FollowUpMode QueueMode
}

// State is a snapshot of the in-memory agent state.
type State struct {
	SystemPrompt     string
	Model            model.Model
	Reasoning        string
	Tools            tool.Registry
	Messages         []message.Message
	IsStreaming      bool
	StreamingMessage *message.Message
	PendingToolCalls map[string]message.ToolCall
	ErrorMessage     string
}

// Agent owns an in-memory transcript and one active loop run.
type Agent struct {
	mu          sync.Mutex
	state       State
	loopConfig  loop.Config
	subscribers map[int]func(event.Event)
	nextSubID   int
	cancel      context.CancelFunc
	done        chan error
	steering    []message.Message
	followUp    []message.Message
}

// New creates an Agent.
func New(cfg Config) *Agent {
	st := cfg.InitialState
	st.Messages = copyMessages(st.Messages)
	if st.PendingToolCalls == nil {
		st.PendingToolCalls = map[string]message.ToolCall{}
	}
	return &Agent{
		state:       st,
		loopConfig:  cfg.LoopConfig,
		subscribers: map[int]func(event.Event){},
	}
}

// Prompt sends a user text prompt.
func (a *Agent) Prompt(ctx context.Context, text string, images []message.Image) error {
	content := []message.Content{message.Text{Text: text}}
	for _, img := range images {
		content = append(content, img)
	}
	return a.PromptMessages(ctx, []message.Message{{Role: message.RoleUser, Content: content, Time: time.Now()}})
}

// PromptMessages appends prompt messages and starts a loop run.
func (a *Agent) PromptMessages(ctx context.Context, msgs []message.Message) error {
	return a.start(ctx, msgs, false)
}

// Continue starts a loop run without appending new prompt messages.
func (a *Agent) Continue(ctx context.Context) error {
	return a.start(ctx, nil, true)
}

func (a *Agent) start(ctx context.Context, prompts []message.Message, cont bool) error {
	a.mu.Lock()
	if a.state.IsStreaming {
		a.mu.Unlock()
		return errors.New("agent is already streaming")
	}
	runCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.done = make(chan error, 1)
	a.state.IsStreaming = true
	a.state.ErrorMessage = ""
	base := loop.Context{SystemPrompt: a.state.SystemPrompt, Messages: copyMessages(a.state.Messages), Tools: a.state.Tools}
	cfg := a.loopConfig
	if cfg.Model.ID == "" {
		cfg.Model = a.state.Model
	}
	if cfg.Reasoning == "" {
		cfg.Reasoning = a.state.Reasoning
	}
	cfg.GetSteering = a.drainSteering
	cfg.GetFollowUp = a.drainFollowUp
	done := a.done
	a.mu.Unlock()

	go func() {
		var messages []message.Message
		var err error
		sawAgentEnd := false
		emit := func(ev event.Event) error {
			if _, ok := ev.(event.AgentEnd); ok {
				sawAgentEnd = true
			}
			a.applyEvent(ev)
			a.publish(ev)
			return nil
		}
		if cont {
			messages, err = loop.Continue(runCtx, base, cfg, emit)
		} else {
			messages, err = loop.Run(runCtx, prompts, base, cfg, emit)
		}

		if !sawAgentEnd {
			_ = emit(event.AgentEnd{Messages: messages})
		}

		a.mu.Lock()
		a.state.IsStreaming = false
		a.state.StreamingMessage = nil
		a.state.PendingToolCalls = map[string]message.ToolCall{}
		if len(messages) > 0 {
			a.state.Messages = copyMessages(messages)
		}
		errText := ""
		if err != nil {
			errText = err.Error()
			a.state.ErrorMessage = errText
		}
		a.mu.Unlock()
		if errText != "" {
			_ = emit(event.Error{Err: errText})
		}
		_ = emit(event.Settled{Err: errText})
		done <- err
		close(done)
	}()
	return nil
}

// Abort cancels the active run, if any.
func (a *Agent) Abort() {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Wait waits for the active run to complete.
func (a *Agent) Wait(ctx context.Context) error {
	a.mu.Lock()
	done := a.done
	a.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Subscribe registers an event subscriber and returns an unsubscribe function.
func (a *Agent) Subscribe(fn func(event.Event)) func() {
	a.mu.Lock()
	id := a.nextSubID
	a.nextSubID++
	a.subscribers[id] = fn
	a.mu.Unlock()
	return func() {
		a.mu.Lock()
		delete(a.subscribers, id)
		a.mu.Unlock()
	}
}

// State returns a snapshot.
func (a *Agent) State() State {
	a.mu.Lock()
	defer a.mu.Unlock()
	return copyState(a.state)
}

// SetMessages replaces the in-memory transcript.
func (a *Agent) SetMessages(messages []message.Message) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Messages = copyMessages(messages)
}

// SetModel updates the model used for subsequent runs.
func (a *Agent) SetModel(m model.Model) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Model = m
	a.loopConfig.Model = m
}

// SetStream updates the provider stream used for subsequent runs.
func (a *Agent) SetStream(stream provider.StreamFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.loopConfig.Stream = stream
}

// SetReasoning updates the reasoning level used for subsequent runs.
func (a *Agent) SetReasoning(reasoning string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Reasoning = reasoning
	a.loopConfig.Reasoning = reasoning
}

// SetTools updates the tool registry used for subsequent runs.
func (a *Agent) SetTools(tools tool.Registry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Tools = tools
}

// Messages returns a transcript copy.
func (a *Agent) Messages() []message.Message {
	return a.State().Messages
}

// Steer queues a steering message to be appended before the next provider turn.
func (a *Agent) Steer(msg message.Message) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.steering = append(a.steering, msg)
}

// FollowUp queues a message to run after the current turn would otherwise settle.
func (a *Agent) FollowUp(msg message.Message) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.followUp = append(a.followUp, msg)
}

func (a *Agent) drainSteering(context.Context) ([]message.Message, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	msgs := copyMessages(a.steering)
	a.steering = nil
	return msgs, nil
}

func (a *Agent) drainFollowUp(context.Context) ([]message.Message, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	msgs := copyMessages(a.followUp)
	a.followUp = nil
	return msgs, nil
}

func (a *Agent) applyEvent(ev event.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch e := ev.(type) {
	case event.MessageStart:
		msg := e.Message
		a.state.StreamingMessage = &msg
	case event.MessageDelta:
		if a.state.StreamingMessage == nil {
			msg := message.Message{Role: message.RoleAssistant}
			a.state.StreamingMessage = &msg
		}
		a.state.StreamingMessage.Content = appendText(a.state.StreamingMessage.Content, e.Text)
	case event.MessageEnd:
		if e.Message.Role == message.RoleAssistant {
			a.state.StreamingMessage = nil
		}
	case event.ToolExecutionStart:
		if a.state.PendingToolCalls == nil {
			a.state.PendingToolCalls = map[string]message.ToolCall{}
		}
		a.state.PendingToolCalls[e.ID] = message.ToolCall{ID: e.ID, Name: e.Name, Arguments: e.Args}
	case event.ToolExecutionEnd:
		delete(a.state.PendingToolCalls, e.ID)
	case event.Error:
		a.state.ErrorMessage = e.Err
	}
}

func (a *Agent) publish(ev event.Event) {
	a.mu.Lock()
	subs := make([]func(event.Event), 0, len(a.subscribers))
	for _, fn := range a.subscribers {
		subs = append(subs, fn)
	}
	a.mu.Unlock()
	for _, fn := range subs {
		fn(ev)
	}
}

func copyState(st State) State {
	st.Messages = copyMessages(st.Messages)
	if st.StreamingMessage != nil {
		msg := *st.StreamingMessage
		msg.Content = append([]message.Content(nil), msg.Content...)
		st.StreamingMessage = &msg
	}
	pending := make(map[string]message.ToolCall, len(st.PendingToolCalls))
	for k, v := range st.PendingToolCalls {
		pending[k] = v
	}
	st.PendingToolCalls = pending
	return st
}

func copyMessages(in []message.Message) []message.Message {
	out := make([]message.Message, len(in))
	copy(out, in)
	return out
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
