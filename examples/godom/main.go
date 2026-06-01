package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	erasmusapp "erasmus/packages/app"
	"erasmus/packages/auth"
	"erasmus/packages/compact"
	"erasmus/packages/config"
	"erasmus/packages/event"
	"erasmus/packages/harness"
	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/prompt"
	"erasmus/packages/provider"
	"erasmus/packages/sandbox"
	"erasmus/packages/session"
	"erasmus/packages/session/jsonl"
	"erasmus/packages/skill"
	"erasmus/packages/swarm"
	"erasmus/packages/tool"
	"erasmus/packages/tools"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type EventRow struct {
	When string
	Kind string
	Text string
}

type ChatMessage struct {
	Role  string
	Label string
	Text  string
	When  string
}

type AgentRow struct {
	ID        string
	Task      string
	SessionID string
	Running   bool
	Events    int
	EventLog  string
	Error     string
}

type ToolRow struct {
	Name        string
	Description string
}

type SkillRow struct {
	Name        string
	Description string
	Source      string
}

type ToolCard struct {
	godom.Component
	Name        string `godom:"prop"`
	Description string `godom:"prop"`
}

func (c *ToolCard) Use() {
	c.Emit("AppendPrompt", " use-tool "+c.Name)
}

type SkillCard struct {
	godom.Component
	Name        string `godom:"prop"`
	Description string `godom:"prop"`
}

func (c *SkillCard) Use() {
	c.Emit("AppendPrompt", " skill "+c.Name+" summarize this project")
}

type exampleOptions struct {
	Live      bool
	Provider  string
	ModelID   string
	Reasoning string
	Config    string
	Auth      string
	Session   string
	CWD       string
}

type App struct {
	godom.Component

	Title      string
	PromptText string
	ChatInput  string
	Status     string
	Error      string

	Provider  string
	ModelID   string
	Reasoning string
	SessionID string
	CWD       string

	InputTokens  int
	OutputTokens int
	Running      bool
	AutoScroll   bool

	LastAssistant string
	Transcript    string
	Summary       string

	ChatMessages []ChatMessage
	Tools        []ToolRow
	Skills       []SkillRow
	Events       []EventRow
	Agents       []AgentRow

	harness            *harness.Harness
	swarm              *swarm.Swarm
	session            session.Session
	streamingChatIndex int
	mu                 sync.Mutex
}

func NewApp(ctx context.Context) (*App, error) {
	return NewAppWithOptions(ctx, exampleOptions{})
}

func NewAppWithOptions(ctx context.Context, opts exampleOptions) (*App, error) {
	if opts.Live {
		return NewLiveApp(ctx, opts)
	}
	return NewDemoApp(ctx)
}

func NewDemoApp(ctx context.Context) (*App, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	stateDir := filepath.Join(cwd, ".erasmus", "examples", "godom")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, err
	}
	if err := writeExampleSkills(filepath.Join(stateDir, "skills")); err != nil {
		return nil, err
	}

	policy, err := sandbox.New(cwd)
	if err != nil {
		return nil, err
	}
	registry := tools.DefaultRegistry(policy)
	skills, err := skill.Discover(ctx, filepath.Join(stateDir, "skills"))
	if err != nil {
		return nil, err
	}
	sess, err := jsonl.Open(filepath.Join(stateDir, "session.jsonl"), session.Metadata{ID: "godom-demo", CWD: cwd})
	if err != nil {
		return nil, err
	}

	stream := demoStream()
	h, err := harness.New(ctx, harness.Config{
		Session: sess,
		Stream:  stream,
		Model: model.Model{
			Provider:      "fake",
			ID:            "godom-demo",
			DisplayName:   "godom demo fake model",
			ContextWindow: 32000,
			MaxOutput:     2048,
		},
		Reasoning: "medium",
		Prompt:    prompt.StaticBuilder{Base: "You are the Erasmus godom example assistant. Demonstrate tools, sessions, skills, compaction, events, and swarm."},
		Skills:    skills,
		Tools:     registry,
		MaxSteps:  6,
	})
	if err != nil {
		return nil, err
	}

	app := &App{
		Title:              "Erasmus godom chat",
		PromptText:         "write .erasmus/examples/godom/notes.txt with hello from the browser console",
		ChatInput:          "Ask the demo assistant to summarize what it can do.",
		Status:             "ready",
		Provider:           "fake",
		ModelID:            "godom-demo",
		Reasoning:          "medium",
		SessionID:          sess.ID(),
		CWD:                cwd,
		AutoScroll:         true,
		harness:            h,
		swarm:              nil,
		session:            sess,
		streamingChatIndex: -1,
		Tools:              toolRows(registry),
		Skills:             skillRows(skills),
	}

	s, err := newDemoSwarm(ctx, cwd, stateDir, registry, skills)
	if err != nil {
		return nil, err
	}
	app.swarm = s
	h.Subscribe(func(ev event.Event) { app.recordEvent(ev) })
	app.refreshState(ctx)
	return app, nil
}

