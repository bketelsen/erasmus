package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"erasmus/packages/auth"
	"erasmus/packages/compact"
	"erasmus/packages/event"
	"erasmus/packages/harness"
	"erasmus/packages/model"
	"erasmus/packages/session"
	"erasmus/packages/skill"
)

// RuntimeFactory creates a runtime for a multi-runtime RPC server.
type RuntimeFactory func(context.Context, RuntimeCreateParams) (*Runtime, error)

// Runtime is one RPC-managed harness plus optional runtime resources.
type Runtime struct {
	Harness                 *harness.Harness
	OnEvent                 func(context.Context, event.Event) error
	ExtensionCommands       func(context.Context) ([]ExtensionCommandSummary, error)
	ExecuteExtensionCommand func(context.Context, string, json.RawMessage) ([]ExtensionHostAction, error)
	ExtensionDiagnostics    func(context.Context) ([]string, error)
	Close                   func(context.Context) error
}

// ExtensionProcessParams describes one extension subprocess for runtime_create.
type ExtensionProcessParams struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// ExtensionCommandSummary describes a registered extension command.
type ExtensionCommandSummary struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ExtensionHostAction is returned by extension command execution.
type ExtensionHostAction struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// SkillReloader reloads skills for a runtime.
type SkillReloader func(context.Context, string, *harness.Harness) ([]skill.Skill, error)

// RuntimeCreateParams configures runtime_create.
type RuntimeCreateParams struct {
	ID          string                   `json:"id,omitempty"`
	Provider    string                   `json:"provider,omitempty"`
	Model       string                   `json:"model,omitempty"`
	Reasoning   string                   `json:"reasoning,omitempty"`
	CWD         string                   `json:"cwd,omitempty"`
	SessionPath string                   `json:"session_path,omitempty"`
	Tools       []string                 `json:"tools,omitempty"`
	NoTools     bool                     `json:"no_tools,omitempty"`
	Extensions  []ExtensionProcessParams `json:"extensions,omitempty"`
}

// RuntimeRefParams identifies an existing runtime.
type RuntimeRefParams struct {
	RuntimeID string `json:"runtime_id"`
}

// RuntimePromptParams starts a prompt on an existing runtime.
type RuntimePromptParams struct {
	RuntimeID string `json:"runtime_id"`
	Text      string `json:"text"`
}

// RuntimeMoveToParams moves a runtime to a session tree entry.
type RuntimeMoveToParams struct {
	RuntimeID string                 `json:"runtime_id"`
	EntryID   string                 `json:"entry_id"`
	Summary   string                 `json:"summary,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// RuntimeBranchParams branches a runtime session at an entry.
type RuntimeBranchParams struct {
	RuntimeID string `json:"runtime_id"`
	EntryID   string `json:"entry_id"`
}

// RuntimeSetModelParams updates a runtime model.
type RuntimeSetModelParams struct {
	RuntimeID string `json:"runtime_id"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model"`
}

// RuntimeSetReasoningParams updates a runtime reasoning level.
type RuntimeSetReasoningParams struct {
	RuntimeID string `json:"runtime_id"`
	Reasoning string `json:"reasoning"`
}

// RuntimeCompactParams compacts a runtime transcript.
type RuntimeCompactParams struct {
	RuntimeID          string `json:"runtime_id"`
	KeepTail           int    `json:"keep_tail,omitempty"`
	CustomInstructions string `json:"custom_instructions,omitempty"`
	MaxTokens          int    `json:"max_tokens,omitempty"`
}

// RuntimeCheckpointParams appends a checkpoint marker to a runtime session.
type RuntimeCheckpointParams struct {
	RuntimeID string `json:"runtime_id"`
	Label     string `json:"label,omitempty"`
	Data      any    `json:"data,omitempty"`
}

// RuntimeExtensionCommandParams executes a runtime extension command.
type RuntimeExtensionCommandParams struct {
	RuntimeID string          `json:"runtime_id"`
	Command   string          `json:"command"`
	Input     json.RawMessage `json:"input,omitempty"`
}

