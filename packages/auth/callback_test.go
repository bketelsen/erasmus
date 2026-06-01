package auth

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCallbackServerResult(t *testing.T) {
	p := OpenAIOAuth
	p.RedirectPort = freePort(t)
	state := "state-123"
	cs, err := NewCallbackServer(p, state)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Shutdown()

	resp, err := http.Get(p.RedirectURI() + "?code=code-123&state=" + state)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := cs.Result(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Code != "code-123" || result.State != state {
		t.Fatalf("bad result: %+v", result)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestCallbackServerStateMismatch(t *testing.T) {
	p := OpenAIOAuth
	p.RedirectPort = freePort(t)
	cs, err := NewCallbackServer(p, "wanted")
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Shutdown()

	resp, err := http.Get(p.RedirectURI() + "?code=code-123&state=bad")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := cs.Result(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Err == nil || !strings.Contains(result.Err.Error(), "state mismatch") {
		t.Fatalf("expected state mismatch, got %+v", result)
	}
}
