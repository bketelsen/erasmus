// Package harness is the central durable Erasmus runtime abstraction.
package harness

import (
	"context"
	"fmt"
	"sync"

	"erasmus/packages/agent"
	"erasmus/packages/compact"
	"erasmus/packages/event"
	"erasmus/packages/loop"
	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/prompt"
	"erasmus/packages/provider"
	"erasmus/packages/session"
	"erasmus/packages/skill"
	"erasmus/packages/tool"
)

// Config configures a Harness.
type Config struct {
	Session         session.Session
	Stream          provider.StreamFunc
	Model           model.Model
	Reasoning       string
	SystemPrompt    string
	Prompt          prompt.Builder
	Skills          []skill.Skill
	Tools           tool.Registry
	ActiveTools     []string
	Hooks           Hooks
	LoopHooks       loop.Hooks
	ConfirmToolCall func(context.Context, loop.ToolCallContext) (bool, error)
	MaxSteps        int
}

// PromptOptions controls prompt submission.
type PromptOptions struct{}

// Hooks customizes harness-level runtime behavior.
type Hooks struct {
	ToolCall func(context.Context, ToolCallContext) (ToolCallDecision, error)
}

// ToolCallContext describes a pending tool call observed by harness hooks.
type ToolCallContext struct {
	Call message.ToolCall
	Tool tool.Tool
}

// ToolCallDecision may allow, deny, or patch a tool call before execution.
type ToolCallDecision struct {
	Deny      bool
	Result    *tool.Result
	Arguments []byte
}

// Resources groups runtime prompt resources that can be changed together.
type Resources struct {
	Skills      []skill.Skill
	Tools       []tool.Tool
	ActiveTools []string
}

// State is the harness state exposed to frontends.
type State struct {
	Agent   agent.State
	Session session.Metadata
	Skills  []skill.Skill
}

// TreeState describes the backing session tree when supported.
type TreeState struct {
	LeafID  session.EntryID     `json:"leaf_id,omitempty"`
	Entries []session.TreeEntry `json:"entries,omitempty"`
}

// Harness owns session persistence around an in-memory agent.
type Harness struct {
	mu      sync.Mutex
	session session.Session
	agent   *agent.Agent
	stream  provider.StreamFunc
	subs    map[int]func(event.Event)
	nextSub int
	seen    int
	skills  []skill.Skill
	tools   tool.Registry
}

// New creates a harness from durable session context.
func New(ctx context.Context, cfg Config) (*Harness, error) {
	if cfg.Session == nil {
		return nil, fmt.Errorf("session is required")
	}
	if cfg.Stream == nil {
		return nil, fmt.Errorf("stream function is required")
	}
	built, err := cfg.Session.BuildContext(ctx)
	if err != nil {
		return nil, err
	}
	m := cfg.Model
	if m.ID == "" {
		m = built.Model
	}
	reasoning := cfg.Reasoning
	if reasoning == "" {
		reasoning = built.Reasoning
	}
	systemPrompt := cfg.SystemPrompt
	activeTools := tool.Select(cfg.Tools, cfg.ActiveTools)
	if systemPrompt == "" && cfg.Prompt != nil {
		meta, err := cfg.Session.Metadata(ctx)
		if err != nil {
			return nil, err
		}
		var promptTools []tool.Tool
		if activeTools != nil {
			promptTools = activeTools.List()
		}
		systemPrompt, err = cfg.Prompt.Build(ctx, prompt.Input{Model: m, Reasoning: reasoning, ActiveTools: promptTools, Skills: cfg.Skills, SessionMeta: meta})
		if err != nil {
			return nil, err
		}
	}
	loopHooks := composeLoopHooks(cfg.LoopHooks, cfg.Hooks, cfg.ConfirmToolCall)
	a := agent.New(agent.Config{
		InitialState: agent.State{SystemPrompt: systemPrompt, Model: m, Reasoning: reasoning, Tools: activeTools, Messages: built.Messages},
		LoopConfig:   loop.Config{Model: m, Reasoning: reasoning, Stream: cfg.Stream, Hooks: loopHooks, MaxSteps: cfg.MaxSteps, SessionID: cfg.Session.ID()},
	})
	h := &Harness{session: cfg.Session, agent: a, stream: cfg.Stream, subs: map[int]func(event.Event){}, seen: len(built.Messages), skills: append([]skill.Skill(nil), cfg.Skills...), tools: cfg.Tools}
	a.Subscribe(h.handleEvent)
	return h, nil
}

