package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"erasmus/packages/auth"
	"erasmus/packages/config"
	"erasmus/packages/harness"
	"erasmus/packages/model"
	"erasmus/packages/rpc"
	"erasmus/packages/session"
	"erasmus/packages/session/jsonl"
	"erasmus/packages/session/memory"
	"erasmus/packages/skill"
	"erasmus/packages/tool"
)

// RunRPCConfigured serves a multi-runtime JSON-lines RPC server using saved config/auth provider resolution.
func RunRPCConfigured(ctx context.Context, in io.Reader, out io.Writer, cfg config.Config, store auth.Store) error {
	factory := func(ctx context.Context, params rpc.RuntimeCreateParams) (*rpc.Runtime, error) {
		runtimeCfg := cfg
		if params.CWD != "" {
			runtimeCfg.CWD = params.CWD
		}
		if params.Provider != "" {
			runtimeCfg.Provider = params.Provider
		}
		if params.Model != "" {
			runtimeCfg.Model = params.Model
		}
		if params.Reasoning != "" {
			runtimeCfg.Reasoning = params.Reasoning
		}
		if params.Tools != nil {
			runtimeCfg.Tools = params.Tools
		}
		if params.NoTools {
			runtimeCfg.NoTools = true
		}
		if params.Extensions != nil {
			runtimeCfg.Extensions = make([]config.ExtensionConfig, 0, len(params.Extensions))
			for _, ext := range params.Extensions {
				runtimeCfg.Extensions = append(runtimeCfg.Extensions, config.ExtensionConfig{Command: ext.Command, Args: ext.Args})
			}
		}
		skills, err := DiscoverSkills(ctx, runtimeCfg.CWD)
		if err != nil {
			return nil, err
		}
		sess, err := rpcSession(params, runtimeCfg.CWD)
		if err != nil {
			return nil, err
		}
		extensions, err := StartConfiguredExtensionSet(ctx, runtimeCfg)
		if err != nil {
			_ = sess.Close(ctx)
			return nil, err
		}
		var extraTools tool.Registry
		if extensions != nil {
			extraTools = extensions.Tools()
		}
		resolved, err := ResolveHarnessConfig(ctx, ResolveOptions{
			Config:     runtimeCfg,
			Session:    sess,
			Skills:     skills,
			Catalog:    model.DefaultCatalog(),
			Auth:       store,
			ExtraTools: extraTools,
		})
		if err != nil {
			if extensions != nil {
				extensions.Close()
			}
			_ = sess.Close(ctx)
			return nil, err
		}
		h, err := harness.New(ctx, resolved.Harness)
		if err != nil {
			if extensions != nil {
				extensions.Close()
			}
			_ = sess.Close(ctx)
			return nil, err
		}
		return &rpc.Runtime{
			Harness: h,
			ExtensionCommands: func(ctx context.Context) ([]rpc.ExtensionCommandSummary, error) {
				var out []rpc.ExtensionCommandSummary
				if extensions == nil {
					return out, ctx.Err()
				}
				for _, cmd := range extensions.Commands() {
					out = append(out, rpc.ExtensionCommandSummary{Name: cmd.Name(), Description: cmd.Description()})
				}
				return out, ctx.Err()
			},
			ExecuteExtensionCommand: func(ctx context.Context, name string, input json.RawMessage) ([]rpc.ExtensionHostAction, error) {
				if extensions == nil {
					return nil, fmt.Errorf("runtime has no extension commands")
				}
				cmd, ok := extensions.Command(name)
				if !ok {
					return nil, fmt.Errorf("extension command %q is not registered", name)
				}
				if len(input) == 0 {
					input = json.RawMessage(`{}`)
				}
				res, err := cmd.Execute(ctx, input)
				if err != nil {
					return nil, err
				}
				actions := make([]rpc.ExtensionHostAction, 0, len(res.Actions))
				for _, action := range res.Actions {
					actions = append(actions, rpc.ExtensionHostAction{Type: action.Type, Data: action.Data})
				}
				return actions, nil
			},
			ExtensionDiagnostics: func(ctx context.Context) ([]string, error) {
				if extensions == nil {
					return nil, ctx.Err()
				}
				return extensions.Diagnostics(), ctx.Err()
			},
			Close: func(ctx context.Context) error {
				if extensions != nil {
					extensions.Close()
				}
				return ctx.Err()
			},
		}, nil
	}
	reloader := func(ctx context.Context, runtimeID string, h *harness.Harness) ([]skill.Skill, error) {
		state := h.State(ctx)
		skillCWD := state.Session.CWD
		if skillCWD == "" {
			skillCWD = cfg.CWD
		}
		return DiscoverSkills(ctx, skillCWD)
	}
	return (&rpc.MultiServer{Factory: factory, SkillReloader: reloader, Catalog: model.DefaultCatalog(), Auth: store}).Serve(ctx, in, out)
}

// RunRPCFake serves a multi-runtime JSON-lines RPC server backed by deterministic fake providers.
func RunRPCFake(ctx context.Context, in io.Reader, out io.Writer, cwd string) error {
	return RunRPCConfigured(ctx, in, out, config.Config{Provider: "fake", Model: "echo", CWD: cwd}, auth.NewMemoryStore())
}

func rpcSession(params rpc.RuntimeCreateParams, cwd string) (session.Session, error) {
	if params.SessionPath == "" {
		return memory.New(params.ID), nil
	}
	return jsonl.Open(params.SessionPath, session.Metadata{ID: params.ID, CWD: cwd})
}
