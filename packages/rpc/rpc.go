// Package rpc exposes a harness over a small newline-delimited JSON protocol.
package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/bketelsen/erasmus/packages/auth"
	"github.com/bketelsen/erasmus/packages/event"
	"github.com/bketelsen/erasmus/packages/harness"
	"github.com/bketelsen/erasmus/packages/model"
)

// Request is one JSON-lines RPC request.
type Request struct {
	ID     string          `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is one JSON-lines RPC response.
type Response struct {
	ID     string `json:"id,omitempty"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// EventNotification is emitted asynchronously for harness events.
type EventNotification struct {
	Method string       `json:"method"`
	Params EventPayload `json:"params"`
}

// EventPayload wraps a concrete event with its canonical type string.
type EventPayload struct {
	Type  string      `json:"type"`
	Event event.Event `json:"event"`
}

// Server serves requests against a single harness.
type Server struct {
	Harness *harness.Harness
	Catalog model.Catalog
	Auth    auth.Store

	wg sync.WaitGroup
}

// Serve reads newline-delimited JSON requests from in and writes responses/events to out.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	if s.Harness == nil {
		return fmt.Errorf("harness is required")
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
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

type promptParams struct {
	Text string `json:"text"`
}

func (s *Server) handle(ctx context.Context, req Request, write func(any) error) error {
	switch req.Method {
	case "state":
		return write(Response{ID: req.ID, Result: s.Harness.State(ctx)})
	case "session":
		meta, err := s.Harness.Session().Metadata(ctx)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: meta})
	case "session_context":
		sctx, err := s.Harness.Session().BuildContext(ctx)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: sctx})
	case "models":
		catalog := s.Catalog
		if catalog == nil {
			catalog = model.DefaultCatalog()
		}
		return write(Response{ID: req.ID, Result: catalog.List()})
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
	case "prompt":
		var params promptParams
		if err := json.Unmarshal(req.Params, &params); err != nil && len(req.Params) > 0 {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		events, err := s.Harness.Prompt(ctx, params.Text, harness.PromptOptions{})
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			streamEvents(events, write)
		}()
		return write(Response{ID: req.ID, Result: map[string]string{"status": "started"}})
	case "continue":
		events, err := s.Harness.Continue(ctx)
		if err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			streamEvents(events, write)
		}()
		return write(Response{ID: req.ID, Result: map[string]string{"status": "started"}})
	case "abort":
		if err := s.Harness.Abort(ctx); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: map[string]string{"status": "aborted"}})
	case "wait":
		if err := s.Harness.Wait(ctx); err != nil {
			return write(Response{ID: req.ID, Error: err.Error()})
		}
		return write(Response{ID: req.ID, Result: map[string]string{"status": "settled"}})
	default:
		return write(Response{ID: req.ID, Error: fmt.Sprintf("unknown method %q", req.Method)})
	}
}

func streamEvents(events <-chan event.Event, write func(any) error) {
	for ev := range events {
		_ = write(EventNotification{Method: "event", Params: EventPayload{Type: ev.Type(), Event: ev}})
	}
}
