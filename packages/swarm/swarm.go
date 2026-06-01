// Package swarm supervises background harness runtimes.
package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bketelsen/erasmus/packages/event"
	"github.com/bketelsen/erasmus/packages/harness"
)

// Factory creates a harness for a spawned swarm agent.
type Factory func(context.Context, SpawnRequest) (*harness.Harness, error)

// Config configures a Swarm.
type Config struct {
	Factory     Factory
	EventLogDir string
}

// SpawnRequest describes a background agent to create.
type SpawnRequest struct {
	ID           string `json:"id,omitempty"`
	Task         string `json:"task"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	CWD          string `json:"cwd,omitempty"`
	SessionScope string `json:"session_scope,omitempty"`
}

// Snapshot is a listable swarm agent summary.
type Snapshot struct {
	ID            string    `json:"id"`
	Task          string    `json:"task,omitempty"`
	SessionID     string    `json:"session_id,omitempty"`
	CWD           string    `json:"cwd,omitempty"`
	Provider      string    `json:"provider,omitempty"`
	Model         string    `json:"model,omitempty"`
	Reasoning     string    `json:"reasoning,omitempty"`
	State         string    `json:"state,omitempty"`
	Running       bool      `json:"running"`
	Created       time.Time `json:"created,omitempty"`
	Updated       time.Time `json:"updated,omitempty"`
	Messages      int       `json:"messages,omitempty"`
	PendingTools  int       `json:"pending_tools,omitempty"`
	Events        int       `json:"events,omitempty"`
	LastEventType string    `json:"last_event_type,omitempty"`
	LastEventAt   time.Time `json:"last_event_at,omitempty"`
	EventLog      string    `json:"event_log,omitempty"`
	Error         string    `json:"error,omitempty"`
}

// Agent is a supervised background harness.
type Agent struct {
	id      string
	task    string
	cwd     string
	created time.Time
	logPath string

	mu            sync.Mutex
	harness       *harness.Harness
	events        []event.Event
	running       bool
	err           string
	lastEventType string
	lastEventAt   time.Time
}

// Swarm owns a set of background harness agents.
type Swarm struct {
	mu          sync.Mutex
	factory     Factory
	eventLogDir string
	agents      map[string]*Agent
	nextID      int
}

// New creates a Swarm.
func New(cfg Config) (*Swarm, error) {
	if cfg.Factory == nil {
		return nil, fmt.Errorf("swarm factory is required")
	}
	return &Swarm{factory: cfg.Factory, eventLogDir: cfg.EventLogDir, agents: map[string]*Agent{}}, nil
}

// Spawn creates an agent and starts its initial task if provided.
func (s *Swarm) Spawn(ctx context.Context, req SpawnRequest) (*Agent, error) {
	if req.ID == "" {
		req.ID = s.newID()
	}
	h, err := s.factory(ctx, req)
	if err != nil {
		return nil, err
	}
	logPath, err := s.eventLogPath(req.ID)
	if err != nil {
		return nil, err
	}
	a := &Agent{id: req.ID, task: req.Task, cwd: req.CWD, created: time.Now(), logPath: logPath, harness: h}
	h.Subscribe(func(ev event.Event) { a.record(ev) })

	s.mu.Lock()
	if _, ok := s.agents[req.ID]; ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("swarm agent %q already exists", req.ID)
	}
	s.agents[req.ID] = a
	s.mu.Unlock()

	if req.Task != "" {
		if err := a.Prompt(ctx, req.Task); err != nil {
			return nil, err
		}
	}
	return a, nil
}

// Send prompts an existing agent.
func (s *Swarm) Send(ctx context.Context, id, text string) error {
	a, err := s.get(id)
	if err != nil {
		return err
	}
	return a.Prompt(ctx, text)
}

// Stop aborts an existing agent's active run.
func (s *Swarm) Stop(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a, err := s.get(id)
	if err != nil {
		return err
	}
	return a.Abort(ctx)
}

// Resume returns an existing in-process agent.
func (s *Swarm) Resume(ctx context.Context, id string) (*Agent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.get(id)
}

// List returns snapshots for all agents.
func (s *Swarm) List(ctx context.Context) ([]Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	agents := make([]*Agent, 0, len(s.agents))
	for _, a := range s.agents {
		agents = append(agents, a)
	}
	s.mu.Unlock()
	out := make([]Snapshot, 0, len(agents))
	for _, a := range agents {
		out = append(out, a.Snapshot(ctx))
	}
	return out, nil
}

// ID returns the agent ID.
func (a *Agent) ID() string { return a.id }

// Harness returns the supervised harness.
func (a *Agent) Harness() *harness.Harness { return a.harness }

// Prompt starts a prompt on the agent harness.
func (a *Agent) Prompt(ctx context.Context, text string) error {
	a.setRunning(true)
	events, err := a.harness.Prompt(ctx, text, harness.PromptOptions{})
	if err != nil {
		a.setError(err)
		return err
	}
	go a.drain(events)
	return nil
}

// Abort cancels the active run.
func (a *Agent) Abort(ctx context.Context) error {
	return a.harness.Abort(ctx)
}

// Wait waits for the active run.
func (a *Agent) Wait(ctx context.Context) error {
	err := a.harness.Wait(ctx)
	if err != nil {
		a.setError(err)
	} else {
		a.setRunning(false)
	}
	return err
}

// Events returns a copy of the agent event log.
func (a *Agent) Events() []event.Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]event.Event(nil), a.events...)
}

// Snapshot returns a summary of the agent.
func (a *Agent) Snapshot(ctx context.Context) Snapshot {
	a.mu.Lock()
	id := a.id
	task := a.task
	cwd := a.cwd
	created := a.created
	logPath := a.logPath
	running := a.running
	errText := a.err
	events := len(a.events)
	lastEventType := a.lastEventType
	lastEventAt := a.lastEventAt
	a.mu.Unlock()

	meta, _ := a.harness.Session().Metadata(ctx)
	state := a.harness.State(ctx)
	agentState := "settled"
	if running || state.Agent.IsStreaming {
		agentState = "running"
	}
	if errText != "" || state.Agent.ErrorMessage != "" {
		agentState = "error"
		if errText == "" {
			errText = state.Agent.ErrorMessage
		}
	}
	if cwd == "" {
		cwd = meta.CWD
	}
	return Snapshot{
		ID:            id,
		Task:          task,
		SessionID:     meta.ID,
		CWD:           cwd,
		Provider:      state.Agent.Model.Provider,
		Model:         state.Agent.Model.ID,
		Reasoning:     state.Agent.Reasoning,
		State:         agentState,
		Running:       running || state.Agent.IsStreaming,
		Created:       created,
		Updated:       meta.Updated,
		Messages:      len(state.Agent.Messages),
		PendingTools:  len(state.Agent.PendingToolCalls),
		Events:        events,
		LastEventType: lastEventType,
		LastEventAt:   lastEventAt,
		EventLog:      logPath,
		Error:         errText,
	}
}

func (s *Swarm) newID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	return fmt.Sprintf("swarm-%d", s.nextID)
}

func (s *Swarm) get(id string) (*Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.agents[id]
	if !ok {
		return nil, fmt.Errorf("swarm agent %q not found", id)
	}
	return a, nil
}

func (s *Swarm) eventLogPath(id string) (string, error) {
	if s.eventLogDir == "" {
		return "", nil
	}
	if err := os.MkdirAll(s.eventLogDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(s.eventLogDir, id+".events.jsonl"), nil
}

func (a *Agent) drain(events <-chan event.Event) {
	for range events {
	}
	a.setRunning(false)
}

func (a *Agent) record(ev event.Event) {
	a.mu.Lock()
	a.events = append(a.events, ev)
	a.lastEventType = ev.Type()
	a.lastEventAt = time.Now()
	if ev.Type() == "agent_end" {
		a.running = false
	}
	logPath := a.logPath
	a.mu.Unlock()
	if logPath != "" {
		_ = appendEventLog(logPath, ev)
	}
}

func appendEventLog(path string, ev event.Event) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := json.Marshal(struct {
		Time  time.Time   `json:"time"`
		Type  string      `json:"type"`
		Event event.Event `json:"event"`
	}{Time: time.Now(), Type: ev.Type(), Event: ev})
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func (a *Agent) setRunning(running bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.running = running
	if running {
		a.err = ""
	}
}

func (a *Agent) setError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		a.err = err.Error()
	}
	a.running = false
}
