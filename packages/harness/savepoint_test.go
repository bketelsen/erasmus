package harness_test

import (
	"context"
	"testing"

	"erasmus/packages/event"
	"erasmus/packages/harness"
	"erasmus/packages/model"
	"erasmus/packages/session/memory"
)

func TestHarnessSavePointPersistsAndEmitsEvent(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{Session: memory.New("savepoint"), Stream: noopStream, Model: model.Model{Provider: "fake", ID: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	var got event.Event
	h.Subscribe(func(ev event.Event) { got = ev })
	entryID, err := h.SavePoint(ctx, "before-edit", map[string]string{"path": "main.go"})
	if err != nil {
		t.Fatal(err)
	}
	if entryID == "" {
		t.Fatal("entry id is empty")
	}
	update, ok := got.(event.SavePoint)
	if !ok {
		t.Fatalf("event = %T, want SavePoint", got)
	}
	if update.EntryID != string(entryID) || update.Label != "before-edit" {
		t.Fatalf("save point event = %+v, entry id = %q", update, entryID)
	}
	tree, err := h.Tree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tree.LeafID != entryID {
		t.Fatalf("leaf id = %q, want %q", tree.LeafID, entryID)
	}
}