func composeLoopHooks(hooks loop.Hooks, harnessHooks Hooks, confirm func(context.Context, loop.ToolCallContext) (bool, error)) loop.Hooks {
	if harnessHooks.ToolCall == nil && confirm == nil {
		return hooks
	}
	previous := hooks.BeforeToolCall
	hooks.BeforeToolCall = func(ctx context.Context, tc loop.ToolCallContext) (loop.ToolDecision, error) {
		var decision loop.ToolDecision
		if previous != nil {
			prior, err := previous(ctx, tc)
			if err != nil || prior.Deny {
				return prior, err
			}
			decision = prior
			if len(prior.Arguments) > 0 {
				tc.Call.Arguments = prior.Arguments
			}
		}
		if harnessHooks.ToolCall != nil {
			next, err := harnessHooks.ToolCall(ctx, ToolCallContext{Call: tc.Call, Tool: tc.Tool})
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
		if confirm != nil {
			ok, err := confirm(ctx, tc)
			if err != nil {
				return loop.ToolDecision{}, err
			}
			if !ok {
				decision.Deny = true
				return decision, nil
			}
		}
		return decision, nil
	}
	return hooks
}

// Prompt starts a prompt and returns a subscription channel for future events.
func (h *Harness) Prompt(ctx context.Context, text string, opts PromptOptions) (<-chan event.Event, error) {
	ch, unsubscribe := h.eventChan()
	if err := h.agent.Prompt(ctx, text, nil); err != nil {
		unsubscribe()
		close(ch)
		return nil, err
	}
	return ch, nil
}

// Continue continues the session.
func (h *Harness) Continue(ctx context.Context) (<-chan event.Event, error) {
	ch, unsubscribe := h.eventChan()
	if err := h.agent.Continue(ctx); err != nil {
		unsubscribe()
		close(ch)
		return nil, err
	}
	return ch, nil
}

// Abort aborts the active run.
func (h *Harness) Abort(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h.agent.Abort()
	return nil
}

// Wait waits for the active run.
func (h *Harness) Wait(ctx context.Context) error { return h.agent.Wait(ctx) }

// State returns a state snapshot.
func (h *Harness) State(ctx context.Context) State {
	meta, _ := h.session.Metadata(ctx)
	h.mu.Lock()
	skills := append([]skill.Skill(nil), h.skills...)
	h.mu.Unlock()
	return State{Agent: h.agent.State(), Session: meta, Skills: skills}
}

// Subscribe subscribes to harness events.
func (h *Harness) Subscribe(fn func(event.Event)) func() {
	h.mu.Lock()
	id := h.nextSub
	h.nextSub++
	h.subs[id] = fn
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		delete(h.subs, id)
		h.mu.Unlock()
	}
}

// Compact summarizes earlier transcript and updates in-memory context.
func (h *Harness) Compact(ctx context.Context, opts compact.Options) (compact.Result, error) {
	messages := h.agent.Messages()
	prep, err := compact.Prepare(messages, opts)
	if err != nil {
		return compact.Result{}, err
	}
	result, err := compact.Run(ctx, h.stream, prep)
	if err != nil {
		return compact.Result{}, err
	}
	if _, err := h.session.AppendCompaction(ctx, session.Compaction{Summary: result.Summary}); err != nil {
		return compact.Result{}, err
	}
	for _, msg := range result.Messages[1:] {
		if _, err := h.session.AppendMessage(ctx, msg); err != nil {
			return compact.Result{}, err
		}
	}
	h.agent.SetMessages(result.Messages)
	h.mu.Lock()
	h.seen = len(result.Messages)
	h.mu.Unlock()
	h.publish(event.SessionCompact{Summary: result.Summary, TokensBefore: result.TokensBefore})
	return result, nil
}

// SetModel updates the runtime model and persists the change.
func (h *Harness) SetModel(ctx context.Context, m model.Model) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := h.session.AppendModelChange(ctx, m.Provider, m.ID); err != nil {
		return err
	}
	h.agent.SetModel(m)
	h.publish(event.ModelUpdate{Model: m})
	return nil
}

// SetModelAndStream updates the runtime model and provider stream together.
func (h *Harness) SetModelAndStream(ctx context.Context, m model.Model, stream provider.StreamFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if stream == nil {
		return fmt.Errorf("stream function is required")
	}
	if _, err := h.session.AppendModelChange(ctx, m.Provider, m.ID); err != nil {
		return err
	}
	h.mu.Lock()
	h.stream = stream
	h.mu.Unlock()
	h.agent.SetStream(stream)
	h.agent.SetModel(m)
	h.publish(event.ModelUpdate{Model: m})
	return nil
}

// SetReasoning updates the runtime reasoning level and persists the change.
func (h *Harness) SetReasoning(ctx context.Context, reasoning string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := h.session.AppendReasoningChange(ctx, reasoning); err != nil {
		return err
	}
	h.agent.SetReasoning(reasoning)
	h.publish(event.ReasoningUpdate{Reasoning: reasoning})
	return nil
}

// SetSkills updates harness skill resources and emits a resource update event.
func (h *Harness) SetSkills(ctx context.Context, skills []skill.Skill) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h.mu.Lock()
	h.skills = copySkills(skills)
	h.mu.Unlock()
	h.publish(event.ResourcesUpdate{Skills: copySkills(skills)})
	return nil
}

// SetTools replaces the known tool set and selects the active tools.
func (h *Harness) SetTools(ctx context.Context, tools []tool.Tool, active []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	registry := tool.NewRegistry(tools...)
	selected := tool.Select(registry, active)
	h.mu.Lock()
	h.tools = registry
	h.mu.Unlock()
	h.agent.SetTools(selected)
	h.publish(event.ResourcesUpdate{Tools: toolSpecs(selected), ActiveTools: toolNames(selected)})
	return nil
}

