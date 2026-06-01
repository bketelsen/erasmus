// Package tui provides a minimal terminal frontend over the harness.
package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"erasmus/packages/compact"
	"erasmus/packages/event"
	"erasmus/packages/harness"
	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/session"
	"erasmus/packages/swarm"

	"golang.org/x/term"
)

// SessionSummary is a TUI-safe durable session listing entry.
type SessionSummary struct {
	Path     string
	ID       string
	Updated  string
	Messages int
}

// SwarmServerSummary is a TUI-safe swarm server registry/status entry.
type SwarmServerSummary struct {
	Name      string
	Socket    string
	CWD       string
	Provider  string
	Model     string
	Status    string
	Reachable bool
	Error     string
}

// SwarmStatusSummary is a TUI-safe swarm server status snapshot.
type SwarmStatusSummary struct {
	PID      int
	Socket   string
	CWD      string
	Provider string
	Model    string
	Uptime   string
	Agents   []swarm.Snapshot
}

// App is a small line-oriented TUI MVP.
type App struct {
	Harness        *harness.Harness
	HarnessCleanup func()
	ListSessions   func(context.Context, string) ([]SessionSummary, error)
	OpenSession    func(context.Context, string) (*harness.Harness, func(), error)
	ApplyModel     func(context.Context, model.Model, string) error
	ListSwarms     func(context.Context) ([]SwarmServerSummary, error)
	SwarmStatus    func(context.Context, SwarmServerSummary) (SwarmStatusSummary, error)
	SwarmSend      func(context.Context, SwarmServerSummary, string, string) (SwarmStatusSummary, error)
	SwarmStop      func(context.Context, SwarmServerSummary, string) (SwarmStatusSummary, error)
	SwarmSpawn     func(context.Context, SwarmServerSummary, string) (SwarmStatusSummary, error)
	In             io.Reader
	Out            io.Writer
	Prompt         string
	Theme          string
}

// Run starts the line-oriented TUI loop.
func (a *App) Run(ctx context.Context) error {
	if a.Harness == nil {
		return fmt.Errorf("harness is required")
	}
	in := a.In
	if in == nil {
		in = strings.NewReader("")
	}
	out := a.Out
	if out == nil {
		out = io.Discard
	}
	if a.Prompt == "" && isTerminal(in) && isTerminal(out) {
		return a.runBubble(ctx, in, out)
	}
	prompt := a.Prompt
	if prompt == "" {
		prompt = "> "
	}
	defer func() {
		if a.HarnessCleanup != nil {
			a.HarnessCleanup()
		}
	}()
	fmt.Fprintln(out, "Erasmus TUI MVP. Type /quit to exit, /state for state.")
	scanner := bufio.NewScanner(in)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		fmt.Fprint(out, prompt)
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if handled, err := a.handleCommand(ctx, out, line); handled {
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
			}
			if line == "/quit" || line == "/exit" {
				return nil
			}
			continue
		}
		fmt.Fprintf(out, "you: %s\n", line)
		events, err := a.Harness.Prompt(ctx, line, harness.PromptOptions{})
		if err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
			continue
		}
		renderEvents(out, events)
		if err := a.Harness.Wait(ctx); err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
			continue
		}
		fmt.Fprintln(out)
	}
	return scanner.Err()
}

