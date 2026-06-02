package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/bketelsen/erasmus/packages/event"
	"github.com/bketelsen/erasmus/packages/harness"
	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/prompt"
	"github.com/bketelsen/erasmus/packages/provider"
	"github.com/bketelsen/erasmus/packages/sandbox"
	"github.com/bketelsen/erasmus/packages/session"
	"github.com/bketelsen/erasmus/packages/session/jsonl"
	"github.com/bketelsen/erasmus/packages/tools"
)

type options struct {
	Inbox    string
	Out      string
	Repo     string
	State    string
	Watch    bool
	Once     bool
	Interval time.Duration
}

type task struct {
	ID      string
	Path    string
	Content string
}

type taskStatus struct {
	ID          string    `json:"id"`
	TaskPath    string    `json:"task_path"`
	SessionPath string    `json:"session_path"`
	SummaryPath string    `json:"summary_path"`
	EventsPath  string    `json:"events_path"`
	Status      string    `json:"status"`
	Error       string    `json:"error,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
}

type eventRecord struct {
	Type  string          `json:"type"`
	Event json.RawMessage `json:"event"`
}

func main() {
	opts := parseFlags(os.Args[1:])
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, opts); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func parseFlags(args []string) options {
	fs := flag.NewFlagSet("task-daemon", flag.ExitOnError)
	opts := options{Inbox: "inbox", Out: "out", Repo: ".", State: filepath.Join(".erasmus", "task-daemon"), Once: true, Interval: 30 * time.Second}
	fs.StringVar(&opts.Inbox, "inbox", opts.Inbox, "directory containing Markdown task files")
	fs.StringVar(&opts.Out, "out", opts.Out, "directory for task outputs")
	fs.StringVar(&opts.Repo, "repo", opts.Repo, "repository/sandbox root for harness tools")
	fs.StringVar(&opts.State, "state", opts.State, "daemon state directory")
	fs.BoolVar(&opts.Watch, "watch", false, "continue polling the inbox")
	fs.BoolVar(&opts.Once, "once", true, "process pending tasks once and exit")
	fs.DurationVar(&opts.Interval, "interval", opts.Interval, "poll interval for --watch")
	_ = fs.Parse(args)
	if opts.Watch {
		opts.Once = false
	}
	return opts
}

func run(ctx context.Context, opts options) error {
	if err := ensureDirs(opts.Inbox, opts.Out, opts.State); err != nil {
		return err
	}
	for {
		processed, err := processPending(ctx, opts)
		if err != nil {
			return err
		}
		if opts.Once {
			log.Printf("processed %d task(s)", processed)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(opts.Interval):
		}
	}
}

func ensureDirs(dirs ...string) error {
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func processPending(ctx context.Context, opts options) (int, error) {
	tasks, err := discoverTasks(opts.Inbox)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, t := range tasks {
		if ctx.Err() != nil {
			return processed, ctx.Err()
		}
		if alreadyDone(opts.Out, t.ID) {
			continue
		}
		if err := processTask(ctx, opts, t); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func discoverTasks(inbox string) ([]task, error) {
	matches, err := filepath.Glob(filepath.Join(inbox, "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	out := make([]task, 0, len(matches))
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, task{ID: taskID(path), Path: path, Content: string(data)})
	}
	return out, nil
}

func taskID(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	clean := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, base)
	clean = strings.Trim(clean, "-")
	if clean != "" {
		return clean
	}
	sum := sha1.Sum([]byte(path))
	return hex.EncodeToString(sum[:])[:12]
}

func alreadyDone(outRoot, id string) bool {
	data, err := os.ReadFile(filepath.Join(outRoot, id, "status.json"))
	if err != nil {
		return false
	}
	var status taskStatus
	return json.Unmarshal(data, &status) == nil && status.Status == "done"
}

func processTask(ctx context.Context, opts options, t task) error {
	runDir := filepath.Join(opts.Out, t.ID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	sessionPath := filepath.Join(runDir, "session.jsonl")
	eventsPath := filepath.Join(runDir, "events.jsonl")
	summaryPath := filepath.Join(runDir, "summary.md")
	statusPath := filepath.Join(runDir, "status.json")

	status := taskStatus{ID: t.ID, TaskPath: t.Path, SessionPath: sessionPath, SummaryPath: summaryPath, EventsPath: eventsPath, Status: "running", StartedAt: time.Now().UTC()}
	if err := writeJSON(statusPath, status); err != nil {
		return err
	}

	sess, err := jsonl.Open(sessionPath, session.Metadata{ID: t.ID, CWD: opts.Repo})
	if err != nil {
		return err
	}
	defer sess.Close(ctx)

	policy, err := sandbox.New(opts.Repo)
	if err != nil {
		return err
	}
	h, err := harness.New(ctx, harness.Config{
		Session:   sess,
		Stream:    daemonFakeStream(),
		Model:     model.Model{Provider: "fake", ID: "task-daemon", DisplayName: "task daemon fake model", ContextWindow: 32000, MaxOutput: 2048},
		Reasoning: "low",
		Prompt:    prompt.StaticBuilder{Base: "You are an autonomous Erasmus task daemon. Process one inbox task, use tools when useful, and finish with a concise Markdown summary."},
		Tools:     tools.DefaultRegistry(policy),
		MaxSteps:  4,
	})
	if err != nil {
		return err
	}
	if _, err := h.SavePoint(ctx, "task-start", map[string]string{"task": t.ID, "path": t.Path}); err != nil {
		return err
	}

	eventFile, err := os.Create(eventsPath)
	if err != nil {
		return err
	}
	defer eventFile.Close()

	promptText := fmt.Sprintf("Task file: %s\n\n%s", t.Path, t.Content)
	events, err := h.Prompt(ctx, promptText, harness.PromptOptions{})
	if err != nil {
		return err
	}
	var summary strings.Builder
	for ev := range events {
		if err := writeEvent(eventFile, ev); err != nil {
			return err
		}
		if delta, ok := ev.(event.MessageDelta); ok {
			summary.WriteString(delta.Text)
		}
	}
	if err := h.Wait(ctx); err != nil {
		status.Status = "error"
		status.Error = err.Error()
		status.FinishedAt = time.Now().UTC()
		_ = writeJSON(statusPath, status)
		return err
	}
	if strings.TrimSpace(summary.String()) == "" {
		summary.WriteString("Task completed without assistant text.\n")
	}
	if err := os.WriteFile(summaryPath, []byte(summary.String()), 0o644); err != nil {
		return err
	}
	status.Status = "done"
	status.FinishedAt = time.Now().UTC()
	return writeJSON(statusPath, status)
}

func writeEvent(f *os.File, ev event.Event) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	record, err := json.Marshal(eventRecord{Type: ev.Type(), Event: payload})
	if err != nil {
		return err
	}
	_, err = f.Write(append(record, '\n'))
	return err
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func daemonFakeStream() provider.StreamFunc {
	return func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		text := "Processed task.\n\n" + summarizeTask(userText(req))
		return streamEvents(provider.MessageStart{MessageID: "task-daemon-fake"}, provider.TextDelta{Text: text}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}
}

func summarizeTask(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if line != "" && !strings.HasPrefix(line, "Task file:") {
			return "Summary: " + line + "\n"
		}
	}
	return "Summary: no task body was provided.\n"
}

func userText(req provider.Request) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role != message.RoleUser {
			continue
		}
		for _, c := range req.Messages[i].Content {
			if text, ok := c.(message.Text); ok {
				return text.Text
			}
		}
	}
	return ""
}

func streamEvents(events ...provider.Event) <-chan provider.Event {
	ch := make(chan provider.Event, len(events))
	for _, ev := range events {
		ch <- ev
	}
	close(ch)
	return ch
}
