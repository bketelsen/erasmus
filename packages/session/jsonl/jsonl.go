// Package jsonl provides a JSON-lines session backend.
package jsonl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/session"
)

// Session persists session entries as newline-delimited JSON.
type Session struct {
	mu     sync.Mutex
	id     string
	path   string
	file   *os.File
	closed bool
	meta   session.Metadata
	leafID session.EntryID
	nextID int
}

type entry struct {
	Type       string              `json:"type"`
	ID         session.EntryID     `json:"id,omitempty"`
	Parent     session.EntryID     `json:"parent,omitempty"`
	Leaf       session.EntryID     `json:"leaf,omitempty"`
	Time       time.Time           `json:"time,omitempty"`
	Meta       *session.Metadata   `json:"meta,omitempty"`
	Message    *wireMessage        `json:"message,omitempty"`
	Usage      *model.Usage        `json:"usage,omitempty"`
	Cumulative *model.Usage        `json:"cumulative,omitempty"`
	Provider   string              `json:"provider,omitempty"`
	ModelID    string              `json:"model,omitempty"`
	Reasoning  string              `json:"reasoning,omitempty"`
	Names      []string            `json:"names,omitempty"`
	Compact    *session.Compaction `json:"compaction,omitempty"`
	CustomType string              `json:"custom_type,omitempty"`
	Data       json.RawMessage     `json:"data,omitempty"`
}

type wireMessage struct {
	ID      string            `json:"id,omitempty"`
	Role    message.Role      `json:"role"`
	Content []wireContent     `json:"content,omitempty"`
	Time    time.Time         `json:"time,omitempty"`
	Meta    map[string]string `json:"meta,omitempty"`
}

type wireContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	MimeType  string          `json:"mime_type,omitempty"`
	Data      []byte          `json:"data,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Content   []wireContent   `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Summary   string          `json:"summary,omitempty"`
	Encrypted string          `json:"encrypted,omitempty"`
	Kind      string          `json:"kind,omitempty"`
}

// Open opens or creates a JSONL session file.
func Open(path string, meta session.Metadata) (*Session, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	s := &Session{path: path, file: file, meta: meta}
	if s.meta.ID == "" {
		s.meta.ID = filepath.Base(path)
	}
	s.id = s.meta.ID
	if err := s.load(); err != nil {
		_ = file.Close()
		return nil, err
	}
	if s.meta.Created.IsZero() {
		now := time.Now()
		s.meta.Created = now
		s.meta.Updated = now
		if err := s.writeEntry(entry{Type: "meta", Time: now, Meta: &s.meta}); err != nil {
			_ = file.Close()
			return nil, err
		}
	}
	return s, nil
}

// ID returns the session ID.
func (s *Session) ID() string { return s.id }

// Metadata returns metadata.
func (s *Session) Metadata(ctx context.Context) (session.Metadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return session.Metadata{}, err
	}
	return s.meta, nil
}

// BuildContext reconstructs session context from the log.
func (s *Session) BuildContext(ctx context.Context) (session.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return session.Context{}, err
	}
	entries, err := readEntries(s.path)
	if err != nil {
		return session.Context{}, err
	}
	var out session.Context
	for _, e := range activeEntries(entries, leafFromEntries(entries)) {
		switch e.Type {
		case "message":
			if e.Message != nil {
				out.Messages = append(out.Messages, fromWireMessage(*e.Message))
			}
		case "usage":
			if e.Cumulative != nil {
				out.Usage = *e.Cumulative
			}
		case "model_change":
			out.Model = model.Model{Provider: e.Provider, ID: e.ModelID}
		case "reasoning_change":
			out.Reasoning = e.Reasoning
		case "active_tools_change":
			out.ActiveTools = append([]string(nil), e.Names...)
		case "compaction":
			if e.Compact != nil {
				out.Messages = []message.Message{{Role: message.RoleSystem, Content: []message.Content{message.Text{Text: e.Compact.Summary}}, Time: e.Compact.Created}}
			}
		}
	}
	return out, nil
}

