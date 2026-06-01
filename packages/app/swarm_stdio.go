package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bketelsen/erasmus/packages/auth"
	"github.com/bketelsen/erasmus/packages/config"
	"github.com/bketelsen/erasmus/packages/event"
	"github.com/bketelsen/erasmus/packages/harness"
	"github.com/bketelsen/erasmus/packages/skill"
	"github.com/bketelsen/erasmus/packages/swarm"
	"github.com/bketelsen/erasmus/packages/tool"
)

type swarmStdioRequest struct {
	ID     string          `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type swarmStdioResponse struct {
	ID     string `json:"id,omitempty"`
	OK     bool   `json:"ok"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

type swarmSpawnParams struct {
	ID           string `json:"id,omitempty"`
	Task         string `json:"task,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	CWD          string `json:"cwd,omitempty"`
	SessionPath  string `json:"session_path,omitempty"`
	SessionDir   string `json:"session_dir,omitempty"`
	Memory       bool   `json:"memory,omitempty"`
	EventLogDir  string `json:"event_log_dir,omitempty"`
	SessionScope string `json:"session_scope,omitempty"`
}

type swarmIDParams struct {
	ID string `json:"id"`
}

type swarmSendParams struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type swarmController struct {
	mu                sync.Mutex
	s                 *swarm.Swarm
	runtimes          map[string]*harness.Harness
	cleanupExtensions func()
	shutdown          func()
	started           time.Time
	pid               int
	cwd               string
	provider          string
	model             string
	socket            string
}

type swarmControllerStatus struct {
	PID      int              `json:"pid"`
	Socket   string           `json:"socket,omitempty"`
	CWD      string           `json:"cwd,omitempty"`
	Provider string           `json:"provider,omitempty"`
	Model    string           `json:"model,omitempty"`
	Started  time.Time        `json:"started"`
	Uptime   string           `json:"uptime"`
	Agents   []swarm.Snapshot `json:"agents"`
}

func newSwarmController(ctx context.Context, cfg config.Config, store auth.Store) (*swarmController, error) {
	runtimes := map[string]*harness.Harness{}
	cwd := cfg.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	extensions, err := StartConfiguredExtensionSet(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var extraTools tool.Registry
	var extensionSkills []skill.Skill
	var cleanupExtensions func()
	if extensions != nil {
		extraTools = extensions.Tools()
		extensionSkills = extensions.Skills()
		cleanupExtensions = extensions.Close
	} else {
		cleanupExtensions = func() {}
	}
	s, err := swarm.New(swarm.Config{EventLogDir: defaultSwarmEventLogDir(cwd), Factory: func(ctx context.Context, req swarm.SpawnRequest) (*harness.Harness, error) {
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
		skills = append(skills, extensionSkills...)
		sess, err := swarmSession(req.ID, runtimeCfg.CWD, SwarmRunOptions{SessionPath: reqSessionPath(req), SessionDir: reqSessionDir(req), MemorySession: req.SessionScope == "memory"})
		if err != nil {
			return nil, err
		}
		resolved, err := ResolveHarnessConfig(ctx, ResolveOptions{Config: runtimeCfg, Session: sess, Auth: store, Skills: skills, ExtraTools: extraTools})
		if err != nil {
			_ = sess.Close(ctx)
			return nil, err
		}
		h, err := harness.New(ctx, resolved.Harness)
		if err != nil {
			_ = sess.Close(ctx)
			return nil, err
		}
		if extensions != nil {
			if err := applyExtensionHostActions(ctx, h, extensions.DrainHostActions()); err != nil {
				_ = sess.Close(ctx)
				return nil, err
			}
			h.Subscribe(func(ev event.Event) {
				_ = forwardExtensionRuntimeEvent(context.Background(), h, extensions, ev)
			})
		}
		runtimes[req.ID] = h
		return h, nil
	}})
	if err != nil {
		cleanupExtensions()
		return nil, err
	}
	if extensions != nil {
		if err := applyExtensionBackgroundActions(ctx, s, extensions.DrainHostActions()); err != nil {
			cleanupExtensions()
			return nil, err
		}
	}
	return &swarmController{s: s, runtimes: runtimes, cleanupExtensions: cleanupExtensions, started: time.Now(), pid: os.Getpid(), cwd: cwd, provider: cfg.Provider, model: cfg.Model}, nil
}

func defaultSwarmEventLogDir(cwd string) string {
	if dir := os.Getenv("ERASMUS_SWARM_EVENT_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(xdgStateHome(), "erasmus", "swarm", "events", stateProjectKey(cwd))
}

// ServeSwarmStdio serves a long-lived swarm controller over newline-delimited JSON on stdin/stdout.
func ServeSwarmStdio(ctx context.Context, in io.Reader, out io.Writer, cfg config.Config, store auth.Store) error {
	controller, err := newSwarmController(ctx, cfg, store)
	if err != nil {
		return err
	}
	defer controller.close(ctx)
	return controller.serveConn(ctx, in, out)
}

func (c *swarmController) serveConn(ctx context.Context, in io.Reader, out io.Writer) error {
	enc := json.NewEncoder(out)
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		var req swarmStdioRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = enc.Encode(swarmStdioResponse{OK: false, Error: err.Error()})
			continue
		}
		result, err := c.handle(ctx, req)
		resp := swarmStdioResponse{ID: req.ID, OK: err == nil, Result: result}
		if err != nil {
			resp.Error = err.Error()
			resp.Result = nil
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
		if req.Method == "close" && err == nil {
			return nil
		}
	}
	return scanner.Err()
}

func (c *swarmController) handle(ctx context.Context, req swarmStdioRequest) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch req.Method {
	case "spawn":
		var params swarmSpawnParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, err
		}
		scope := params.SessionScope
		if params.Memory {
			scope = "memory"
		}
		agent, err := c.s.Spawn(ctx, swarm.SpawnRequest{ID: params.ID, Task: params.Task, Provider: params.Provider, Model: params.Model, CWD: params.CWD, SessionScope: encodeSessionScope(scope, params.SessionPath, params.SessionDir)})
		if err != nil {
			return nil, err
		}
		return agent.Snapshot(ctx), nil
	case "send":
		var params swarmSendParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, err
		}
		return map[string]string{"id": params.ID}, c.s.Send(ctx, params.ID, params.Text)
	case "wait":
		var params swarmIDParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, err
		}
		agent, err := c.s.Resume(ctx, params.ID)
		if err != nil {
			return nil, err
		}
		if err := agent.Wait(ctx); err != nil {
			return nil, err
		}
		return agent.Snapshot(ctx), nil
	case "stop":
		var params swarmIDParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, err
		}
		return map[string]string{"id": params.ID}, c.s.Stop(ctx, params.ID)
	case "list":
		return c.s.List(ctx)
	case "status":
		agents, err := c.s.List(ctx)
		if err != nil {
			return nil, err
		}
		return swarmControllerStatus{PID: c.pid, Socket: c.socket, CWD: c.cwd, Provider: c.provider, Model: c.model, Started: c.started, Uptime: time.Since(c.started).Round(time.Second).String(), Agents: agents}, nil
	case "close":
		c.close(ctx)
		if c.shutdown != nil {
			c.shutdown()
		}
		return map[string]bool{"closed": true}, nil
	default:
		return nil, fmt.Errorf("unknown swarm method %q", req.Method)
	}
}

func encodeSessionScope(scope, sessionPath, sessionDir string) string {
	if scope == "memory" {
		return "memory"
	}
	if sessionPath != "" {
		return "path:" + sessionPath
	}
	if sessionDir != "" {
		return "dir:" + sessionDir
	}
	return scope
}

func (c *swarmController) close(ctx context.Context) {
	for _, h := range c.runtimes {
		_ = h.Session().Close(ctx)
	}
	if c.cleanupExtensions != nil {
		c.cleanupExtensions()
		c.cleanupExtensions = nil
	}
}

func reqSessionPath(req swarm.SpawnRequest) string {
	if strings.HasPrefix(req.SessionScope, "path:") {
		return strings.TrimPrefix(req.SessionScope, "path:")
	}
	return ""
}

func reqSessionDir(req swarm.SpawnRequest) string {
	if strings.HasPrefix(req.SessionScope, "dir:") {
		return strings.TrimPrefix(req.SessionScope, "dir:")
	}
	return ""
}
