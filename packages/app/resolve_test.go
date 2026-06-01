package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"erasmus/packages/app"
	"erasmus/packages/auth"
	"erasmus/packages/config"
	"erasmus/packages/model"
	"erasmus/packages/provider"
	"erasmus/packages/session/memory"
)

func TestResolveHarnessConfigDefaultsFakeModelAndTools(t *testing.T) {
	resolved, err := app.ResolveHarnessConfig(context.Background(), app.ResolveOptions{
		Session: memory.New("test"),
		Stream:  noopStream,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Model.Provider != "fake" || resolved.Model.ID != "echo" {
		t.Fatalf("model = %+v", resolved.Model)
	}
	if _, ok := resolved.Tools.Get("read"); !ok {
		t.Fatal("read tool missing")
	}
}

func TestResolveHarnessConfigSelectsTools(t *testing.T) {
	resolved, err := app.ResolveHarnessConfig(context.Background(), app.ResolveOptions{
		Config:  config.Config{Tools: []string{"read"}},
		Session: memory.New("test"),
		Stream:  noopStream,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Tools.List()) != 1 {
		t.Fatalf("tools len = %d, want 1", len(resolved.Tools.List()))
	}
	if _, ok := resolved.Tools.Get("read"); !ok {
		t.Fatal("read tool missing")
	}
	if _, ok := resolved.Tools.Get("write"); ok {
		t.Fatal("write tool should not be active")
	}
}

func TestResolveHarnessConfigBuildsFakeStreamWhenOmitted(t *testing.T) {
	resolved, err := app.ResolveHarnessConfig(context.Background(), app.ResolveOptions{Session: memory.New("test")})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Harness.Stream == nil {
		t.Fatal("expected stream")
	}
}

func TestResolveHarnessConfigBuildsOpenAIStreamFromAuth(t *testing.T) {
	store := auth.NewMemoryStore()
	if err := store.Set(context.Background(), auth.Credential{Provider: "openai", APIKey: "key"}); err != nil {
		t.Fatal(err)
	}
	resolved, err := app.ResolveHarnessConfig(context.Background(), app.ResolveOptions{
		Config:  config.Config{Provider: "openai", Model: "gpt-4o-mini"},
		Session: memory.New("test"),
		Auth:    store,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Model.Provider != "openai" || resolved.Harness.Stream == nil {
		t.Fatalf("unexpected resolved: %+v", resolved.Model)
	}
}

func TestResolveHarnessConfigBuildsCodexStreamFromOAuth(t *testing.T) {
	store := auth.NewMemoryStore()
	if err := store.Set(context.Background(), auth.Credential{Provider: "openai-codex", OAuth: &auth.OAuthToken{AccessToken: "tok", AccountID: "acct"}}); err != nil {
		t.Fatal(err)
	}
	resolved, err := app.ResolveHarnessConfig(context.Background(), app.ResolveOptions{
		Config:  config.Config{Provider: "openai-codex", Model: "gpt-5.3-codex"},
		Session: memory.New("test"),
		Auth:    store,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Model.Provider != "openai-codex" || resolved.Harness.Stream == nil {
		t.Fatalf("unexpected resolved: %+v", resolved.Model)
	}
}

func TestResolveHarnessConfigAllowsExplicitProviderModelOutsideStaticCatalog(t *testing.T) {
	resolved, err := app.ResolveHarnessConfig(context.Background(), app.ResolveOptions{
		Config:  config.Config{Provider: "openai-codex", Model: "future-codex-model"},
		Session: memory.New("test"),
		Stream:  noopStream,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Model.Provider != "openai-codex" || resolved.Model.ID != "future-codex-model" || resolved.Model.Source != "explicit" {
		t.Fatalf("model = %+v", resolved.Model)
	}
}

func TestResolveHarnessConfigRefreshesExpiredCodexOAuth(t *testing.T) {
	oldProvider := auth.OpenAIOAuth
	defer func() { auth.OpenAIOAuth = oldProvider }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type = %q", got)
		}
		if got := r.Form.Get("refresh_token"); got != "refresh-old" {
			t.Fatalf("refresh_token = %q", got)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-new","expires_in":3600}`))
	}))
	defer srv.Close()
	auth.OpenAIOAuth.TokenURL = srv.URL

	store := auth.NewMemoryStore()
	if err := store.Set(context.Background(), auth.Credential{Provider: "openai-codex", OAuth: &auth.OAuthToken{AccessToken: "tok-old", RefreshToken: "refresh-old", AccountID: "acct", Expiry: time.Now().Add(-time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	_, err := app.ResolveHarnessConfig(context.Background(), app.ResolveOptions{
		Config:  config.Config{Provider: "openai-codex", Model: "gpt-5.3-codex"},
		Session: memory.New("test"),
		Auth:    store,
	})
	if err != nil {
		t.Fatal(err)
	}
	cred, err := store.Get(context.Background(), "openai-codex")
	if err != nil {
		t.Fatal(err)
	}
	if cred.OAuth.AccessToken != "tok-new" || cred.OAuth.RefreshToken != "refresh-old" || cred.OAuth.AccountID != "acct" {
		t.Fatalf("bad refreshed credential: %+v", cred.OAuth)
	}
}

func TestResolveHarnessConfigRequiresAuthForNonFakeProvider(t *testing.T) {
	catalog := model.StaticCatalog{Models: []model.Model{{Provider: "real", ID: "m"}}, Defaults: map[string]string{"real": "m"}}
	_, err := app.ResolveHarnessConfig(context.Background(), app.ResolveOptions{
		Config:  config.Config{Provider: "real", Model: "m"},
		Session: memory.New("test"),
		Stream:  noopStream,
		Catalog: catalog,
		Auth:    auth.NewMemoryStore(),
	})
	if err == nil {
		t.Fatal("expected missing auth error")
	}

	store := auth.NewMemoryStore()
	if err := store.Set(context.Background(), auth.Credential{Provider: "real", APIKey: "key"}); err != nil {
		t.Fatal(err)
	}
	_, err = app.ResolveHarnessConfig(context.Background(), app.ResolveOptions{
		Config:  config.Config{Provider: "real", Model: "m"},
		Session: memory.New("test"),
		Stream:  noopStream,
		Catalog: catalog,
		Auth:    store,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func noopStream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	ch := make(chan provider.Event)
	close(ch)
	return ch, nil
}
