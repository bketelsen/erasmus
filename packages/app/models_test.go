package app_test

import (
	"testing"

	"erasmus/packages/app"
	"erasmus/packages/config"
	"erasmus/packages/model"
)

func TestCatalogFromConfigAddsUserModels(t *testing.T) {
	catalog := app.CatalogFromConfig(config.Config{Models: []model.Model{
		{Provider: "openai-codex", ID: "codex-custom", DisplayName: "Custom Codex", Source: "user"},
	}}, model.DefaultCatalog())

	got, err := catalog.Find("openai-codex", "codex-custom")
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != "Custom Codex" || got.Source != "user" {
		t.Fatalf("model = %+v", got)
	}
}

func TestCatalogFromConfigOverridesBuiltinModels(t *testing.T) {
	catalog := app.CatalogFromConfig(config.Config{Models: []model.Model{
		{Provider: "openai-codex", ID: "gpt-5.5", DisplayName: "GPT-5.5 User Override", Source: "user"},
	}}, model.DefaultCatalog())

	got, err := catalog.Find("openai-codex", "gpt-5.5")
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != "GPT-5.5 User Override" || got.Source != "user" {
		t.Fatalf("model = %+v", got)
	}
}