func NewLiveApp(ctx context.Context, opts exampleOptions) (*App, error) {
	cwd := opts.CWD
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	stateDir := filepath.Join(cwd, ".erasmus", "examples", "godom")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, err
	}
	cfgPath := opts.Config
	if cfgPath == "" {
		cfgPath = defaultConfigPath()
	}
	authPath := opts.Auth
	if authPath == "" {
		authPath = defaultAuthPath()
	}
	cfg, err := erasmusapp.ConfigGet(ctx, cfgPath)
	if err != nil {
		return nil, err
	}
	overrides := config.Config{CWD: cwd, NoTools: true}
	if opts.Provider != "" {
		overrides.Provider = opts.Provider
	}
	if opts.ModelID != "" {
		overrides.Model = opts.ModelID
	}
	if opts.Reasoning != "" {
		overrides.Reasoning = opts.Reasoning
	}
	cfg = config.Merge(cfg, overrides)
	if cfg.Provider == "" || cfg.Provider == "fake" {
		return nil, errors.New("live mode requires a real provider; pass --provider or configure Erasmus with a non-fake provider")
	}
	sessionPath := opts.Session
	if sessionPath == "" {
		sessionPath = filepath.Join(stateDir, "live-session.jsonl")
	}
	sess, err := jsonl.Open(sessionPath, session.Metadata{ID: "godom-live", CWD: cwd})
	if err != nil {
		return nil, err
	}
	resolved, err := erasmusapp.ResolveHarnessConfig(ctx, erasmusapp.ResolveOptions{
		Config:  cfg,
		Session: sess,
		Auth:    auth.NewFileStore(authPath),
	})
	if err != nil {
		_ = sess.Close(ctx)
		return nil, err
	}
	h, err := harness.New(ctx, resolved.Harness)
	if err != nil {
		_ = sess.Close(ctx)
		return nil, err
	}
	app := &App{
		Title:              "Erasmus live chat",
		ChatInput:          "",
		Status:             "ready",
		Provider:           resolved.Model.Provider,
		ModelID:            resolved.Model.ID,
		Reasoning:          cfg.Reasoning,
		SessionID:          sess.ID(),
		CWD:                cwd,
		AutoScroll:         true,
		harness:            h,
		session:            sess,
		streamingChatIndex: -1,
		Tools:              toolRows(resolved.Tools),
	}
	h.Subscribe(func(ev event.Event) { app.recordEvent(ev) })
	app.refreshState(ctx)
	return app, nil
}

func newDemoSwarm(ctx context.Context, cwd, stateDir string, registry tool.Registry, skills []skill.Skill) (*swarm.Swarm, error) {
	return swarm.New(swarm.Config{
		EventLogDir: filepath.Join(stateDir, "swarm-events"),
		Factory: func(ctx context.Context, req swarm.SpawnRequest) (*harness.Harness, error) {
			id := req.ID
			if id == "" {
				id = strings.ReplaceAll(time.Now().Format("150405.000"), ".", "-")
			}
			sess, err := jsonl.Open(filepath.Join(stateDir, "swarm", id+".jsonl"), session.Metadata{ID: id, CWD: cwd})
			if err != nil {
				return nil, err
			}
			return harness.New(ctx, harness.Config{
				Session:   sess,
				Stream:    demoStream(),
				Model:     model.Model{Provider: "fake", ID: "swarm-demo", DisplayName: "swarm demo fake model"},
				Reasoning: "low",
				Prompt:    prompt.StaticBuilder{Base: "You are a supervised Erasmus swarm worker in the godom example."},
				Skills:    skills,
				Tools:     registry,
				MaxSteps:  6,
			})
		},
	})
}