// AppendMessage appends a message.
func (s *Session) AppendMessage(ctx context.Context, msg message.Message) (session.EntryID, error) {
	return s.append(ctx, entry{Type: "message", Message: ptr(toWireMessage(msg))})
}

// AppendUsage appends usage.
func (s *Session) AppendUsage(ctx context.Context, usage model.Usage, cumulative model.Usage) (session.EntryID, error) {
	return s.append(ctx, entry{Type: "usage", Usage: &usage, Cumulative: &cumulative})
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
	return s.append(ctx, entry{Type: "active_tools_change", Names: append([]string(nil), names...)})
}

// AppendCompaction appends compaction.
func (s *Session) AppendCompaction(ctx context.Context, c session.Compaction) (session.EntryID, error) {
	if c.Created.IsZero() {
		c.Created = time.Now()
	}
	return s.append(ctx, entry{Type: "compaction", Compact: &c})
}

// AppendCustom appends custom data.
func (s *Session) AppendCustom(ctx context.Context, typ string, data any) (session.EntryID, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return s.append(ctx, entry{Type: "custom", CustomType: typ, Data: raw})
}

// LeafID returns the active leaf entry.
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
	entries, err := readEntries(s.path)
	if err != nil {
		return err
	}
	if id != "" && !hasEntry(entries, id) {
		return fmt.Errorf("entry %q not found", id)
	}
	s.leafID = id
	if err := s.writeEntry(entry{Type: "leaf", Leaf: id, Time: time.Now()}); err != nil {
		return err
	}
	if summary != nil && summary.Summary != "" {
		raw, err := json.Marshal(*summary)
		if err != nil {
			return err
		}
		s.nextID++
		now := time.Now()
		e := entry{Type: "custom", ID: session.EntryID(fmt.Sprintf("%d", s.nextID)), Parent: s.leafID, Time: now, CustomType: "branch_summary", Data: raw}
		s.leafID = e.ID
		s.meta.Updated = now
		if err := s.writeEntry(e); err != nil {
			return err
		}
		return s.writeEntry(entry{Type: "leaf", Leaf: s.leafID, Time: now})
	}
	return nil
}

// Branch creates a new JSONL session copy whose active leaf is at the requested entry.
func (s *Session) Branch(ctx context.Context, at session.EntryID) (session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	entries, err := readEntries(s.path)
	if err != nil {
		return nil, err
	}
	if at != "" && !hasEntry(entries, at) {
		return nil, fmt.Errorf("entry %q not found", at)
	}
	branchPath := branchPath(s.path)
	if err := copyFile(s.path, branchPath); err != nil {
		return nil, err
	}
	child, err := Open(branchPath, session.Metadata{ID: s.id + "-branch", CWD: s.meta.CWD})
	if err != nil {
		return nil, err
	}
	if err := child.MoveTo(ctx, at, nil); err != nil {
		_ = child.Close(ctx)
		return nil, err
	}
	return child, nil
}

// Entries returns navigable entries.
func (s *Session) Entries(ctx context.Context) ([]session.TreeEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	entries, err := readEntries(s.path)
	if err != nil {
		return nil, err
	}
	out := make([]session.TreeEntry, 0, len(entries))
	for _, e := range entries {
		if e.ID == "" {
			continue
		}
		out = append(out, session.TreeEntry{ID: e.ID, Parent: e.Parent, Type: e.Type, Time: e.Time})
	}
	return out, nil
}

// Close closes the session file.
func (s *Session) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	s.closed = true
	return s.file.Close()
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
	s.leafID = e.ID
	s.meta.Updated = e.Time
	if err := s.writeEntry(e); err != nil {
		return "", err
	}
	return e.ID, s.writeEntry(entry{Type: "leaf", Leaf: s.leafID, Time: e.Time})
}

func (s *Session) writeEntry(e entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := s.file.Write(append(data, '\n')); err != nil {
		return err
	}
	return s.file.Sync()
}

