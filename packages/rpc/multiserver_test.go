package rpc

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/bketelsen/erasmus/packages/harness"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/provider"
	"github.com/bketelsen/erasmus/packages/session/memory"
	"github.com/bketelsen/erasmus/packages/skill"
)

func TestMultiServerRuntimeLifecycle(t *testing.T) {
	ctx := context.Background()
	factory := func(ctx context.Context, params RuntimeCreateParams) (*Runtime, error) {
		h, err := harness.New(ctx, harness.Config{
			Session: memory.New(params.ID),
			Model:   model.Model{Provider: "fake", ID: "echo"},
			Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
				ch := make(chan provider.Event, 3)
				ch <- provider.MessageStart{MessageID: "m1"}
				ch <- provider.TextDelta{Text: "runtime says hi"}
				ch <- provider.MessageEnd{StopReason: "end_turn"}
				close(ch)
				return ch, nil
			},
		})
		if err != nil {
			return nil, err
		}
		return &Runtime{Harness: h}, nil
	}
	in := strings.NewReader(
		`{"id":"1","method":"runtime_create","params":{"id":"alpha"}}` + "\n" +
			`{"id":"2","method":"runtime_list"}` + "\n" +
			`{"id":"3","method":"runtime_state","params":{"runtime_id":"alpha"}}` + "\n" +
			`{"id":"4","method":"runtime_set_reasoning","params":{"runtime_id":"alpha","reasoning":"medium"}}` + "\n" +
			`{"id":"5","method":"runtime_set_model","params":{"runtime_id":"alpha","provider":"fake","model":"echo"}}` + "\n" +
			`{"id":"6","method":"runtime_prompt","params":{"runtime_id":"alpha","text":"hello"}}` + "\n" +
			`{"id":"7","method":"runtime_wait","params":{"runtime_id":"alpha"}}` + "\n" +
			`{"id":"8","method":"runtime_tree","params":{"runtime_id":"alpha"}}` + "\n" +
			`{"id":"9","method":"runtime_move_to","params":{"runtime_id":"alpha","entry_id":"1","summary":"tree switch"}}` + "\n" +
			`{"id":"10","method":"runtime_branch","params":{"runtime_id":"alpha","entry_id":"2"}}` + "\n" +
			`{"id":"11","method":"runtime_reload_skills","params":{"runtime_id":"alpha"}}` + "\n" +
			`{"id":"12","method":"runtime_checkpoint","params":{"runtime_id":"alpha","label":"before compact","data":{"source":"test"}}}` + "\n" +
			`{"id":"13","method":"runtime_compact","params":{"runtime_id":"alpha","keep_tail":1,"custom_instructions":"keep facts"}}` + "\n" +
			`{"id":"14","method":"runtime_session_context","params":{"runtime_id":"alpha"}}` + "\n" +
			`{"id":"15","method":"runtime_close","params":{"runtime_id":"alpha"}}` + "\n" +
			`{"id":"16","method":"runtime_list"}` + "\n")
	server := &MultiServer{
		Factory: factory,
		Catalog: model.DefaultCatalog(),
		SkillReloader: func(ctx context.Context, runtimeID string, h *harness.Harness) ([]skill.Skill, error) {
			return []skill.Skill{{Name: "review", Description: "Review things", Body: "Check it."}}, nil
		},
	}
	var out bytes.Buffer
	if err := server.Serve(ctx, in, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{`"id":"1"`, `"alpha"`, `"id":"2"`, `"id":"3"`, `"id":"4"`, `"reasoning":"medium"`, `"id":"5"`, `"Fake Echo"`, `"id":"6"`, `"status":"started"`, `"method":"runtime_event"`, `"runtime_id":"alpha"`, `"type":"message_delta"`, "runtime says hi", `"id":"7"`, `"status":"settled"`, `"id":"8"`, `"leaf_id"`, `"id":"9"`, `"entries"`, `"id":"10"`, `"session_id"`, `"id":"11"`, `"review"`, `"id":"12"`, `"status":"saved"`, `"id":"13"`, "No earlier conversation", "keep facts", `"id":"14"`, `"messages"`, `"id":"15"`, `"status":"closed"`, `"id":"16"`, `"result":[]`} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %s:\n%s", want, got)
		}
	}
}
