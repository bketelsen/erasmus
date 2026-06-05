package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bketelsen/erasmus/packages/auth"
	"github.com/bketelsen/erasmus/packages/config"
	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/provider"
	"github.com/bketelsen/erasmus/packages/session/memory"
)

func TestResolveHarnessConfigRefreshesExpiredGitHubCopilotOAuth(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/copilot_internal/v2/token":
			if got := r.Header.Get("Authorization"); got != "Bearer github-access" {
				t.Fatalf("authorization = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"copilot-new;proxy-ep=` + strings.TrimPrefix(serverURL(r), "https://") + `;","expires_at":4102444800}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	oldProvider := gitHubCopilotDeviceProvider
	gitHubCopilotDeviceProvider = func() auth.GitHubCopilotDeviceProvider {
		return auth.GitHubCopilotDeviceProvider{CopilotTokenURL: server.URL + "/copilot_internal/v2/token", HTTPClient: server.Client()}
	}
	defer func() { gitHubCopilotDeviceProvider = oldProvider }()

	store := auth.NewMemoryStore()
	if err := store.Set(ctx, auth.Credential{Provider: "github-copilot", OAuth: &auth.OAuthToken{AccessToken: "copilot-old;proxy-ep=proxy.individual.githubcopilot.com;", RefreshToken: "github-access", Expiry: time.Now().Add(-time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveHarnessConfig(ctx, ResolveOptions{
		Config:  config.Config{Provider: "github-copilot", Model: "gpt-4.1"},
		Session: memory.New("test"),
		Auth:    store,
	})
	if err != nil {
		t.Fatal(err)
	}
	cred, err := store.Get(ctx, "github-copilot")
	if err != nil {
		t.Fatal(err)
	}
	if cred.OAuth.AccessToken == "copilot-old;proxy-ep=proxy.individual.githubcopilot.com;" || cred.OAuth.RefreshToken != "github-access" || cred.OAuth.Expiry.IsZero() {
		t.Fatalf("bad refreshed credential: %+v", cred.OAuth)
	}
}

func TestRefreshModelCacheRefreshesExpiredGitHubCopilotOAuth(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/copilot_internal/v2/token":
			if got := r.Header.Get("Authorization"); got != "Bearer github-access" {
				t.Fatalf("authorization = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"copilot-new;proxy-ep=` + strings.TrimPrefix(serverURL(r), "https://") + `;","expires_at":4102444800}`))
		case "/models":
			if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer copilot-new;") {
				t.Fatalf("models authorization = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4.1"}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	oldProvider := gitHubCopilotDeviceProvider
	gitHubCopilotDeviceProvider = func() auth.GitHubCopilotDeviceProvider {
		return auth.GitHubCopilotDeviceProvider{CopilotTokenURL: server.URL + "/copilot_internal/v2/token", HTTPClient: server.Client()}
	}
	defer func() { gitHubCopilotDeviceProvider = oldProvider }()
	oldDefaultClient := http.DefaultClient
	http.DefaultClient = server.Client()
	defer func() { http.DefaultClient = oldDefaultClient }()

	store := auth.NewMemoryStore()
	if err := store.Set(ctx, auth.Credential{Provider: "github-copilot", OAuth: &auth.OAuthToken{AccessToken: "copilot-old;proxy-ep=proxy.individual.githubcopilot.com;", RefreshToken: "github-access", Expiry: time.Now().Add(-time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	models, err := RefreshModelCacheWithAuth(ctx, "github-copilot", model.NewFileCache(filepath.Join(t.TempDir(), "models.json")), store)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "gpt-4.1" {
		t.Fatalf("models = %+v", models)
	}
	cred, err := store.Get(ctx, "github-copilot")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cred.OAuth.AccessToken, "copilot-new;") || cred.OAuth.RefreshToken != "github-access" {
		t.Fatalf("bad refreshed credential: %+v", cred.OAuth)
	}
}

func TestResolveHarnessConfigExpiredGitHubCopilotRequiresGitHubAccessToken(t *testing.T) {
	ctx := context.Background()
	store := auth.NewMemoryStore()
	if err := store.Set(ctx, auth.Credential{Provider: "github-copilot", OAuth: &auth.OAuthToken{AccessToken: "copilot-old;proxy-ep=proxy.individual.githubcopilot.com;", Expiry: time.Now().Add(-time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveHarnessConfig(ctx, ResolveOptions{
		Config:  config.Config{Provider: "github-copilot", Model: "gpt-4.1"},
		Session: memory.New("test"),
		Auth:    store,
	})
	if err == nil || !strings.Contains(err.Error(), "github-copilot OAuth token is expired and has no GitHub access token") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveHarnessConfigRenewsExpiredGitHubUserToken(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "ghr_old" {
				t.Fatalf("refresh form = %s", r.Form.Encode())
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"ghu_new","refresh_token":"ghr_new","token_type":"bearer","expires_in":28800,"refresh_token_expires_in":15897600}`))
		case "/copilot_internal/v2/token":
			if got := r.Header.Get("Authorization"); got != "Bearer ghu_new" {
				t.Fatalf("authorization = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"copilot-new;proxy-ep=` + strings.TrimPrefix(serverURL(r), "https://") + `;","expires_at":4102444800}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	oldProvider := gitHubCopilotDeviceProvider
	gitHubCopilotDeviceProvider = func() auth.GitHubCopilotDeviceProvider {
		return auth.GitHubCopilotDeviceProvider{
			AccessTokenURL:  server.URL + "/login/oauth/access_token",
			CopilotTokenURL: server.URL + "/copilot_internal/v2/token",
			HTTPClient:      server.Client(),
		}
	}
	defer func() { gitHubCopilotDeviceProvider = oldProvider }()

	store := auth.NewMemoryStore()
	if err := store.Set(ctx, auth.Credential{Provider: "github-copilot", OAuth: &auth.OAuthToken{
		AccessToken:        "copilot-old;proxy-ep=proxy.individual.githubcopilot.com;",
		RefreshToken:       "ghu_old",
		GitHubRefreshToken: "ghr_old",
		GitHubTokenExpiry:  time.Now().Add(-time.Hour),
		Expiry:             time.Now().Add(-time.Hour),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveHarnessConfig(ctx, ResolveOptions{
		Config:  config.Config{Provider: "github-copilot", Model: "gpt-4.1"},
		Session: memory.New("test"),
		Auth:    store,
	}); err != nil {
		t.Fatal(err)
	}
	cred, err := store.Get(ctx, "github-copilot")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cred.OAuth.AccessToken, "copilot-new;") {
		t.Fatalf("copilot token not refreshed: %+v", cred.OAuth)
	}
	if cred.OAuth.RefreshToken != "ghu_new" {
		t.Fatalf("ghu_ not renewed: %q", cred.OAuth.RefreshToken)
	}
	if cred.OAuth.GitHubRefreshToken != "ghr_new" {
		t.Fatalf("ghr_ not rotated: %q", cred.OAuth.GitHubRefreshToken)
	}
	if d := time.Until(cred.OAuth.GitHubTokenExpiry); d < 7*time.Hour || d > 9*time.Hour {
		t.Fatalf("renewed ghu_ expiry delta = %s, want ~8h", d)
	}
}

func serverURL(r *http.Request) string {
	return "https://" + r.Host
}

// TestResolveStreamGitHubCopilotRefreshesExpiredTokenPerRequest proves the
// stream func re-checks the credential on every request: a token that is fresh
// when the stream is built but expires mid-session is renewed before the next
// request, rather than failing with the stale token baked in at resolve time.
func TestResolveStreamGitHubCopilotRefreshesExpiredTokenPerRequest(t *testing.T) {
	ctx := context.Background()
	var host string
	var tokenEndpointHits int
	var streamAuth []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/copilot_internal/v2/token":
			tokenEndpointHits++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"copilot-refreshed;proxy-ep=` + host + `;","expires_at":4102444800}`))
		case "/chat/completions":
			streamAuth = append(streamAuth, r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	host = strings.TrimPrefix(server.URL, "https://")

	oldProvider := gitHubCopilotDeviceProvider
	gitHubCopilotDeviceProvider = func() auth.GitHubCopilotDeviceProvider {
		return auth.GitHubCopilotDeviceProvider{CopilotTokenURL: server.URL + "/copilot_internal/v2/token", HTTPClient: server.Client()}
	}
	defer func() { gitHubCopilotDeviceProvider = oldProvider }()
	oldDefaultClient := http.DefaultClient
	http.DefaultClient = server.Client()
	defer func() { http.DefaultClient = oldDefaultClient }()

	store := auth.NewMemoryStore()
	freshToken := "copilot-initial;proxy-ep=" + host + ";"
	if err := store.Set(ctx, auth.Credential{Provider: "github-copilot", OAuth: &auth.OAuthToken{AccessToken: freshToken, RefreshToken: "github-access", Expiry: time.Now().Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}

	streamFn, err := resolveStream(ctx, model.Model{Provider: "github-copilot", ID: "gpt-4.1"}, store)
	if err != nil {
		t.Fatal(err)
	}

	// First request: the stored token is still valid, so no refresh happens and
	// the live token is sent.
	drainStream(t, ctx, streamFn, model.Model{Provider: "github-copilot", ID: "gpt-4.1"})
	if tokenEndpointHits != 0 {
		t.Fatalf("expected no refresh on first request, got %d", tokenEndpointHits)
	}
	if len(streamAuth) != 1 || streamAuth[0] != "Bearer "+freshToken {
		t.Fatalf("first request auth = %v", streamAuth)
	}

	// Simulate the Copilot token expiring mid-session.
	if err := store.Set(ctx, auth.Credential{Provider: "github-copilot", OAuth: &auth.OAuthToken{AccessToken: freshToken, RefreshToken: "github-access", Expiry: time.Now().Add(-time.Minute)}}); err != nil {
		t.Fatal(err)
	}

	// Second request: the stream func must renew the token before streaming.
	drainStream(t, ctx, streamFn, model.Model{Provider: "github-copilot", ID: "gpt-4.1"})
	if tokenEndpointHits != 1 {
		t.Fatalf("expected one refresh on second request, got %d", tokenEndpointHits)
	}
	if len(streamAuth) != 2 || !strings.HasPrefix(streamAuth[1], "Bearer copilot-refreshed;") {
		t.Fatalf("second request auth = %v", streamAuth)
	}

	cred, err := store.Get(ctx, "github-copilot")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cred.OAuth.AccessToken, "copilot-refreshed;") {
		t.Fatalf("refreshed token not persisted: %+v", cred.OAuth)
	}
}

func drainStream(t *testing.T, ctx context.Context, streamFn provider.StreamFunc, m model.Model) {
	t.Helper()
	events, err := streamFn(ctx, provider.Request{Model: m, Messages: []message.Message{{Role: message.RoleUser, Content: []message.Content{message.Text{Text: "hi"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}
}