func (s *Session) load() error {
	entries, err := readEntries(s.path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Meta != nil {
			s.meta = *e.Meta
			s.id = s.meta.ID
		}
		var n int
		if _, err := fmt.Sscanf(string(e.ID), "%d", &n); err == nil && n > s.nextID {
			s.nextID = n
		}
		if e.Type == "leaf" {
			s.leafID = e.Leaf
		} else if e.ID != "" && s.leafID == "" {
			s.leafID = e.ID
		}
		if !e.Time.IsZero() && e.Time.After(s.meta.Updated) {
			s.meta.Updated = e.Time
		}
	}
	return nil
}

func readEntries(path string) ([]entry, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var entries []entry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}

func leafFromEntries(entries []entry) session.EntryID {
	var leaf session.EntryID
	for _, e := range entries {
		if e.Type == "leaf" {
			leaf = e.Leaf
		} else if e.ID != "" && leaf == "" {
			leaf = e.ID
		}
	}
	return leaf
}

func activeEntries(entries []entry, leaf session.EntryID) []entry {
	if leaf == "" {
		var out []entry
		for _, e := range entries {
			if e.ID != "" {
				out = append(out, e)
			}
		}
		return out
	}
	byID := map[session.EntryID]entry{}
	for _, e := range entries {
		if e.ID != "" {
			byID[e.ID] = e
		}
	}
	var rev []entry
	for id := leaf; id != ""; {
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

func hasEntry(entries []entry, id session.EntryID) bool {
	for _, e := range entries {
		if e.ID == id {
			return true
		}
	}
	return false
}

func branchPath(path string) string {
	ext := filepath.Ext(path)
	base := path[:len(path)-len(ext)]
	if ext == "" {
		ext = ".jsonl"
	}
	return fmt.Sprintf("%s.branch-%d%s", base, time.Now().UnixNano(), ext)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
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

func toWireMessage(m message.Message) wireMessage {
	return wireMessage{ID: m.ID, Role: m.Role, Content: toWireContent(m.Content), Time: m.Time, Meta: m.Meta}
}

func fromWireMessage(m wireMessage) message.Message {
	return message.Message{ID: m.ID, Role: m.Role, Content: fromWireContent(m.Content), Time: m.Time, Meta: m.Meta}
}

func toWireContent(in []message.Content) []wireContent {
	out := make([]wireContent, 0, len(in))
	for _, c := range in {
		switch v := c.(type) {
		case message.Text:
			out = append(out, wireContent{Type: "text", Text: v.Text})
		case message.Image:
			out = append(out, wireContent{Type: "image", MimeType: v.MimeType, Data: v.Data})
		case message.ToolCall:
			out = append(out, wireContent{Type: "tool_call", ID: v.ID, Name: v.Name, Arguments: v.Arguments})
		case message.ToolResult:
			out = append(out, wireContent{Type: "tool_result", CallID: v.CallID, Content: toWireContent(v.Content), IsError: v.IsError})
		case message.Reasoning:
			out = append(out, wireContent{Type: "reasoning", ID: v.ID, Summary: v.Summary, Encrypted: v.Encrypted})
		case message.Custom:
			out = append(out, wireContent{Type: "custom", Kind: v.Kind, Data: v.Data})
		}
	}
	return out
}

func fromWireContent(in []wireContent) []message.Content {
	out := make([]message.Content, 0, len(in))
	for _, c := range in {
		switch c.Type {
		case "text":
			out = append(out, message.Text{Text: c.Text})
		case "image":
			out = append(out, message.Image{MimeType: c.MimeType, Data: c.Data})
		case "tool_call":
			out = append(out, message.ToolCall{ID: c.ID, Name: c.Name, Arguments: c.Arguments})
		case "tool_result":
			out = append(out, message.ToolResult{CallID: c.CallID, Content: fromWireContent(c.Content), IsError: c.IsError})
		case "reasoning":
			out = append(out, message.Reasoning{ID: c.ID, Summary: c.Summary, Encrypted: c.Encrypted})
		case "custom":
			out = append(out, message.Custom{Kind: c.Kind, Data: c.Data})
		}
	}
	return out
}

func ptr[T any](v T) *T { return &v }

var _ session.Session = (*Session)(nil)
var _ session.Tree = (*Session)(nil)
