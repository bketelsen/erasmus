package swarm

import (
	"context"
	"encoding/json"
	"net"
	"testing"
)

func TestSocketRequest(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte(`{"ok":true,"result":{"pong":true}}` + "\n"))
	}()
	resp, err := SocketRequest(context.Background(), ln.Addr().String(), StdioRequest{Method: "ping"})
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]bool
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatal(err)
	}
	if !result["pong"] {
		t.Fatalf("result = %s", string(resp.Result))
	}
}
