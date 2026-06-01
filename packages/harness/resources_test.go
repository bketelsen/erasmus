package harness_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bketelsen/erasmus/packages/event"
	"github.com/bketelsen/erasmus/packages/harness"
	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/provider"
	"github.com/bketelsen/erasmus/packages/session/memory"
	"github.com/bketelsen/erasmus/packages/skill"
	"github.com/bketelsen/erasmus/packages/tool"
)

func TestHarnessSetSkillsEmitsResourceUpdate(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{Session: memory.New("test"), Stream: noopStream, Model: model.Model{Provider: "fake", ID: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	var got event.Event
	h.Subscribe(func(ev event.Event) { got = ev })
	if err := h.SetSkills(ctx, []skill.Skill{{Name: "review"}}); err != nil {
		t.Fatal(err)
	}
	update, ok := got.(event.ResourcesUpdate)
	if !ok {
		t.Fatalf("event = %T, want ResourcesUpdate", got)
	}
	if len(update.Skills) != 1 || update.Skills[0].Name != "review" {
		t.Fatalf("update = %+v", update)
	}
	st := h.State(ctx)
	if len(st.Skills) != 1 || st.Skills[0].Name != "review" {
		t.Fatalf("state = %+v", st.Skills)
	}
}

func TestHarnessSetToolsUpdatesActiveToolsAndEmitsResourceUpdate(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{Session: memory.New("test"), Stream: noopStream, Model: model.Model{Provider: "fake", ID: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	var got event.Event
	h.Subscribe(func(ev event.Event) { got = ev })
	if err := h.SetTools(ctx, []tool.Tool{namedTool("read"), namedTool("write")}, []string{"write"}); err != nil {
		t.Fatal(err)
	}
	update, ok := got.(event.ResourcesUpdate)
	if !ok {
		t.Fatalf("event = %T, want ResourcesUpdate", got)
	}
	if len(update.Tools) != 1 || update.Tools[0].Name != "write" {
		t.Fatalf("update tools = %+v", update.Tools)
	}
	if len(update.ActiveTools) != 1 || update.ActiveTools[0] != "write" {
		t.Fatalf("active tools = %+v", update.ActiveTools)
	}
	st := h.State(ctx)
	if _, ok := st.Agent.Tools.Get("write"); !ok {
		t.Fatal("active tool registry does not include write")
	}
	if _, ok := st.Agent.Tools.Get("read"); ok {
		t.Fatal("active tool registry includes inactive read tool")
	}
}

func TestHarnessSetActiveToolsFiltersExistingToolSet(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("test"),
		Stream:  noopStream,
		Model:   model.Model{Provider: "fake", ID: "test"},
		Tools:   tool.NewRegistry(namedTool("read"), namedTool("write")),
	})
	if err != nil {
		t.Fatal(err)
	}
	var got event.Event
	h.Subscribe(func(ev event.Event) { got = ev })
	if err := h.SetActiveTools(ctx, []string{"write"}); err != nil {
		t.Fatal(err)
	}
	update, ok := got.(event.ResourcesUpdate)
	if !ok {
		t.Fatalf("event = %T, want ResourcesUpdate", got)
	}
	if len(update.Tools) != 1 || update.Tools[0].Name != "write" {
		t.Fatalf("update tools = %+v", update.Tools)
	}
	st := h.State(ctx)
	if _, ok := st.Agent.Tools.Get("write"); !ok {
		t.Fatal("active tool registry does not include write")
	}
	if _, ok := st.Agent.Tools.Get("read"); ok {
		t.Fatal("active tool registry includes inactive read tool")
	}
}

func TestHarnessSetResourcesUpdatesSkillsAndTools(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{Session: memory.New("test"), Stream: noopStream, Model: model.Model{Provider: "fake", ID: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	var got event.Event
	h.Subscribe(func(ev event.Event) { got = ev })
	err = h.SetResources(ctx, harness.Resources{
		Skills:      []skill.Skill{{Name: "review"}},
		Tools:       []tool.Tool{namedTool("read"), namedTool("write")},
		ActiveTools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	update, ok := got.(event.ResourcesUpdate)
	if !ok {
		t.Fatalf("event = %T, want ResourcesUpdate", got)
	}
	if len(update.Skills) != 1 || update.Skills[0].Name != "review" {
		t.Fatalf("update skills = %+v", update.Skills)
	}
	if len(update.Tools) != 1 || update.Tools[0].Name != "read" {
		t.Fatalf("update tools = %+v", update.Tools)
	}
	st := h.State(ctx)
	if len(st.Skills) != 1 || st.Skills[0].Name != "review" {
		t.Fatalf("state skills = %+v", st.Skills)
	}
	if _, ok := st.Agent.Tools.Get("read"); !ok {
		t.Fatal("active tool registry does not include read")
	}
	if _, ok := st.Agent.Tools.Get("write"); ok {
		t.Fatal("active tool registry includes inactive write tool")
	}
}

func noopStream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	ch := make(chan provider.Event)
	close(ch)
	return ch, nil
}

type namedTool string

func (t namedTool) Name() string            { return string(t) }
func (t namedTool) Description() string     { return string(t) }
func (t namedTool) Schema() json.RawMessage { return nil }
func (t namedTool) Execute(context.Context, json.RawMessage, func(tool.Progress)) (tool.Result, error) {
	return tool.Result{Content: []message.Content{message.Text{Text: "ran"}}}, nil
}