func (a *App) SubmitPrompt() {
	text := strings.TrimSpace(a.PromptText)
	if text == "" || a.Running {
		return
	}
	a.startPrompt(text)
}

func (a *App) SendChat() {
	text := strings.TrimSpace(a.ChatInput)
	if text == "" || a.Running {
		return
	}
	a.ChatInput = ""
	a.PromptText = text
	a.ChatMessages = append(a.ChatMessages, ChatMessage{Role: "user", Label: "You", Text: text, When: time.Now().Format("15:04")})
	a.startPrompt(text)
}

func (a *App) startPrompt(text string) {
	a.Status = "running"
	a.Error = ""
	a.Running = true
	a.LastAssistant = ""
	a.streamingChatIndex = -1
	go func() {
		ctx := context.Background()
		events, err := a.harness.Prompt(ctx, text, harness.PromptOptions{})
		if err != nil {
			a.setStatus("prompt failed", err.Error())
			return
		}
		for range events {
		}
		if err := a.harness.Wait(ctx); err != nil {
			a.setStatus("run failed", err.Error())
			return
		}
		a.setStatus("ready", "")
		a.refreshState(ctx)
	}()
}

func (a *App) ContinueRun() {
	if a.Running {
		return
	}
	a.Status = "continuing"
	a.Error = ""
	a.Running = true
	go func() {
		ctx := context.Background()
		events, err := a.harness.Continue(ctx)
		if err != nil {
			a.setStatus("continue failed", err.Error())
			return
		}
		for range events {
		}
		if err := a.harness.Wait(ctx); err != nil {
			a.setStatus("continue failed", err.Error())
			return
		}
		a.setStatus("ready", "")
		a.refreshState(ctx)
	}()
}

func (a *App) AbortRun() {
	go func() {
		if err := a.harness.Abort(context.Background()); err != nil {
			a.setStatus("abort failed", err.Error())
			return
		}
		a.setStatus("aborted", "")
	}()
}

func (a *App) CompactSession() {
	go func() {
		result, err := a.harness.Compact(context.Background(), compact.Options{KeepTail: 4, CustomInstructions: "Keep demo decisions, tool actions, and swarm handoff notes."})
		if err != nil {
			a.setStatus("compact failed", err.Error())
			return
		}
		a.mu.Lock()
		a.Summary = result.Summary
		a.mu.Unlock()
		a.setStatus("compacted", "")
		a.refreshState(context.Background())
	}()
}

func (a *App) ToggleReasoning() {
	next := "high"
	if a.Reasoning == "high" {
		next = "low"
	}
	go func() {
		if err := a.harness.SetReasoning(context.Background(), next); err != nil {
			a.setStatus("reasoning failed", err.Error())
			return
		}
		a.refreshState(context.Background())
	}()
}

func (a *App) SwitchModel() {
	id := "godom-demo-alt"
	if a.ModelID == id {
		id = "godom-demo"
	}
	go func() {
		if err := a.harness.SetModel(context.Background(), model.Model{Provider: "fake", ID: id, DisplayName: id}); err != nil {
			a.setStatus("model switch failed", err.Error())
			return
		}
		a.refreshState(context.Background())
	}()
}

func (a *App) ReloadSkills() {
	go func() {
		skills, err := skill.Discover(context.Background(), filepath.Join(a.CWD, ".erasmus", "examples", "godom", "skills"))
		if err != nil {
			a.setStatus("skill reload failed", err.Error())
			return
		}
		if err := a.harness.SetSkills(context.Background(), skills); err != nil {
			a.setStatus("skill reload failed", err.Error())
			return
		}
		a.mu.Lock()
		a.Skills = skillRows(skills)
		a.mu.Unlock()
		a.setStatus("skills reloaded", "")
		a.refreshState(context.Background())
	}()
}

func (a *App) SpawnWorker() {
	if a.swarm == nil {
		a.setStatus("swarm unavailable", "live mode does not start the demo swarm")
		return
	}
	go func() {
		ctx := context.Background()
		id := fmt.Sprintf("worker-%d", time.Now().Unix()%100000)
		agent, err := a.swarm.Spawn(ctx, swarm.SpawnRequest{ID: id, Task: "read README.md and produce a tiny project brief", CWD: a.CWD})
		if err != nil {
			a.setStatus("spawn failed", err.Error())
			return
		}
		_ = agent.Wait(ctx)
		a.setStatus("worker complete", "")
		a.refreshState(ctx)
	}()
}

