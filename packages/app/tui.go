package app

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"

	"github.com/bketelsen/erasmus/packages/auth"
	"github.com/bketelsen/erasmus/packages/config"
	"github.com/bketelsen/erasmus/packages/event"
	"github.com/bketelsen/erasmus/packages/harness"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/session"
	"github.com/bketelsen/erasmus/packages/session/jsonl"
	"github.com/bketelsen/erasmus/packages/session/memory"
	"github.com/bketelsen/erasmus/packages/skill"
	"github.com/bketelsen/erasmus/packages/swarm"
	"github.com/bketelsen/erasmus/packages/tool"
	"github.com/bketelsen/erasmus/packages/tui"
)

// TUIOptions configures the line-oriented TUI MVP.
type TUIOptions struct {
	In            io.Reader
	Out           io.Writer
	CWD           string
	SessionPath   string
	MemorySession bool
	Theme         string
}

// RunTUIConfigured runs the TUI MVP using saved config/auth provider resolution.
func RunTUIConfigured(ctx context.Context, opts TUIOptions, cfg config.Config, store auth.Store) error {
	if opts.CWD != "" && cfg.CWD == "" {
		cfg.CWD = opts.CWD
	}
	if opts.Theme != "" {
		cfg.Theme = opts.Theme
	}
	skills, err := DiscoverSkills(ctx, cfg.CWD)
	if err != nil {
		return err
	}
	h, cleanup, err := buildTUIHarness(ctx, cfg, store, skills, opts)
	if err != nil {
		return err
	}
	listSessions := func(ctx context.Context, dir string) ([]tui.SessionSummary, error) {
		entries, err := ListSessions(ctx, dir)
		if err != nil {
			return nil, err
		}
		out := make([]tui.SessionSummary, 0, len(entries))
		for _, entry := range entries {
			out = append(out, tui.SessionSummary{Path: entry.Path, ID: entry.ID, Updated: entry.Updated, Messages: entry.Messages})
		}
		return out, nil
	}
	openSession := func(ctx context.Context, path string) (*harness.Harness, func(), error) {
		openOpts := opts
		openOpts.MemorySession = false
		openOpts.SessionPath = path
		return buildTUIHarness(ctx, cfg, store, skills, openOpts)
	}
	var tuiApp *tui.App
	applyModel := func(ctx context.Context, selected model.Model, reasoning string) error {
		stream, err := resolveStream(ctx, selected, store)
		if err != nil {
			return err
		}
		if err := tuiApp.Harness.SetModelAndStream(ctx, selected, stream); err != nil {
			return err
		}
		return tuiApp.Harness.SetReasoning(ctx, reasoning)
	}
	listSwarms := func(ctx context.Context) ([]tui.SwarmServerSummary, error) {
		records, err := CheckSwarmServers(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]tui.SwarmServerSummary, 0, len(records))
		for _, rec := range records {
			out = append(out, tui.SwarmServerSummary{Name: rec.Name, Socket: rec.Socket, CWD: rec.CWD, Provider: rec.Provider, Model: rec.Model, Status: rec.Status, Reachable: rec.Reachable, Error: rec.Error})
		}
		return out, nil
	}
	swarmStatus := func(ctx context.Context, server tui.SwarmServerSummary) (tui.SwarmStatusSummary, error) {
		return tuiSwarmStatus(ctx, server)
	}
	swarmSend := func(ctx context.Context, server tui.SwarmServerSummary, agentID, text string) (tui.SwarmStatusSummary, error) {
		if _, err := swarm.SocketRequest(ctx, server.Socket, swarm.StdioRequest{Method: "send", Params: map[string]string{"id": agentID, "text": text}}); err != nil {
			return tui.SwarmStatusSummary{}, err
		}
		if _, err := swarm.SocketRequest(ctx, server.Socket, swarm.StdioRequest{Method: "wait", Params: map[string]string{"id": agentID}}); err != nil {
			return tui.SwarmStatusSummary{}, err
		}
		return tuiSwarmStatus(ctx, server)
	}
	swarmStop := func(ctx context.Context, server tui.SwarmServerSummary, agentID string) (tui.SwarmStatusSummary, error) {
		if _, err := swarm.SocketRequest(ctx, server.Socket, swarm.StdioRequest{Method: "stop", Params: map[string]string{"id": agentID}}); err != nil {
			return tui.SwarmStatusSummary{}, err
		}
		return tuiSwarmStatus(ctx, server)
	}
	swarmSpawn := func(ctx context.Context, server tui.SwarmServerSummary, task string) (tui.SwarmStatusSummary, error) {
		if _, err := swarm.SocketRequest(ctx, server.Socket, swarm.StdioRequest{Method: "spawn", Params: map[string]any{"task": task, "memory": true}}); err != nil {
			return tui.SwarmStatusSummary{}, err
		}
		return tuiSwarmStatus(ctx, server)
	}
	tuiApp = &tui.App{Harness: h, HarnessCleanup: cleanup, ListSessions: listSessions, OpenSession: openSession, ApplyModel: applyModel, ListSwarms: listSwarms, SwarmStatus: swarmStatus, SwarmSend: swarmSend, SwarmStop: swarmStop, SwarmSpawn: swarmSpawn, In: opts.In, Out: opts.Out, Theme: cfg.Theme}
	return tuiApp.Run(ctx)
}

