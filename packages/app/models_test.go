package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestRefreshModelCacheGitHubCopilotDiscoversAccountModels(t *testing.T) {
	ctx := context.Background()
	var gotAuth, gotUserAgent string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-sonnet-4.5"},{"id":"gpt-5.3-codex"},{"id":"account-only-preview"}]}`))
	}))
	defer server.Close()
	oldDefaultClient := http.DefaultClient
	http.DefaultClient = server.Client()
	defer func() { http.DefaultClient = oldDefaultClient }()

	store := auth.NewMemoryStore()
	if err := store.Set(ctx, auth.Credential{Provider: "github-copilot", OAuth: &auth.OAuthToken{AccessToken: "copilot-token;proxy-ep=" + strings.TrimPrefix(server.URL, "https://") + ";", RefreshToken: "github-access"}}); err != nil {
		t.Fatal(err)
	}
	cache := model.NewFileCache(filepath.Join(t.TempDir(), "models.json"))
	models, err := app.RefreshModelCacheWithAuth(ctx, "github-copilot", cache, store)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer copilot-token;proxy-ep="+strings.TrimPrefix(server.URL, "https://")+";" || gotUserAgent != "GitHubCopilotChat/0.35.0" {
		t.Fatalf("headers auth=%q user-agent=%q", gotAuth, gotUserAgent)
	}
	if len(models) != 3 {
		t.Fatalf("models = %+v", models)
	}
	if models[0].ID != "account-only-preview" || models[0].DisplayName != "account-only-preview" || models[0].Source != "live" {
		t.Fatalf("unknown model metadata = %+v", models[0])
	}
	if models[1].ID != "claude-sonnet-4.5" || models[1].DisplayName != "Claude Sonnet 4.5" || !models[1].Reasoning || models[1].ContextWindow == 0 || models[1].Source != "live" {
		t.Fatalf("claude metadata = %+v", models[1])
	}
	if models[2].ID != "gpt-5.3-codex" || models[2].DisplayName != "GPT-5.3-Codex" || !models[2].Reasoning || models[2].ContextWindow == 0 || models[2].Source != "live" {
		t.Fatalf("gpt metadata = %+v", models[2])
	}
	cached, err := cache.ListProvider(ctx, "github-copilot")
	if err != nil {
		t.Fatal(err)
	}
	if len(cached) != 3 || cached[0].Source != "cache" || cached[1].Source != "cache" || cached[2].Source != "cache" {
		t.Fatalf("cached models = %+v", cached)
	}
}
