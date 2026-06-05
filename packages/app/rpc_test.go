package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunRPCFake(t *testing.T) {
	in := strings.NewReader(`{"id":"1","method":"models"}` + "\n" + `{"id":"2","method":"auth_login","params":{"provider":"fake","api_key":"secret"}}` + "\n" + `{"id":"3","method":"auth_status"}` + "\n" + `{"id":"4","method":"runtime_create","params":{"id":"main"}}` + "\n" + `{"id":"5","method":"runtime_set_model","params":{"runtime_id":"main","provider":"fake","model":"echo"}}` + "\n" + `{"id":"6","method":"runtime_set_reasoning","params":{"runtime_id":"main","reasoning":"low"}}` + "\n" + `{"id":"7","method":"runtime_reload_skills","params":{"runtime_id":"main"}}` + "\n" + `{"id":"8","method":"runtime_prompt","params":{"runtime_id":"main","text":"hello"}}` + "\n" + `{"id":"9","method":"runtime_wait","params":{"runtime_id":"main"}}` + "\n" + `{"id":"10","method":"runtime_tree","params":{"runtime_id":"main"}}` + "\n" + `{"id":"11","method":"runtime_move_to","params":{"runtime_id":"main","entry_id":"1","summary":"back"}}` + "\n" + `{"id":"12","method":"runtime_branch","params":{"runtime_id":"main","entry_id":"1"}}` + "\n" + `{"id":"13","method":"runtime_checkpoint","params":{"runtime_id":"main","label":"smoke"}}` + "\n" + `{"id":"14","method":"runtime_compact","params":{"runtime_id":"main","keep_tail":1}}` + "\n" + `{"id":"15","method":"auth_logout","params":{"provider":"fake"}}` + "\n")
	var out bytes.Buffer
	if err := RunRPCFake(context.Background(), in, &out, ""); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	// Compaction (id 14) is model-driven: its summary is the streamed model
	// text ("fake response: ...") rather than the former placeholder local
	// summary.
	if !strings.Contains(got, `"Fake Echo"`) || !strings.Contains(got, `"status":"saved"`) || !strings.Contains(got, `"fake"`) || !strings.Contains(got, `"reasoning":"low"`) || !strings.Contains(got, `"runtime_id":"main"`) || !strings.Contains(got, `"type":"message_delta"`) || !strings.Contains(got, "fake response: hello") || !strings.Contains(got, `"leaf_id"`) || !strings.Contains(got, `"session_id"`) || !strings.Contains(got, `"Summary":"fake response: `) || !strings.Contains(got, `"status":"removed"`) {
		t.Fatalf("unexpected rpc output:\n%s", got)
	}
}