// AuthCredentialParams mutates provider credentials without returning secrets.
type AuthCredentialParams struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key,omitempty"`
}

// RuntimeSummary describes a runtime known to a MultiServer.
type RuntimeSummary struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id,omitempty"`
}

// RuntimeEventNotification is emitted asynchronously for multi-runtime harness events.
type RuntimeEventNotification struct {
	Method string              `json:"method"`
	Params RuntimeEventPayload `json:"params"`
}

// RuntimeEventPayload wraps a concrete event with runtime identity.
type RuntimeEventPayload struct {
	RuntimeID string      `json:"runtime_id"`
	Type      string      `json:"type"`
	Event     event.Event `json:"event"`
}

// MultiServer serves JSON-lines RPC for multiple harness runtimes.
type MultiServer struct {
	Factory       RuntimeFactory
	SkillReloader SkillReloader
	Catalog       model.Catalog
	Auth          auth.Store

	mu       sync.Mutex
	wg       sync.WaitGroup
	runtimes map[string]*Runtime
	nextID   int
}

// Serve reads newline-delimited JSON requests from in and writes responses/events to out.
func (s *MultiServer) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	if s.Factory == nil {
		return fmt.Errorf("runtime factory is required")
	}
	enc := json.NewEncoder(out)
	var writeMu sync.Mutex
	write := func(v any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return enc.Encode(v)
	}

	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			if err := write(Response{Error: err.Error()}); err != nil {
				return err
			}
			continue
		}
		if err := s.handle(ctx, req, write); err != nil {
			return err
		}
	}
	s.wg.Wait()
	return scanner.Err()
}

