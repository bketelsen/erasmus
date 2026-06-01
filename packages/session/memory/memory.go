// Package memory provides an in-memory session backend for tests and embedding.
package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/session"
)

// Session is an in-memory implementation of session.Session.
type Session struct {
	mu      sync.Mutex
	id      string
	meta    session.Metadata
	entries []entry
	leafID  session.EntryID
	closed  bool
	nextID  int
}

type entry struct {
	ID         session.EntryID
	Parent     session.EntryID
	Time       time.Time
	Type       string
	Message    *message.Message
	Usage      model.Usage
	Cumulative model.Usage
	Provider   string
	ModelID    string
	Reasoning  string
	Tools      []string
	Compact    *session.Compaction
	CustomType string
	CustomData any
}

// New creates a memory session with id.
func New(id string) *Session {
	if id == "" {
		id = fmt.Sprintf("mem-%d", time.Now().UnixNano())
	}
	now := time.Now()
	return &Session{id: id, meta: session.Metadata{ID: id, Created: now, Updated: now}}
}

// ID returns the session ID.
func (s *Session) ID() string { return s.id }

// Metadata returns session metadata.
func (s *Session) Metadata(ctx context.Context) (session.Metadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return session.Metadata{}, err
	}
	return s.meta, nil
}

// BuildContext reconstructs context from entries.
func (s *Session) BuildContext(ctx context.Context) (session.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return session.Context{}, err
	}
	var out session.Context
	for _, e := range s.activeEntries() {
		switch e.Type {
		case "message":
			if e.Message != nil {
				out.Messages = append(out.Messages, *e.Message)
			}
		case "usage":
			out.Usage = e.Cumulative
		case "model_change":
			out.Model = model.Model{Provider: e.Provider, ID: e.ModelID}
		case "reasoning_change":
			out.Reasoning = e.Reasoning
		case "active_tools_change":
			out.ActiveTools = append([]string(nil), e.Tools...)
		case "compaction":
			if e.Compact != nil {
				out.Messages = []message.Message{{Role: message.RoleSystem, Content: []message.Content{message.Text{Text: e.Compact.Summary}}, Time: e.Compact.Created}}
			}
		}
	}
	return out, nil
}

// AppendMessage appends a message entry.
func (s *Session) AppendMessage(ctx context.Context, msg message.Message) (session.EntryID, error) {
	m := msg
	return s.append(ctx, entry{Type: "message", Message: &m})
}

// AppendUsage appends usage.
func (s *Session) AppendUsage(ctx context.Context, usage model.Usage, cumulative model.Usage) (session.EntryID, error) {
	return s.append(ctx, entry{Type: "usage", Usage: usage, Cumulative: cumulative})
}

// AppendModelChange appends a model change.
func (s *Session) AppendModelChange(ctx context.Context, provider, modelID string) (session.EntryID, error) {
	return s.append(ctx, entry{Type: "model_change", Provider: provider, ModelID: modelID})
}

// AppendReasoningChange appends a reasoning change.
func (s *Session) AppendReasoningChange(ctx context.Context, level string) (session.EntryID, error) {
	return s.append(ctx, entry{Type: "reasoning_change", Reasoning: level})
}

// AppendActiveToolsChange appends active tools.
func (s *Session) AppendActiveToolsChange(ctx context.Context, names []string) (session.EntryID, error) {
	return s.append(ctx, entry{Type: "active_tools_change", Tools: append([]string(nil), names...)})
}

// AppendCompaction appends a compaction.
func (s *Session) AppendCompaction(ctx context.Context, c session.Compaction) (session.EntryID, error) {
	if c.Created.IsZero() {
		c.Created = time.Now()
	}
	return s.append(ctx, entry{Type: "compaction", Compact: &c})
}

// AppendCustom appends custom data.
func (s *Session) AppendCustom(ctx context.Context, typ string, data any) (session.EntryID, error) {
	return s.append(ctx, entry{Type: "custom", CustomType: typ, CustomData: data})
}

// LeafID returns the current active leaf entry.
func (s *Session) LeafID(ctx context.Context) (session.EntryID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return "", err
	}
	return s.leafID, nil
}

// MoveTo moves the active leaf to an existing entry, optionally appending a branch summary marker.
func (s *Session) MoveTo(ctx context.Context, id session.EntryID, summary *session.BranchSummary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return err
	}
	if id != "" && !s.hasEntry(id) {
		return fmt.Errorf("entry %q not found", id)
	}
	s.leafID = id
	if summary != nil && summary.Summary != "" {
		s.nextID++
		now := time.Now()
		e := entry{ID: session.EntryID(fmt.Sprintf("%d", s.nextID)), Parent: s.leafID, Time: now, Type: "custom", CustomType: "branch_summary", CustomData: *summary}
		s.entries = append(s.entries, e)
		s.leafID = e.ID
		s.meta.Updated = now
	}
	return nil
}

// Branch creates an in-memory session copy whose active leaf is at the requested entry.
func (s *Session) Branch(ctx context.Context, at session.EntryID) (session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	if at != "" && !s.hasEntry(at) {
		return nil, fmt.Errorf("entry %q not found", at)
	}
	child := New(s.id + "-branch")
	child.meta.CWD = s.meta.CWD
	child.entries = append([]entry(nil), s.entries...)
	child.leafID = at
	child.nextID = s.nextID
	return child, nil
}

// Entries returns navigable entries.
func (s *Session) Entries(ctx context.Context) ([]session.TreeEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	out := make([]session.TreeEntry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, session.TreeEntry{ID: e.ID, Parent: e.Parent, Type: e.Type, Time: e.Time})
	}
	return out, nil
}

// Close closes the session.
func (s *Session) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	s.closed = true
	return nil
}

func (s *Session) append(ctx context.Context, e entry) (session.EntryID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return "", err
	}
	s.nextID++
	e.ID = session.EntryID(fmt.Sprintf("%d", s.nextID))
	e.Parent = s.leafID
	e.Time = time.Now()
	s.entries = append(s.entries, e)
	s.leafID = e.ID
	s.meta.Updated = e.Time
	return e.ID, nil
}

func (s *Session) activeEntries() []entry {
	if s.leafID == "" {
		return append([]entry(nil), s.entries...)
	}
	byID := map[session.EntryID]entry{}
	for _, e := range s.entries {
		byID[e.ID] = e
	}
	var rev []entry
	for id := s.leafID; id != ""; {
		e, ok := byID[id]
		if !ok {
			break
		}
		rev = append(rev, e)
		id = e.Parent
	}
	out := make([]entry, 0, len(rev))
	for i := len(rev) - 1; i >= 0; i-- {
		out = append(out, rev[i])
	}
	return out
}

func (s *Session) hasEntry(id session.EntryID) bool {
	for _, e := range s.entries {
		if e.ID == id {
			return true
		}
	}
	return false
}

func (s *Session) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.closed {
		return fmt.Errorf("session %q is closed", s.id)
	}
	return nil
}

var _ session.Session = (*Session)(nil)
var _ session.Tree = (*Session)(nil)
