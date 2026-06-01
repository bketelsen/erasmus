package model_test

import (
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
	for _, id := range []string{"gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex", "gpt-5.3-codex-spark", "gpt-5.2"} {
		if _, err := catalog.Find("openai-codex", id); err != nil {
			t.Fatalf("codex model %q missing: %v", id, err)
		}
	}
}
