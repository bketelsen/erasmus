package model_test

import (
	"slices"
	"testing"

	"erasmus/packages/model"
)

func TestDefaultCatalogCodexModels(t *testing.T) {
	catalog := model.DefaultCatalog()
	defaultModel, err := catalog.Default("openai-codex")
	if err != nil {
		t.Fatal(err)
	}
	if defaultModel.ID != "gpt-5.5" {
		t.Fatalf("openai-codex default = %q, want gpt-5.5", defaultModel.ID)
	}
	want := []string{"gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.3-codex", "gpt-5.2", "gpt-5.1"}
	var got []string
	for _, m := range catalog.ListProvider("openai-codex") {
		got = append(got, m.ID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("openai-codex models = %v, want %v", got, want)
	}
}
