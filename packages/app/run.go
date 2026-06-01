// Package app contains CLI-facing application services.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"erasmus/packages/auth"
	"erasmus/packages/config"
	"erasmus/packages/event"
	"erasmus/packages/extension"
	"erasmus/packages/harness"
	"erasmus/packages/message"
	"erasmus/packages/provider"
	"erasmus/packages/session"
	"erasmus/packages/session/jsonl"
	"erasmus/packages/session/memory"
	"erasmus/packages/tool"
)

// RunOptions configures a one-shot run.
type RunOptions struct {
	Prompt        string
	Out           io.Writer
	CWD           string
	Tools         []string
	SessionPath   string
	MemorySession bool
	ShowTools     bool
}

// RunConfigured runs a one-shot prompt through harness using resolved config/auth/provider wiring.
func RunConfigured(ctx context.Context, opts RunOptions, cfg config.Config, store auth.Store) error {
	if strings.TrimSpace(opts.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	if opts.CWD != "" {
		cfg.CWD = opts.CWD
	}
	if opts.Tools != nil {
		cfg.Tools = opts.Tools
	}
	skills, err := DiscoverSkills(ctx, cfg.CWD)
	if err != nil {
		return err
	}
	sess, err := runSession(cfg.CWD, opts)
	if err != nil {
		return err
	}
	defer sess.Close(ctx)
	extensions, err := StartConfiguredExtensionSet(ctx, cfg)
	if err != nil {
		return err
	}
	if extensions != nil {
		defer extensions.Close()
	}
	var extraTools tool.Registry
	if extensions != nil {
		extraTools = extensions.Tools()
	}
	resolved, err := ResolveHarnessConfig(ctx, ResolveOptions{Config: cfg, Session: sess, Auth: store, Skills: skills, ExtraTools: extraTools})
	if err != nil {
		return err
	}
	h, err := harness.New(ctx, resolved.Harness)
	if err != nil {
		return err
	}
	if extensions != nil {
		if err := applyExtensionHostActions(ctx, h, extensions.DrainHostActions()); err != nil {
			return err
		}
	}
	events, err := h.Prompt(ctx, opts.Prompt, harness.PromptOptions{})
	if err != nil {
		return err
	}
	for ev := range events {
		if extensions != nil {
			if err := extensions.PublishEvent(ctx, ev); err != nil {
				return err
			}
			if err := applyExtensionHostActions(ctx, h, extensions.DrainHostActions()); err != nil {
				return err
			}
		}
		renderRunEvent(out, ev, opts.ShowTools)
	}
	if err := h.Wait(ctx); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out)
	return nil
}

// RunFake runs a one-shot prompt through harness using a deterministic fake provider stream.
func RunFake(ctx context.Context, opts RunOptions) error {
	opts.MemorySession = true
	if strings.TrimSpace(opts.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	skills, err := DiscoverSkills(ctx, opts.CWD)
	if err != nil {
		return err
	}
	stream := fakeStream()
	sess, err := runSession(opts.CWD, opts)
	if err != nil {
		return err
	}
	defer sess.Close(ctx)
	resolved, err := ResolveHarnessConfig(ctx, ResolveOptions{
		Config:  config.Config{Provider: "fake", Model: "echo", CWD: opts.CWD, Tools: opts.Tools},
		Session: sess,
		Stream:  stream,
		Skills:  skills,
	})
	if err != nil {
		return err
	}
	h, err := harness.New(ctx, resolved.Harness)
	if err != nil {
		return err
	}
	events, err := h.Prompt(ctx, opts.Prompt, harness.PromptOptions{})
	if err != nil {
		return err
	}
	for ev := range events {
		renderRunEvent(out, ev, opts.ShowTools)
	}
	if err := h.Wait(ctx); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out)
	return nil
}