func (s *MultiServer) handle(ctx context.Context, req Request, write func(any) error) error {
	s.ensure()
	switch req.Method {
	case "runtime_create":
		var params RuntimeCreateParams
		if err := json.Unmarshal(req.Params, &params); err != nil && len(req.Params) > 0 {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		if params.ID == "" {
			params.ID = s.newID()
		}
		rt, err := s.Factory(ctx, params)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		if rt == nil || rt.Harness == nil {
			return write(Response{ID: req.ID, Error: "runtime factory returned nil runtime"})
		}
		if err := s.addRuntime(params.ID, rt); err != nil {
			if rt.Close != nil {
				_ = rt.Close(ctx)
			}
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: RuntimeSummary{ID: params.ID, SessionID: rt.Harness.Session().ID()}})
	case "runtime_list":
		return write(Response{ID: req.ID, Result: s.listRuntimes()})
	case "runtime_state":
		rt, err := s.runtimeFromReq(req.Params)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: rt.Harness.State(ctx)})
	case "runtime_session":
		rt, err := s.runtimeFromReq(req.Params)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		meta, err := rt.Harness.Session().Metadata(ctx)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: meta})
	case "runtime_session_context":
		rt, err := s.runtimeFromReq(req.Params)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		sctx, err := rt.Harness.Session().BuildContext(ctx)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: sctx})
	case "runtime_tree":
		rt, err := s.runtimeFromReq(req.Params)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		tree, err := rt.Harness.Tree(ctx)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: tree})
	case "runtime_move_to":
		var params RuntimeMoveToParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		rt, err := s.getRuntime(params.RuntimeID)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		var summary *session.BranchSummary
		if params.Summary != "" {
			summary = &session.BranchSummary{Summary: params.Summary}
		}
		if err := rt.Harness.MoveTo(ctx, session.EntryID(params.EntryID), summary); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		tree, err := rt.Harness.Tree(ctx)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: tree})
	case "runtime_branch":
		var params RuntimeBranchParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		rt, err := s.getRuntime(params.RuntimeID)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		branched, err := rt.Harness.Branch(ctx, session.EntryID(params.EntryID))
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: map[string]string{"session_id": branched.ID()}})
	case "runtime_prompt":
		var params RuntimePromptParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		rt, err := s.getRuntime(params.RuntimeID)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		events, err := rt.Harness.Prompt(ctx, params.Text, harness.PromptOptions{})
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			streamRuntimeEvents(ctx, params.RuntimeID, events, rt.OnEvent, write)
		}()
		return write(Response{ID: req.ID, Result: map[string]string{"status": "started"}})
	case "runtime_set_model":
		var params RuntimeSetModelParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		rt, err := s.getRuntime(params.RuntimeID)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		providerID := params.Provider
		if providerID == "" {
			providerID = rt.Harness.State(ctx).Agent.Model.Provider
		}
		m, err := s.catalog().Find(providerID, params.Model)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		if err := rt.Harness.SetModel(ctx, m); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: m})
	case "runtime_set_reasoning":
		var params RuntimeSetReasoningParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		rt, err := s.getRuntime(params.RuntimeID)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		if err := rt.Harness.SetReasoning(ctx, params.Reasoning); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: map[string]string{"reasoning": params.Reasoning}})
	case "runtime_reload_skills":
		rt, err := s.runtimeFromReq(req.Params)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		if s.SkillReloader == nil {
			return write(Response{ID: req.ID, Error: "skill reloader is not configured"})
		}
		skills, err := s.SkillReloader(ctx, rt.ID, rt.Harness)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		if err := rt.Harness.SetSkills(ctx, skills); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: skills})
	case "runtime_compact":
		var params RuntimeCompactParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		rt, err := s.getRuntime(params.RuntimeID)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		result, err := rt.Harness.Compact(ctx, compact.Options{KeepTail: params.KeepTail, CustomInstructions: params.CustomInstructions, MaxTokens: params.MaxTokens})
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: result})
	case "runtime_checkpoint":
		var params RuntimeCheckpointParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		rt, err := s.getRuntime(params.RuntimeID)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		entryID, err := rt.Harness.SavePoint(ctx, params.Label, params.Data)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: map[string]string{"entry_id": string(entryID), "status": "saved"}})
	case "runtime_extension_commands":
		rt, err := s.runtimeFromReq(req.Params)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		if rt.Runtime.ExtensionCommands == nil {
			return write(Response{ID: req.ID, Result: []ExtensionCommandSummary{}})
		}
		commands, err := rt.Runtime.ExtensionCommands(ctx)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: commands})
	case "runtime_extension_command":
		var params RuntimeExtensionCommandParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		rt, err := s.getRuntime(params.RuntimeID)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		if rt.ExecuteExtensionCommand == nil {
			return write(Response{ID: req.ID, Error: "runtime has no extension command executor"})
		}
		actions, err := rt.ExecuteExtensionCommand(ctx, params.Command, params.Input)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: map[string]any{"actions": actions}})
	case "runtime_extension_diagnostics":
		rt, err := s.runtimeFromReq(req.Params)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		if rt.Runtime.ExtensionDiagnostics == nil {
			return write(Response{ID: req.ID, Result: []string{}})
		}
		diagnostics, err := rt.Runtime.ExtensionDiagnostics(ctx)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: diagnostics})
	case "runtime_continue":
		rt, err := s.runtimeFromReq(req.Params)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		events, err := rt.Harness.Continue(ctx)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			streamRuntimeEvents(ctx, rt.ID, events, rt.Runtime.OnEvent, write)
		}()
		return write(Response{ID: req.ID, Result: map[string]string{"status": "started"}})
	case "runtime_abort":
		rt, err := s.runtimeFromReq(req.Params)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		if err := rt.Harness.Abort(ctx); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: map[string]string{"status": "aborted"}})
	case "runtime_close":
		rt, err := s.runtimeFromReq(req.Params)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		if err := rt.Harness.Session().Close(ctx); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		if rt.Runtime.Close != nil {
			if err := rt.Runtime.Close(ctx); err != nil {
				return write(Response{ID: req.ID, Error: err.Error()})
			}
		}
		s.removeRuntime(rt.ID)
		return write(Response{ID: req.ID, Result: map[string]string{"status": "closed"}})
	case "runtime_wait":
		rt, err := s.runtimeFromReq(req.Params)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		if err := rt.Harness.Wait(ctx); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: map[string]string{"status": "settled"}})
	case "models":
		return write(Response{ID: req.ID, Result: s.catalog().List()})
	case "auth_login":
		if s.Auth == nil {
			return write(Response{ID: req.ID, Error: "auth store is not configured"})
		}
		var params AuthCredentialParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		if err := s.Auth.Set(ctx, auth.Credential{Provider: params.Provider, APIKey: params.APIKey}); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: map[string]string{"provider": params.Provider, "status": "saved"}})
	case "auth_logout":
		if s.Auth == nil {
			return write(Response{ID: req.ID, Error: "auth store is not configured"})
		}
		var params AuthCredentialParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		if err := s.Auth.Delete(ctx, params.Provider); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: map[string]string{"provider": params.Provider, "status": "removed"}})
	case "auth_status":
		if s.Auth == nil {
			return write(Response{ID: req.ID, Result: []string{}})
		}
		creds, err := s.Auth.List(ctx)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		providers := make([]string, 0, len(creds))
		for _, c := range creds {
			providers = append(providers, c.Provider)
		}
		return write(Response{ID: req.ID, Result: providers})
	default:
		return write(Response{ID: req.ID, Error: fmt.Sprintf("unknown method %q", req.Method)})
	}
}