func tuiSwarmStatus(ctx context.Context, server tui.SwarmServerSummary) (tui.SwarmStatusSummary, error) {
	resp, err := swarm.SocketRequest(ctx, server.Socket, swarm.StdioRequest{Method: "status"})
	if err != nil {
		return tui.SwarmStatusSummary{}, err
	}
	var status tui.SwarmStatusSummary
	if err := json.Unmarshal(resp.Result, &status); err != nil {
		return tui.SwarmStatusSummary{}, err
	}
	return status, nil
}

// RunTUIFake runs the TUI MVP with the deterministic fake provider.
func RunTUIFake(ctx context.Context, opts TUIOptions) error {
	opts.MemorySession = true
	return RunTUIConfigured(ctx, opts, config.Config{Provider: "fake", Model: "echo", CWD: opts.CWD}, auth.NewMemoryStore())
}

func buildTUIHarness(ctx context.Context, cfg config.Config, store auth.Store, skills []skill.Skill, opts TUIOptions) (*harness.Harness, func(), error) {
	sess, err := tuiSession(cfg.CWD, opts)
	if err != nil {
		return nil, nil, err
	}
	extensions, err := StartConfiguredExtensionSet(ctx, cfg)
	if err != nil {
		_ = sess.Close(ctx)
		return nil, nil, err
	}
	var extraTools tool.Registry
	var extensionSkills []skill.Skill
	if extensions != nil {
		extraTools = extensions.Tools()
		extensionSkills = extensions.Skills()
	}
	cleanup := func() {
		if extensions != nil {
			extensions.Close()
		}
		_ = sess.Close(ctx)
	}
	resolved, err := ResolveHarnessConfig(ctx, ResolveOptions{
		Config:     cfg,
		Session:    sess,
		Auth:       store,
		Skills:     append(skills, extensionSkills...),
		ExtraTools: extraTools,
	})
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	h, err := harness.New(ctx, resolved.Harness)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	if extensions != nil {
		if err := applyExtensionHostActions(ctx, h, extensions.DrainHostActions()); err != nil {
			cleanup()
			return nil, nil, err
		}
		unsubscribe := h.Subscribe(func(ev event.Event) {
			_ = forwardExtensionRuntimeEvent(context.Background(), h, extensions, ev)
		})
		priorCleanup := cleanup
		cleanup = func() {
			unsubscribe()
			priorCleanup()
		}
	}
	return h, cleanup, nil
}

func tuiSession(cwd string, opts TUIOptions) (session.Session, error) {
	if opts.MemorySession {
		return memory.New(""), nil
	}
	path := opts.SessionPath
	if path == "" {
		path = DefaultTUISessionPath(cwd)
	}
	return jsonl.Open(path, session.Metadata{ID: filepath.Base(path), CWD: cwd})
}

// DefaultTUISessionPath returns the default durable session path for the TUI.
func DefaultTUISessionPath(cwd string) string {
	return filepath.Join(DefaultTUISessionDir(cwd), "default.jsonl")
}
