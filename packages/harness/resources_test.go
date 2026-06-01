package harness_test

import (
	"context"
	"testing"

	"erasmus/packages/event"
	"erasmus/packages/harness"
	"erasmus/packages/model"
	"erasmus/packages/provider"
	"erasmus/packages/session/memory"
	"erasmus/packages/skill"
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

func noopStream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	ch := make(chan provider.Event)
	close(ch)
	return ch, nil
}
