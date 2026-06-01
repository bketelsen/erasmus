package app_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"erasmus/packages/app"
	"erasmus/packages/auth"
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

func TestCatalogFromSourcesMergesCachedModelsBeforeUserOverrides(t *testing.T) {
	catalog := app.CatalogFromSources(config.Config{Models: []model.Model{
		{Provider: "openai-codex", ID: "gpt-5.6-codex", DisplayName: "User Codex", Source: "user"},
	}}, []model.Model{
		{Provider: "openai-codex", ID: "gpt-5.6-codex", DisplayName: "Cached Codex", Source: "cache"},
	}, model.DefaultCatalog())

	got, err := catalog.Find("openai-codex", "gpt-5.6-codex")
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != "User Codex" || got.Source != "user" {
		t.Fatalf("model = %+v", got)
	}
}

func TestRefreshModelCacheWritesFakeProviderModels(t *testing.T) {
	ctx := context.Background()
	cache := model.NewFileCache(filepath.Join(t.TempDir(), "models.json"))
	models, err := app.RefreshModelCache(ctx, "fake", cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].Provider != "fake" || models[0].ID != "echo" {
		t.Fatalf("refreshed models = %+v", models)
	}
	cached, err := cache.ListProvider(ctx, "fake")
	if err != nil {
		t.Fatal(err)
	}
	if len(cached) != 1 || cached[0].Provider != "fake" || cached[0].ID != "echo" || cached[0].Source != "cache" {
		t.Fatalf("cached models = %+v", cached)
	}
}

func TestRefreshModelCacheOpenAIRequiresAuthStore(t *testing.T) {
	_, err := app.RefreshModelCacheWithAuth(context.Background(), "openai", model.NewFileCache(filepath.Join(t.TempDir(), "models.json")), nil)
	if err == nil || !strings.Contains(err.Error(), `auth store is required for provider "openai"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestRefreshModelCacheCodexDiscoveryIsUnsupported(t *testing.T) {
	store := auth.NewMemoryStore()
	if err := store.Set(context.Background(), auth.Credential{Provider: "openai-codex", OAuth: &auth.OAuthToken{AccessToken: "tok", AccountID: "acct"}}); err != nil {
		t.Fatal(err)
	}
	_, err := app.RefreshModelCacheWithAuth(context.Background(), "openai-codex", model.NewFileCache(filepath.Join(t.TempDir(), "models.json")), store)
	if err == nil || !strings.Contains(err.Error(), "openai-codex model discovery is not available") {
		t.Fatalf("err = %v", err)
	}
}
