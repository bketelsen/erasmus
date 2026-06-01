package app

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"erasmus/packages/auth"
	"erasmus/packages/config"
)

func TestServeSwarmStdioSpawnWaitListSendClose(t *testing.T) {
	input := strings.Join([]string{
		`{"id":"1","method":"spawn","params":{"id":"main","task":"first","memory":true}}`,
		`{"id":"2","method":"wait","params":{"id":"main"}}`,
		`{"id":"3","method":"send","params":{"id":"main","text":"second"}}`,
		`{"id":"4","method":"wait","params":{"id":"main"}}`,
		`{"id":"5","method":"list"}`,
		`{"id":"6","method":"status"}`,
		`{"id":"7","method":"close"}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	err := ServeSwarmStdio(context.Background(), strings.NewReader(input), &out, config.Config{Provider: "fake", Model: "echo"}, auth.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 7 {
		t.Fatalf("responses = %d, output:\n%s", len(lines), out.String())
	}
	for _, line := range lines {
		var resp struct {
			ID     string          `json:"id"`
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
			Error  string          `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("bad response %q: %v", line, err)
		}
		if !resp.OK {
			t.Fatalf("response failed: %s", line)
		}
	}
	if !strings.Contains(out.String(), `"id":"main"`) || !strings.Contains(out.String(), `"running":false`) || !strings.Contains(out.String(), `"pid":`) {
		t.Fatalf("missing list/snapshot output:\n%s", out.String())
	}
}