func (a *App) SendToFirstWorker() {
	if a.swarm == nil {
		a.setStatus("swarm unavailable", "live mode does not start the demo swarm")
		return
	}
	go func() {
		ctx := context.Background()
		list, err := a.swarm.List(ctx)
		if err != nil || len(list) == 0 {
			a.setStatus("send failed", "no worker available")
			return
		}
		id := list[0].ID
		if err := a.swarm.Send(ctx, id, "append one follow-up risk or opportunity"); err != nil {
			a.setStatus("send failed", err.Error())
			return
		}
		agent, err := a.swarm.Resume(ctx, id)
		if err == nil {
			_ = agent.Wait(ctx)
		}
		a.setStatus("worker updated", "")
		a.refreshState(ctx)
	}()
}

func (a *App) AppendPrompt(suffix string) {
	a.PromptText = strings.TrimSpace(a.PromptText + suffix)
}

func (a *App) ClearEvents() {
	a.Events = nil
}

func (a *App) recordEvent(ev event.Event) {
	row := EventRow{When: time.Now().Format("15:04:05"), Kind: ev.Type(), Text: describeEvent(ev)}
	a.mu.Lock()
	switch e := ev.(type) {
	case event.MessageDelta:
		a.LastAssistant += e.Text
		a.appendAssistantDelta(e.Text)
	case event.AgentStart:
		a.Running = true
		a.LastAssistant = ""
		a.streamingChatIndex = -1
	case event.AgentEnd:
		a.Running = false
		a.streamingChatIndex = -1
	}
	if u, ok := ev.(event.Usage); ok {
		a.InputTokens = u.Cumulative.InputTokens
		a.OutputTokens = u.Cumulative.OutputTokens
	}
	a.Events = append([]EventRow{row}, a.Events...)
	if len(a.Events) > 80 {
		a.Events = a.Events[:80]
	}
	a.Transcript = transcript(a.harness.State(context.Background()).Agent.Messages)
	a.mu.Unlock()
	a.Refresh()
}

func (a *App) appendAssistantDelta(text string) {
	if text == "" {
		return
	}
	if a.streamingChatIndex < 0 || a.streamingChatIndex >= len(a.ChatMessages) || a.ChatMessages[a.streamingChatIndex].Role != "assistant" {
		a.ChatMessages = append(a.ChatMessages, ChatMessage{Role: "assistant", Label: "Assistant", Text: text, When: time.Now().Format("15:04")})
		a.streamingChatIndex = len(a.ChatMessages) - 1
		return
	}
	a.ChatMessages[a.streamingChatIndex].Text += text
}

func (a *App) refreshState(ctx context.Context) {
	state := a.harness.State(ctx)
	var list []swarm.Snapshot
	if a.swarm != nil {
		list, _ = a.swarm.List(ctx)
	}
	a.mu.Lock()
	a.Provider = state.Agent.Model.Provider
	a.ModelID = state.Agent.Model.ID
	a.Reasoning = state.Agent.Reasoning
	a.SessionID = state.Session.ID
	a.CWD = state.Session.CWD
	a.Running = state.Agent.IsStreaming
	a.Transcript = transcript(state.Agent.Messages)
	a.ChatMessages = chatMessages(state.Agent.Messages)
	a.streamingChatIndex = -1
	a.Agents = agentRows(list)
	a.mu.Unlock()
	a.Refresh()
}

func (a *App) setStatus(status, errText string) {
	a.mu.Lock()
	a.Status = status
	a.Error = errText
	a.mu.Unlock()
	a.Refresh()
}

