package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRPCFakeDurableSessionPathResume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	ctx := context.Background()

	var first bytes.Buffer
	firstIn := strings.NewReader(
		`{"id":"1","method":"runtime_create","params":{"id":"main","session_path":` + quoteJSON(path) + `}}` + "\n" +
			`{"id":"2","method":"runtime_prompt","params":{"runtime_id":"main","text":"hello"}}` + "\n" +
			`{"id":"3","method":"runtime_wait","params":{"runtime_id":"main"}}` + "\n" +
			`{"id":"4","method":"runtime_close","params":{"runtime_id":"main"}}` + "\n")
	if err := RunRPCFake(ctx, firstIn, &first, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}

	var second bytes.Buffer
	secondIn := strings.NewReader(
		`{"id":"1","method":"runtime_create","params":{"id":"main","session_path":` + quoteJSON(path) + `}}` + "\n" +
			`{"id":"2","method":"runtime_session_context","params":{"runtime_id":"main"}}` + "\n")
	if err := RunRPCFake(ctx, secondIn, &second, ""); err != nil {
		t.Fatal(err)
	}
	got := second.String()
	if !strings.Contains(got, "fake response: hello") || !strings.Contains(got, `"messages"`) {
		t.Fatalf("expected resumed context, got:\n%s", got)
	}
}

func quoteJSON(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}