func applyExtensionHostActions(ctx context.Context, h *harness.Harness, actions []extension.HostAction) error {
	for _, action := range actions {
		switch action.Type {
		case "set_active_tools":
			var data struct {
				Names []string `json:"names"`
			}
			if err := json.Unmarshal(action.Data, &data); err != nil {
				return err
			}
			if err := h.SetActiveTools(ctx, data.Names); err != nil {
				return err
			}
		case "save_point":
			var data struct {
				Label string          `json:"label"`
				Data  json.RawMessage `json:"data,omitempty"`
			}
			if err := json.Unmarshal(action.Data, &data); err != nil {
				return err
			}
			var payload any
			if len(data.Data) > 0 {
				payload = data.Data
			}
			if _, err := h.SavePoint(ctx, data.Label, payload); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderRunEvent(out io.Writer, ev event.Event, showTools bool) {
	switch e := ev.(type) {
	case event.MessageDelta:
		fmt.Fprint(out, e.Text)
	case event.ToolExecutionStart:
		if showTools {
			fmt.Fprintf(out, "\n[tool start] %s %s\n", e.Name, strings.TrimSpace(string(e.Args)))
		}
	case event.ToolExecutionProgress:
		if showTools && strings.TrimSpace(e.Text) != "" {
			fmt.Fprintf(out, "[tool progress] %s %s\n", e.ID, e.Text)
		}
	case event.ToolExecutionEnd:
		if showTools {
			status := "done"
			if e.IsError {
				status = "error"
			}
			fmt.Fprintf(out, "[tool end] %s %s\n", e.Name, status)
		}
	}
}

func runSession(cwd string, opts RunOptions) (session.Session, error) {
	if opts.MemorySession || opts.SessionPath == "" {
		return memory.New(""), nil
	}
	return jsonl.Open(opts.SessionPath, session.Metadata{ID: filepath.Base(opts.SessionPath), CWD: cwd})
}

func fakeStream() provider.StreamFunc {
	var calls int
	return func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		calls++
		text := strings.ToLower(userText(req))
		if calls == 1 {
			if name, args, ok := parseGenericTool(text, req.Tools); ok {
				return streamEvents(provider.MessageStart{MessageID: "fake-1"}, provider.ToolCall{ID: "call-" + name, Name: name, Arguments: args}, provider.MessageEnd{StopReason: "tool_use"}), nil
			}
			if name, input, ok := parseSkill(text); ok {
				return streamEvents(provider.MessageStart{MessageID: "fake-skill"}, provider.TextDelta{Text: "fake skill invocation: " + name + " " + input}, provider.MessageEnd{StopReason: "end_turn"}), nil
			}
			if path, content, ok := parseWrite(text); ok {
				args, _ := json.Marshal(map[string]string{"path": path, "content": content})
				return streamEvents(provider.MessageStart{MessageID: "fake-1"}, provider.ToolCall{ID: "call-write", Name: "write", Arguments: args}, provider.MessageEnd{StopReason: "tool_use"}), nil
			}
			if path, oldText, newText, ok := parseEdit(text); ok {
				args, _ := json.Marshal(map[string]string{"path": path, "old_text": oldText, "new_text": newText})
				return streamEvents(provider.MessageStart{MessageID: "fake-1"}, provider.ToolCall{ID: "call-edit", Name: "edit", Arguments: args}, provider.MessageEnd{StopReason: "tool_use"}), nil
			}
			if path, ok := parseRead(text); ok {
				args, _ := json.Marshal(map[string]string{"path": path})
				return streamEvents(provider.MessageStart{MessageID: "fake-1"}, provider.ToolCall{ID: "call-read", Name: "read", Arguments: args}, provider.MessageEnd{StopReason: "tool_use"}), nil
			}
		}
		if toolText := lastToolText(req); toolText != "" {
			return streamEvents(provider.MessageStart{MessageID: "fake-final"}, provider.TextDelta{Text: "fake response: " + toolText}, provider.MessageEnd{StopReason: "end_turn"}), nil
		}
		return streamEvents(provider.MessageStart{MessageID: "fake-final"}, provider.TextDelta{Text: "fake response: " + userText(req)}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}
}

func streamEvents(events ...provider.Event) <-chan provider.Event {
	ch := make(chan provider.Event, len(events))
	for _, ev := range events {
		ch <- ev
	}
	close(ch)
	return ch
}

func parseGenericTool(text string, specs []tool.Spec) (name string, args json.RawMessage, ok bool) {
	fields := strings.Fields(text)
	for i, f := range fields {
		if (f == "tool" || f == "use-tool") && i+1 < len(fields) {
			want := fields[i+1]
			for _, spec := range specs {
				if spec.Name == want {
					return want, json.RawMessage(`{}`), true
				}
			}
		}
	}
	return "", nil, false
}

func parseSkill(text string) (name, input string, ok bool) {
	fields := strings.Fields(text)
	for i, f := range fields {
		if (f == "skill" || f == "use-skill") && i+1 < len(fields) {
			return fields[i+1], strings.Join(fields[i+2:], " "), true
		}
	}
	return "", "", false
}

func parseRead(text string) (string, bool) {
	fields := strings.Fields(text)
	for i, f := range fields {
		if f == "read" && i+1 < len(fields) {
			return fields[i+1], true
		}
	}
	return "", false
}

func parseWrite(text string) (path, content string, ok bool) {
	fields := strings.Fields(text)
	for i, f := range fields {
		if f == "write" && i+1 < len(fields) {
			content = "written by erasmus\n"
			if idx := strings.Index(text, " content "); idx >= 0 {
				content = text[idx+len(" content "):]
			}
			return fields[i+1], content, true
		}
	}
	return "", "", false
}

func parseEdit(text string) (path, oldText, newText string, ok bool) {
	fields := strings.Fields(text)
	for i, f := range fields {
		if f == "edit" && i+3 < len(fields) {
			return fields[i+1], fields[i+2], fields[i+3], true
		}
	}
	return "", "", "", false
}

func lastToolText(req provider.Request) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role != message.RoleTool {
			continue
		}
		for _, c := range req.Messages[i].Content {
			if result, ok := c.(message.ToolResult); ok {
				for _, rc := range result.Content {
					if text, ok := rc.(message.Text); ok {
						return text.Text
					}
				}
			}
		}
	}
	return ""
}

func userText(req provider.Request) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role != "user" {
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