func demoStream() provider.StreamFunc {
	return func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		out := make(chan provider.Event, 16)
		go func() {
			defer close(out)
			text := strings.ToLower(lastUserText(req.Messages))
			out <- provider.MessageStart{MessageID: fmt.Sprintf("msg-%d", time.Now().UnixNano())}
			if toolText := lastToolText(req.Messages); toolText != "" {
				out <- provider.TextDelta{Text: "Tool result observed: " + truncate(toolText, 220)}
				out <- provider.Usage{Usage: model.Usage{InputTokens: len(text) / 4, OutputTokens: 32}}
				out <- provider.MessageEnd{StopReason: "end_turn"}
				return
			}
			if name, args, ok := scriptedToolCall(text, req.Tools); ok {
				out <- provider.TextDelta{Text: "I'll use the " + name + " tool.\n"}
				out <- provider.ToolCall{ID: "call-" + name, Name: name, Arguments: args}
				out <- provider.Usage{Usage: model.Usage{InputTokens: len(text) / 4, OutputTokens: 16}}
				out <- provider.MessageEnd{StopReason: "tool_use"}
				return
			}
			if strings.Contains(text, "skill") {
				out <- provider.TextDelta{Text: "Skill-aware response. Available skills are in my system prompt; request was: " + truncate(lastUserText(req.Messages), 180)}
			} else {
				out <- provider.TextDelta{Text: "Demo response from " + req.Model.ID + " with " + fmt.Sprint(len(req.Tools)) + " tools available. Prompt: " + truncate(lastUserText(req.Messages), 200)}
			}
			out <- provider.Usage{Usage: model.Usage{InputTokens: len(text) / 4, OutputTokens: 32}}
			out <- provider.MessageEnd{StopReason: "end_turn"}
		}()
		return out, nil
	}
}

func scriptedToolCall(text string, specs []tool.Spec) (string, json.RawMessage, bool) {
	has := func(name string) bool {
		for _, spec := range specs {
			if spec.Name == name {
				return true
			}
		}
		return false
	}
	if strings.Contains(text, "write") && has("write") {
		return "write", mustJSON(map[string]string{"path": ".erasmus/examples/godom/notes.txt", "content": "hello from the Erasmus godom example\n"}), true
	}
	if strings.Contains(text, "read") && has("read") {
		return "read", mustJSON(map[string]string{"path": "README.md"}), true
	}
	if strings.Contains(text, "bash") && has("bash") {
		return "bash", mustJSON(map[string]any{"command": "go test ./packages/compact ./packages/skill", "timeout_ms": 30000}), true
	}
	if strings.Contains(text, "edit") && has("edit") {
		return "edit", mustJSON(map[string]string{"path": ".erasmus/examples/godom/notes.txt", "old_text": "hello", "new_text": "hello edited"}), true
	}
	return "", nil, false
}

func mustJSON(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func lastUserText(messages []message.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == message.RoleUser {
			return contentText(messages[i].Content)
		}
	}
	return ""
}

func lastToolText(messages []message.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == message.RoleTool {
			return contentText(messages[i].Content)
		}
	}
	return ""
}

func contentText(content []message.Content) string {
	var parts []string
	for _, c := range content {
		switch v := c.(type) {
		case message.Text:
			parts = append(parts, v.Text)
		case message.ToolResult:
			parts = append(parts, contentText(v.Content))
		}
	}
	return strings.Join(parts, " ")
}

func describeEvent(ev event.Event) string {
	switch e := ev.(type) {
	case event.MessageDelta:
		return truncate(e.Text, 120)
	case event.ToolExecutionStart:
		return e.Name + " " + string(e.Args)
	case event.ToolExecutionEnd:
		if e.IsError {
			return e.Name + " failed"
		}
		return e.Name + " complete"
	case event.Usage:
		return fmt.Sprintf("input=%d output=%d", e.Cumulative.InputTokens, e.Cumulative.OutputTokens)
	case event.ModelUpdate:
		return e.Model.Provider + "/" + e.Model.ID
	case event.ReasoningUpdate:
		return e.Reasoning
	case event.SessionCompact:
		return truncate(e.Summary, 120)
	case event.ResourcesUpdate:
		return fmt.Sprintf("%d skills", len(e.Skills))
	default:
		return ""
	}
}

func transcript(messages []message.Message) string {
	var b strings.Builder
	for _, msg := range messages {
		text := strings.TrimSpace(contentText(msg.Content))
		if text == "" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n\n", msg.Role, text)
	}
	return strings.TrimSpace(b.String())
}

func chatMessages(messages []message.Message) []ChatMessage {
	rows := make([]ChatMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.Role != message.RoleUser && msg.Role != message.RoleAssistant {
			continue
		}
		text := strings.TrimSpace(contentText(msg.Content))
		if text == "" {
			continue
		}
		label := "Assistant"
		if msg.Role == message.RoleUser {
			label = "You"
		}
		rows = append(rows, ChatMessage{Role: string(msg.Role), Label: label, Text: text, When: ""})
	}
	return rows
}

