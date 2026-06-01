package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"erasmus/packages/message"
	"erasmus/packages/session"
	"erasmus/packages/session/jsonl"
)

// SessionListEntry is safe display metadata for a JSONL session file.
type SessionListEntry struct {
	Path     string
	ID       string
	Updated  string
	Messages int
}

// ListSessions lists JSONL sessions in dir. The default dir is the current workspace's XDG state session directory.
func ListSessions(ctx context.Context, dir string) ([]SessionListEntry, error) {
	if dir == "" {
		dir = DefaultTUISessionDir("")
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]SessionListEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		s, err := jsonl.Open(path, session.Metadata{})
		if err != nil {
			return nil, err
		}
		meta, _ := s.Metadata(ctx)
		built, _ := s.BuildContext(ctx)
		_ = s.Close(ctx)
		updated := ""
		if !meta.Updated.IsZero() {
			updated = meta.Updated.Format("2006-01-02T15:04:05Z07:00")
		}
		out = append(out, SessionListEntry{Path: path, ID: meta.ID, Updated: updated, Messages: len(built.Messages)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// DefaultTUISessionDir returns the default durable TUI session directory.
func DefaultTUISessionDir(cwd string) string {
	if dir := os.Getenv("ERASMUS_SESSION_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(xdgStateHome(), "erasmus", "sessions", stateProjectKey(cwd))
}

// PrintSessions writes a table of JSONL sessions.
func PrintSessions(ctx context.Context, out io.Writer, dir string) error {
	entries, err := ListSessions(ctx, dir)
	if err != nil {
		return err
	}
	if out == nil {
		out = io.Discard
	}
	if len(entries) == 0 {
		fmt.Fprintln(out, "no sessions found")
		return nil
	}
	for _, entry := range entries {
		fmt.Fprintf(out, "%s\tid=%s\tupdated=%s\tmessages=%d\n", entry.Path, entry.ID, entry.Updated, entry.Messages)
	}
	return nil
}

// PrintSessionShow writes a readable transcript for a JSONL session file.
func PrintSessionShow(ctx context.Context, out io.Writer, path string) error {
	if path == "" {
		return fmt.Errorf("session path is required")
	}
	if out == nil {
		out = io.Discard
	}
	s, err := jsonl.Open(path, session.Metadata{})
	if err != nil {
		return err
	}
	defer s.Close(ctx)
	built, err := s.BuildContext(ctx)
	if err != nil {
		return err
	}
	if len(built.Messages) == 0 {
		fmt.Fprintln(out, "no messages")
		return nil
	}
	for _, msg := range built.Messages {
		text := sessionMessageText(msg)
		if text == "" {
			continue
		}
		fmt.Fprintf(out, "%s: %s\n", msg.Role, text)
	}
	return nil
}

// PrintSessionTree writes tree metadata for a JSONL session file.
func PrintSessionTree(ctx context.Context, out io.Writer, path string) error {
	if path == "" {
		return fmt.Errorf("session path is required")
	}
	if out == nil {
		out = io.Discard
	}
	s, err := jsonl.Open(path, session.Metadata{})
	if err != nil {
		return err
	}
	defer s.Close(ctx)
	leaf, err := s.LeafID(ctx)
	if err != nil {
		return err
	}
	entries, err := s.Entries(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "leaf=%s entries=%d\n", leaf, len(entries))
	for _, entry := range entries {
		fmt.Fprintf(out, "- id=%s parent=%s type=%s\n", entry.ID, entry.Parent, entry.Type)
	}
	return nil
}

func sessionMessageText(msg message.Message) string {
	parts := make([]string, 0, len(msg.Content))
	for _, c := range msg.Content {
		switch v := c.(type) {
		case message.Text:
			parts = append(parts, v.Text)
		case message.ToolCall:
			parts = append(parts, "tool_call:"+v.Name)
		case message.ToolResult:
			parts = append(parts, "tool_result:"+v.CallID)
		case message.Image:
			parts = append(parts, "image:"+v.MimeType)
		case message.Reasoning:
			if v.Summary != "" {
				parts = append(parts, "reasoning:"+v.Summary)
			}
		}
	}
	return strings.Join(parts, " ")
}