// SetActiveTools changes which known tools are exposed to subsequent runs.
func (h *Harness) SetActiveTools(ctx context.Context, names []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h.mu.Lock()
	registry := h.tools
	h.mu.Unlock()
	selected := tool.Select(registry, names)
	h.agent.SetTools(selected)
	h.publish(event.ResourcesUpdate{Tools: toolSpecs(selected), ActiveTools: toolNames(selected)})
	return nil
}

// SetResources updates skill and tool resources together.
func (h *Harness) SetResources(ctx context.Context, resources Resources) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	registry := tool.NewRegistry(resources.Tools...)
	selected := tool.Select(registry, resources.ActiveTools)
	skills := copySkills(resources.Skills)
	h.mu.Lock()
	h.skills = skills
	h.tools = registry
	h.mu.Unlock()
	h.agent.SetTools(selected)
	h.publish(event.ResourcesUpdate{Skills: copySkills(skills), Tools: toolSpecs(selected), ActiveTools: toolNames(selected)})
	return nil
}

// Tree returns session tree state when the backing session supports session.Tree.
func (h *Harness) Tree(ctx context.Context) (TreeState, error) {
	tree, ok := h.session.(session.Tree)
	if !ok {
		return TreeState{}, fmt.Errorf("session %q does not support tree navigation", h.session.ID())
	}
	leaf, err := tree.LeafID(ctx)
	if err != nil {
		return TreeState{}, err
	}
	entries, err := tree.Entries(ctx)
	if err != nil {
		return TreeState{}, err
	}
	return TreeState{LeafID: leaf, Entries: entries}, nil
}

// MoveTo moves to a session tree entry and updates the in-memory agent context.
func (h *Harness) MoveTo(ctx context.Context, id session.EntryID, summary *session.BranchSummary) error {
	tree, ok := h.session.(session.Tree)
	if !ok {
		return fmt.Errorf("session %q does not support tree navigation", h.session.ID())
	}
	if err := tree.MoveTo(ctx, id, summary); err != nil {
		return err
	}
	built, err := h.session.BuildContext(ctx)
	if err != nil {
		return err
	}
	h.agent.SetMessages(built.Messages)
	h.mu.Lock()
	h.seen = len(built.Messages)
	h.mu.Unlock()
	leaf, _ := tree.LeafID(ctx)
	h.publish(event.SessionTree{LeafID: string(leaf), Action: "move_to"})
	return nil
}

// Branch creates a new session branch when the backing session supports session.Tree.
func (h *Harness) Branch(ctx context.Context, at session.EntryID) (session.Session, error) {
	tree, ok := h.session.(session.Tree)
	if !ok {
		return nil, fmt.Errorf("session %q does not support tree navigation", h.session.ID())
	}
	branched, err := tree.Branch(ctx, at)
	if err != nil {
		return nil, err
	}
	h.publish(event.SessionTree{LeafID: string(at), Action: "branch"})
	return branched, nil
}

// Session returns the backing session.
func (h *Harness) Session() session.Session { return h.session }

func (h *Harness) handleEvent(ev event.Event) {
	ctx := context.Background()
	switch e := ev.(type) {
	case event.Usage:
		_, _ = h.session.AppendUsage(ctx, e.Usage, e.Cumulative)
	case event.AgentEnd:
		h.persistNewMessages(ctx, e.Messages)
	}
	h.publish(ev)
}

func (h *Harness) persistNewMessages(ctx context.Context, messages []message.Message) {
	h.mu.Lock()
	start := h.seen
	if start > len(messages) {
		start = 0
	}
	h.seen = len(messages)
	h.mu.Unlock()
	for _, msg := range messages[start:] {
		_, _ = h.session.AppendMessage(ctx, msg)
	}
}

func (h *Harness) publish(ev event.Event) {
	h.mu.Lock()
	subs := make([]func(event.Event), 0, len(h.subs))
	for _, fn := range h.subs {
		subs = append(subs, fn)
	}
	h.mu.Unlock()
	for _, fn := range subs {
		fn(ev)
	}
}

func (h *Harness) eventChan() (chan event.Event, func()) {
	ch := make(chan event.Event, 64)
	var once sync.Once
	var unsub func()
	unsubscribe := func() {
		once.Do(func() {
			if unsub != nil {
				unsub()
			}
		})
	}
	unsub = h.Subscribe(func(ev event.Event) {
		ch <- ev
		if ev.Type() == "agent_end" {
			unsubscribe()
			close(ch)
		}
	})
	return ch, unsubscribe
}

func copySkills(in []skill.Skill) []skill.Skill {
	return append([]skill.Skill(nil), in...)
}

func toolSpecs(registry tool.Registry) []tool.Spec {
	if registry == nil {
		return nil
	}
	return append([]tool.Spec(nil), registry.Specs()...)
}

func toolNames(registry tool.Registry) []string {
	if registry == nil {
		return nil
	}
	tools := registry.List()
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name())
	}
	return names
}