func isTerminal(r any) bool {
	file, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func (a *App) handleCommand(ctx context.Context, out io.Writer, line string) (bool, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 || fields[0] == "/noop" {
		return true, nil
	}
	switch fields[0] {
	case "/quit", "/exit":
		fmt.Fprintln(out, "bye")
		return true, nil
	case "/help":
		renderHelp(out)
		return true, nil
	case "/state", "/status":
		renderState(ctx, out, a.Harness)
		return true, nil
	case "/messages", "/transcript":
		limit := 10
		if len(fields) > 1 {
			if _, err := fmt.Sscanf(fields[1], "%d", &limit); err != nil || limit < 1 {
				return true, fmt.Errorf("usage: %s [count]", fields[0])
			}
		}
		renderMessages(ctx, out, a.Harness, limit)
		return true, nil
	case "/sessions":
		dir := ""
		if len(fields) > 1 {
			dir = fields[1]
		}
		return true, a.renderSessions(ctx, out, dir)
	case "/open":
		if len(fields) < 2 {
			return true, fmt.Errorf("usage: /open <session-path>")
		}
		return true, a.openSession(ctx, out, fields[1])
	case "/tree":
		renderTree(ctx, out, a.Harness)
		return true, nil
	case "/compact":
		result, err := a.Harness.Compact(ctx, compact.Options{KeepTail: 4})
		if err != nil {
			return true, err
		}
		fmt.Fprintf(out, "compacted: %s\n", result.Summary)
		return true, nil
	case "/model":
		state := a.Harness.State(ctx)
		fmt.Fprintf(out, "model=%s/%s reasoning=%s\n", state.Agent.Model.Provider, state.Agent.Model.ID, state.Agent.Reasoning)
		return true, nil
	case "/move":
		if len(fields) < 2 {
			return true, fmt.Errorf("usage: /move <entry-id> [summary]")
		}
		var summary *session.BranchSummary
		if len(fields) > 2 {
			summary = &session.BranchSummary{Summary: strings.Join(fields[2:], " ")}
		}
		if err := a.Harness.MoveTo(ctx, session.EntryID(fields[1]), summary); err != nil {
			return true, err
		}
		renderTree(ctx, out, a.Harness)
		return true, nil
	case "/branch":
		if len(fields) < 2 {
			return true, fmt.Errorf("usage: /branch <entry-id>")
		}
		branched, err := a.Harness.Branch(ctx, session.EntryID(fields[1]))
		if err != nil {
			return true, err
		}
		fmt.Fprintf(out, "branch session=%s\n", branched.ID())
		return true, nil
	}
	if strings.HasPrefix(line, "/") {
		return true, fmt.Errorf("unknown command %q", fields[0])
	}
	return false, nil
}

type slashCommand struct {
	Name        string
	Usage       string
	Description string
}

func slashCommands() []slashCommand {
	return []slashCommand{
		{Name: "/help", Usage: "/help", Description: "show command help"},
		{Name: "/status", Usage: "/status", Description: "show runtime status"},
		{Name: "/state", Usage: "/state", Description: "show runtime status"},
		{Name: "/model", Usage: "/model", Description: "show model and reasoning"},
		{Name: "/messages", Usage: "/messages [count]", Description: "show recent transcript"},
		{Name: "/transcript", Usage: "/transcript [count]", Description: "show recent transcript"},
		{Name: "/sessions", Usage: "/sessions [dir]", Description: "list durable JSONL sessions"},
		{Name: "/open", Usage: "/open <path>", Description: "switch to a durable JSONL session"},
		{Name: "/tree", Usage: "/tree", Description: "show session tree"},
		{Name: "/move", Usage: "/move <id> [summary]", Description: "move to tree entry"},
		{Name: "/branch", Usage: "/branch <id>", Description: "create branch session at entry"},
		{Name: "/compact", Usage: "/compact", Description: "compact transcript"},
		{Name: "/quit", Usage: "/quit", Description: "exit"},
		{Name: "/exit", Usage: "/exit", Description: "exit"},
	}
}

func (a *App) renderSessions(ctx context.Context, out io.Writer, dir string) error {
	if a.ListSessions == nil {
		return fmt.Errorf("session listing is not configured")
	}
	entries, err := a.ListSessions(ctx, dir)
	if err != nil {
		return err
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

func (a *App) openSession(ctx context.Context, out io.Writer, path string) error {
	if a.OpenSession == nil {
		return fmt.Errorf("session opening is not configured")
	}
	state := a.Harness.State(ctx)
	if state.Agent.IsStreaming {
		return fmt.Errorf("cannot open session while streaming")
	}
	h, cleanup, err := a.OpenSession(ctx, path)
	if err != nil {
		return err
	}
	if a.HarnessCleanup != nil {
		a.HarnessCleanup()
	}
	a.Harness = h
	a.HarnessCleanup = cleanup
	meta, _ := h.Session().Metadata(ctx)
	fmt.Fprintf(out, "opened session=%s path=%s messages=%d\n", meta.ID, path, len(h.State(ctx).Agent.Messages))
	return nil
}

func renderHelp(out io.Writer) {
	fmt.Fprintln(out, "commands:")
	for _, cmd := range slashCommands() {
		fmt.Fprintf(out, "  %-22s %s\n", cmd.Usage, cmd.Description)
	}
}

func renderState(ctx context.Context, out io.Writer, h *harness.Harness) {
	state := h.State(ctx)
	tools := 0
	if state.Agent.Tools != nil {
		tools = len(state.Agent.Tools.List())
	}
	fmt.Fprintf(out, "session: %s\n", state.Session.ID)
	if state.Session.CWD != "" {
		fmt.Fprintf(out, "cwd: %s\n", state.Session.CWD)
	}
	fmt.Fprintf(out, "model: %s/%s\n", state.Agent.Model.Provider, state.Agent.Model.ID)
	if state.Agent.Reasoning != "" {
		fmt.Fprintf(out, "reasoning: %s\n", state.Agent.Reasoning)
	}
	fmt.Fprintf(out, "messages: %d\n", len(state.Agent.Messages))
	fmt.Fprintf(out, "streaming: %v\n", state.Agent.IsStreaming)
	fmt.Fprintf(out, "tools: %d\n", tools)
	fmt.Fprintf(out, "skills: %d\n", len(state.Skills))
	if state.Agent.ErrorMessage != "" {
		fmt.Fprintf(out, "error: %s\n", state.Agent.ErrorMessage)
	}
}

func renderMessages(ctx context.Context, out io.Writer, h *harness.Harness, limit int) {
	state := h.State(ctx)
	messages := state.Agent.Messages
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	if len(messages) == 0 {
		fmt.Fprintln(out, "no messages")
		return
	}
	for i, msg := range messages {
		fmt.Fprintf(out, "[%d] %s: %s\n", i+1, msg.Role, messageText(msg))
	}
}

func renderTree(ctx context.Context, out io.Writer, h *harness.Harness) {
	tree, err := h.Tree(ctx)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return
	}
	fmt.Fprintf(out, "leaf=%s entries=%d\n", tree.LeafID, len(tree.Entries))
	for _, e := range tree.Entries {
		marker := " "
		if e.ID == tree.LeafID {
			marker = "*"
		}
		when := ""
		if !e.Time.IsZero() {
			when = e.Time.Format("15:04:05")
		}
		fmt.Fprintf(out, "%s id=%s parent=%s type=%s time=%s\n", marker, e.ID, e.Parent, e.Type, when)
	}
}

func renderEvents(out io.Writer, events <-chan event.Event) {
	assistantOpen := false
	for ev := range events {
		switch e := ev.(type) {
		case event.MessageStart:
			if e.Message.Role == message.RoleAssistant && !assistantOpen {
				fmt.Fprint(out, "assistant: ")
				assistantOpen = true
			}
		case event.MessageDelta:
			if !assistantOpen {
				fmt.Fprint(out, "assistant: ")
				assistantOpen = true
			}
			fmt.Fprint(out, e.Text)
		case event.ToolExecutionStart:
			if assistantOpen {
				fmt.Fprintln(out)
				assistantOpen = false
			}
			fmt.Fprintf(out, "tool %s starting\n", e.Name)
		case event.ToolExecutionProgress:
			if e.Text != "" {
				fmt.Fprintf(out, "tool %s: %s\n", e.ID, e.Text)
			}
		case event.ToolExecutionEnd:
			fmt.Fprintf(out, "tool %s done\n", e.Name)
		case event.SessionCompact:
			if assistantOpen {
				fmt.Fprintln(out)
				assistantOpen = false
			}
			fmt.Fprintf(out, "compact: %s\n", e.Summary)
		}
	}
}

func messageText(msg message.Message) string {
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
	text := strings.Join(parts, " ")
	text = strings.ReplaceAll(text, "\n", " ")
	if len(text) > 200 {
		text = text[:197] + "..."
	}
	return text
}