func toolRows(reg tool.Registry) []ToolRow {
	var rows []ToolRow
	for _, t := range reg.List() {
		rows = append(rows, ToolRow{Name: t.Name(), Description: t.Description()})
	}
	return rows
}

func skillRows(skills []skill.Skill) []SkillRow {
	rows := make([]SkillRow, 0, len(skills))
	for _, s := range skills {
		rows = append(rows, SkillRow{Name: s.Name, Description: s.Description, Source: s.Source})
	}
	return rows
}

func agentRows(items []swarm.Snapshot) []AgentRow {
	rows := make([]AgentRow, 0, len(items))
	for _, s := range items {
		rows = append(rows, AgentRow{ID: s.ID, Task: s.Task, SessionID: s.SessionID, Running: s.Running, Events: s.Events, EventLog: s.EventLog, Error: s.Error})
	}
	return rows
}

func writeExampleSkills(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := map[string]string{
		"review.md": "# Code review\nLook for correctness, test coverage, edge cases, and simple design improvements.",
		"plan.md":   "# Implementation planning\nBreak work into small verifiable steps and keep docs honest.",
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func parseExampleOptions(args []string) (exampleOptions, []string, error) {
	var opts exampleOptions
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--live":
			opts.Live = true
		case strings.HasPrefix(arg, "--provider="):
			opts.Provider = strings.TrimPrefix(arg, "--provider=")
		case arg == "--provider":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return exampleOptions{}, nil, err
			}
			opts.Provider = value
			i = next
		case strings.HasPrefix(arg, "--model="):
			opts.ModelID = strings.TrimPrefix(arg, "--model=")
		case arg == "--model":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return exampleOptions{}, nil, err
			}
			opts.ModelID = value
			i = next
		case strings.HasPrefix(arg, "--reasoning="):
			opts.Reasoning = strings.TrimPrefix(arg, "--reasoning=")
		case arg == "--reasoning":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return exampleOptions{}, nil, err
			}
			opts.Reasoning = value
			i = next
		case strings.HasPrefix(arg, "--config="):
			opts.Config = strings.TrimPrefix(arg, "--config=")
		case arg == "--config":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return exampleOptions{}, nil, err
			}
			opts.Config = value
			i = next
		case strings.HasPrefix(arg, "--auth-file="):
			opts.Auth = strings.TrimPrefix(arg, "--auth-file=")
		case arg == "--auth-file":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return exampleOptions{}, nil, err
			}
			opts.Auth = value
			i = next
		case strings.HasPrefix(arg, "--session="):
			opts.Session = strings.TrimPrefix(arg, "--session=")
		case arg == "--session":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return exampleOptions{}, nil, err
			}
			opts.Session = value
			i = next
		default:
			remaining = append(remaining, arg)
		}
	}
	return opts, remaining, nil
}

func nextArg(args []string, i int, flag string) (string, int, error) {
	if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
		return "", i, fmt.Errorf("%s requires a value", flag)
	}
	return args[i+1], i + 1, nil
}

func defaultConfigPath() string {
	if path := os.Getenv("ERASMUS_CONFIG_FILE"); path != "" {
		return path
	}
	return filepath.Join(xdgConfigHome(), "erasmus", "config.json")
}

func defaultAuthPath() string {
	if path := os.Getenv("ERASMUS_AUTH_FILE"); path != "" {
		return path
	}
	return filepath.Join(xdgDataHome(), "erasmus", "auth.json")
}

func xdgConfigHome() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config")
	}
	return filepath.Join(os.TempDir(), "erasmus", "config")
}

func xdgDataHome() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "share")
	}
	return filepath.Join(os.TempDir(), "erasmus", "data")
}

func main() {
	opts, remaining, err := parseExampleOptions(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
	os.Args = append([]string{os.Args[0]}, remaining...)
	root, err := NewAppWithOptions(context.Background(), opts)
	if err != nil {
		log.Fatal(err)
	}
	app := godom.New()
	app.Component("tool-card", &ToolCard{})
	app.Component("skill-card", &SkillCard{})
	app.Mount(root, ui)
	log.Fatal(app.Start())
}
