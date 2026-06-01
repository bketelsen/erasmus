package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"erasmus/packages/auth"
	"erasmus/packages/config"
	"erasmus/packages/event"
	"erasmus/packages/harness"
	"erasmus/packages/session"
	"erasmus/packages/session/jsonl"
	"erasmus/packages/session/memory"
	"erasmus/packages/swarm"
)

// SwarmRunOptions configures a one-shot swarm run.
type SwarmRunOptions struct {
	Task          string
	Out           io.Writer
	CWD           string
	EventLogDir   string
	SessionPath   string
	SessionDir    string
	MemorySession bool
	Subprocess    bool
}

// RunSwarmConfigured runs one background swarm agent using saved config/auth provider resolution and waits for it.
func RunSwarmConfigured(ctx context.Context, opts SwarmRunOptions, cfg config.Config, store auth.Store) error {
	if opts.Task == "" {
		return fmt.Errorf("task is required")
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	if opts.CWD != "" && cfg.CWD == "" {
		cfg.CWD = opts.CWD
	}
	if opts.Subprocess {
		return RunSwarmSubprocess(ctx, opts)
	}
	extraTools, cleanupExtensions, err := StartConfiguredExtensions(ctx, cfg)
	if err != nil {
		return err
	}
	defer cleanupExtensions()
	s, err := swarm.New(swarm.Config{
		EventLogDir: opts.EventLogDir,
		Factory: func(ctx context.Context, req swarm.SpawnRequest) (*harness.Harness, error) {
			runtimeCfg := cfg
			if req.CWD != "" {
				runtimeCfg.CWD = req.CWD
			}
			if req.Provider != "" {
				runtimeCfg.Provider = req.Provider
			}
			if req.Model != "" {
				runtimeCfg.Model = req.Model
			}
			skills, err := DiscoverSkills(ctx, runtimeCfg.CWD)
			if err != nil {
				return nil, err
			}
			sess, err := swarmSession(req.ID, runtimeCfg.CWD, opts)
			if err != nil {
				return nil, err
			}
			resolved, err := ResolveHarnessConfig(ctx, ResolveOptions{
				Config:     runtimeCfg,
				Session:    sess,
				Auth:       store,
				Skills:     skills,
				ExtraTools: extraTools,
			})
			if err != nil {
				return nil, err
			}
			return harness.New(ctx, resolved.Harness)
		},
	})
	if err != nil {
		return err
	}
	agent, err := s.Spawn(ctx, swarm.SpawnRequest{ID: "main", Task: opts.Task, CWD: cfg.CWD, Provider: cfg.Provider, Model: cfg.Model})
	if err != nil {
		return err
	}
	defer agent.Harness().Session().Close(ctx)
	if err := agent.Wait(ctx); err != nil {
		return err
	}
	fmt.Fprintf(out, "swarm agent %s settled\n", agent.ID())
	if opts.EventLogDir != "" {
		list, err := s.List(ctx)
		if err != nil {
			return err
		}
		if len(list) > 0 && list[0].EventLog != "" {
			fmt.Fprintf(out, "event log: %s\n", list[0].EventLog)
		}
	}
	var text string
	for _, ev := range agent.Events() {
		if delta, ok := ev.(event.MessageDelta); ok {
			text += delta.Text
		}
	}
	if text != "" {
		fmt.Fprintln(out, text)
	}
	return nil
}

// RunSwarmFake runs one background swarm agent with the deterministic fake provider and waits for it.
func RunSwarmFake(ctx context.Context, opts SwarmRunOptions) error {
	opts.MemorySession = true
	return RunSwarmConfigured(ctx, opts, config.Config{Provider: "fake", Model: "echo", CWD: opts.CWD}, auth.NewMemoryStore())
}

// RunSwarmSubprocess runs the one-shot swarm command in an isolated child erasmus process.
func RunSwarmSubprocess(ctx context.Context, opts SwarmRunOptions) error {
	if opts.Task == "" {
		return fmt.Errorf("task is required")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"swarm", "child"}
	if opts.MemorySession {
		args = append(args, "--memory")
	}
	if opts.SessionPath != "" {
		args = append(args, "--session", opts.SessionPath)
	}
	if opts.SessionDir != "" {
		args = append(args, "--session-dir", opts.SessionDir)
	}
	args = append(args, opts.Task)
	return swarm.RunSubprocess(ctx, swarm.SubprocessRun{Executable: exe, Args: args, Env: os.Environ(), Dir: opts.CWD, Stdout: opts.Out})
}

func swarmSession(id, cwd string, opts SwarmRunOptions) (session.Session, error) {
	if opts.MemorySession || (opts.SessionPath == "" && opts.SessionDir == "") {
		return memory.New(id), nil
	}
	path := opts.SessionPath
	if path == "" {
		name := id
		if name == "" {
			name = "main"
		}
		path = filepath.Join(opts.SessionDir, name+".jsonl")
	}
	return jsonl.Open(path, session.Metadata{ID: filepath.Base(path), CWD: cwd})
}
