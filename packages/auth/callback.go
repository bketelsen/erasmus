package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CallbackResult is delivered by an OAuth loopback callback server.
type CallbackResult struct {
	Code    string
	State   string
	Err     error
	RawPath string
}

// CallbackServer is a single-shot OAuth callback listener.
type CallbackServer struct {
	l        net.Listener
	srv      *http.Server
	provider OAuthProvider
	state    string
	result   chan CallbackResult
	once     sync.Once
}

// NewCallbackServer starts a loopback listener for an OAuth redirect.
func NewCallbackServer(p OAuthProvider, expectedState string) (*CallbackServer, error) {
	addr := fmt.Sprintf("%s:%d", p.RedirectHost, p.RedirectPort)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("bind %s: %w", addr, err)
	}
	cs := &CallbackServer{
		l:        l,
		provider: p,
		state:    expectedState,
		result:   make(chan CallbackResult, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc(p.RedirectPath, cs.handle)
	cs.srv = &http.Server{Handler: mux, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second}
	go func() { _ = cs.srv.Serve(l) }()
	return cs, nil
}

// Result waits for the browser callback or context cancellation.
func (cs *CallbackServer) Result(ctx context.Context) (CallbackResult, error) {
	select {
	case r := <-cs.result:
		return r, nil
	case <-ctx.Done():
		return CallbackResult{}, ctx.Err()
	}
}

// Shutdown stops the callback server.
func (cs *CallbackServer) Shutdown() {
	cs.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = cs.srv.Shutdown(ctx)
		_ = cs.l.Close()
	})
}

func (cs *CallbackServer) handle(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	w.Header().Set("content-type", "text/html; charset=utf-8")
	if errParam := q.Get("error"); errParam != "" {
		msg := errParam
		if d := q.Get("error_description"); d != "" {
			msg += ": " + d
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(oauthErrorHTML(msg)))
		cs.deliver(CallbackResult{Err: fmt.Errorf("%s", msg), RawPath: r.URL.RequestURI()})
		return
	}
	code := q.Get("code")
	state := q.Get("state")
	if code == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(oauthErrorHTML("missing authorization code")))
		cs.deliver(CallbackResult{Err: fmt.Errorf("missing authorization code"), RawPath: r.URL.RequestURI()})
		return
	}
	if cs.state != "" && state != cs.state {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(oauthErrorHTML("state mismatch")))
		cs.deliver(CallbackResult{Err: fmt.Errorf("state mismatch"), RawPath: r.URL.RequestURI()})
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(oauthSuccessHTML(cs.provider.Name)))
	cs.deliver(CallbackResult{Code: code, State: state, RawPath: r.URL.RequestURI()})
}

func (cs *CallbackServer) deliver(r CallbackResult) {
	select {
	case cs.result <- r:
	default:
	}
}

func oauthSuccessHTML(provider string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><title>Erasmus login complete</title></head><body><h1>Logged in to ` + htmlEscape(strings.ToLower(provider)) + `</h1><p>You can close this tab and return to Erasmus.</p></body></html>`
}

func oauthErrorHTML(msg string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><title>Erasmus login failed</title></head><body><h1>Login failed</h1><p>` + htmlEscape(msg) + `</p></body></html>`
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}
