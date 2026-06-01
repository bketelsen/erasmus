package swarm

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
)

func TestStdioProcessRequest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	proc, err := StartStdioProcess(context.Background(), StdioProcessConfig{
		Executable: "/bin/sh",
		Args:       []string{"-c", `while IFS= read -r line; do case "$line" in *fail*) printf '%s\n' '{"ok":false,"error":"boom"}';; *) printf '%s\n' '{"id":"1","ok":true,"result":{"pong":true}}';; esac; done`},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()
	resp, err := proc.Request(StdioRequest{ID: "1", Method: "ping"})
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
	if _, err := proc.Request(StdioRequest{ID: "2", Method: "fail"}); err == nil {
		t.Fatal("expected failure")
	}
}
