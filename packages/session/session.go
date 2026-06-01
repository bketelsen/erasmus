// Package session defines durable transcript backend interfaces.
package session

import (
	"context"
	"time"

	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
)

// EntryID identifies an entry in a session log.
type EntryID string

// Metadata describes a session.
type Metadata struct {
	ID      string    `json:"id"`
	CWD     string    `json:"cwd,omitempty"`
	Created time.Time `json:"created,omitempty"`
	Updated time.Time `json:"updated,omitempty"`
}

// Context is the transcript context reconstructed from a session.
type Context struct {
	Messages    []message.Message `json:"messages,omitempty"`
	Usage       model.Usage       `json:"usage,omitempty"`
	Model       model.Model       `json:"model,omitempty"`
	Reasoning   string            `json:"reasoning,omitempty"`
	ActiveTools []string          `json:"active_tools,omitempty"`
}

// Compaction records a compacted session summary.
type Compaction struct {
	Summary          string    `json:"summary"`
	FirstKeptEntryID EntryID   `json:"first_kept_entry_id,omitempty"`
	Created          time.Time `json:"created,omitempty"`
}

// TreeEntry is a navigable session log entry.
type TreeEntry struct {
	ID     EntryID   `json:"id"`
	Parent EntryID   `json:"parent,omitempty"`
	Type   string    `json:"type"`
	Time   time.Time `json:"time,omitempty"`
}

// BranchSummary describes an optional summary inserted when moving between branches.
type BranchSummary struct {
	Summary string `json:"summary"`
}

// Tree is the optional branching/navigation interface for session backends.
type Tree interface {
	LeafID(ctx context.Context) (EntryID, error)
	MoveTo(ctx context.Context, id EntryID, summary *BranchSummary) error
	Branch(ctx context.Context, at EntryID) (Session, error)
	Entries(ctx context.Context) ([]TreeEntry, error)
}

// Session is the common interface for persistent and in-memory session backends.
type Session interface {
	ID() string
	Metadata(ctx context.Context) (Metadata, error)
	BuildContext(ctx context.Context) (Context, error)
	AppendMessage(ctx context.Context, msg message.Message) (EntryID, error)
	AppendUsage(ctx context.Context, usage model.Usage, cumulative model.Usage) (EntryID, error)
	AppendModelChange(ctx context.Context, provider, model string) (EntryID, error)
	AppendReasoningChange(ctx context.Context, level string) (EntryID, error)
	AppendActiveToolsChange(ctx context.Context, names []string) (EntryID, error)
	AppendCompaction(ctx context.Context, c Compaction) (EntryID, error)
	AppendCustom(ctx context.Context, typ string, data any) (EntryID, error)
	Close(ctx context.Context) error
}