type runtimeHandle struct {
	ID      string
	Runtime *Runtime
	Harness *harness.Harness
}

func (s *MultiServer) ensure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtimes == nil {
		s.runtimes = map[string]*Runtime{}
	}
}

func (s *MultiServer) catalog() model.Catalog {
	if s.Catalog != nil {
		return s.Catalog
	}
	return model.DefaultCatalog()
}

func (s *MultiServer) newID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	return fmt.Sprintf("runtime-%d", s.nextID)
}

func (s *MultiServer) addRuntime(id string, rt *Runtime) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runtimes[id]; ok {
		return fmt.Errorf("runtime %q already exists", id)
	}
	s.runtimes[id] = rt
	return nil
}

func (s *MultiServer) getRuntime(id string) (*Runtime, error) {
	if id == "" {
		return nil, fmt.Errorf("runtime_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rt, ok := s.runtimes[id]
	if !ok {
		return nil, fmt.Errorf("runtime %q not found", id)
	}
	return rt, nil
}

func (s *MultiServer) runtimeFromReq(raw json.RawMessage) (runtimeHandle, error) {
	var params RuntimeRefParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return runtimeHandle{}, err
	}
	rt, err := s.getRuntime(params.RuntimeID)
	if err != nil {
		return runtimeHandle{}, err
	}
	return runtimeHandle{ID: params.RuntimeID, Runtime: rt, Harness: rt.Harness}, nil
}

func (s *MultiServer) removeRuntime(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.runtimes, id)
}

func (s *MultiServer) listRuntimes() []RuntimeSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RuntimeSummary, 0, len(s.runtimes))
	for id, rt := range s.runtimes {
		out = append(out, RuntimeSummary{ID: id, SessionID: rt.Harness.Session().ID()})
	}
	return out
}

func streamRuntimeEvents(ctx context.Context, runtimeID string, events <-chan event.Event, onEvent func(context.Context, event.Event) error, write func(any) error) {
	for ev := range events {
		if onEvent != nil {
			if err := onEvent(ctx, ev); err != nil {
				_ = write(RuntimeEventNotification{Method: "runtime_event", Params: RuntimeEventPayload{RuntimeID: runtimeID, Type: "error", Event: event.Error{Err: err.Error()}}})
				return
			}
		}
		_ = write(RuntimeEventNotification{Method: "runtime_event", Params: RuntimeEventPayload{RuntimeID: runtimeID, Type: ev.Type(), Event: ev}})
	}
}
