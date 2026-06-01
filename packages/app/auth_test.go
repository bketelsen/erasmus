package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bketelsen/erasmus/packages/app"
	"github.com/bketelsen/erasmus/packages/auth"
)

func TestAuthServices(t *testing.T) {
	ctx := context.Background()
	store := auth.NewMemoryStore()
	if err := app.Login(ctx, store, "fake", "key"); err != nil {
		t.Fatal(err)
	}
	providers, err := app.AuthStatus(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers[0] != "fake" {
		t.Fatalf("providers = %v", providers)
	}
	if err := app.Logout(ctx, store, "fake"); err != nil {
		t.Fatal(err)
	}
	providers, err = app.AuthStatus(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 0 {
		t.Fatalf("providers = %v", providers)
	}
}

func TestAuthStatusDetails(t *testing.T) {
	ctx := context.Background()
	store := auth.NewMemoryStore()
	expiry := time.Now().Add(time.Hour).Truncate(time.Second)
	if err := store.Set(ctx, auth.Credential{Provider: "openai-codex", OAuth: &auth.OAuthToken{AccessToken: "secret", AccountID: "acct-123", Expiry: expiry}}); err != nil {
		t.Fatal(err)
	}
	entries, err := app.AuthStatusDetails(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %v", entries)
	}
	entry := entries[0]
	if entry.Provider != "openai-codex" || entry.Method != "oauth" || entry.AccountID != "acct-123" || !entry.Expiry.Equal(expiry) {
		t.Fatalf("entry = %+v", entry)
	}
	formatted := entry.String()
	if !strings.Contains(formatted, "openai-codex\toauth\taccount=acct-123") || strings.Contains(formatted, "secret") {
		t.Fatalf("formatted = %q", formatted)
	}
}

func TestLoginGitHubCopilotDeviceStoresOAuthCredential(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/login/device/code":
			_, _ = w.Write([]byte(`{"device_code":"device-123","user_code":"ABCD-EFGH","verification_uri":"https://github.com/login/device","interval":1,"expires_in":60}`))
		case "/login/oauth/access_token":
			_, _ = w.Write([]byte(`{"access_token":"github-access","token_type":"bearer","scope":"read:user"}`))
		case "/copilot_internal/v2/token":
			_, _ = w.Write([]byte(`{"token":"copilot-access","expires_at":4102444800}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	store := auth.NewMemoryStore()
	var out strings.Builder
	provider := auth.GitHubCopilotDeviceProvider{
		DeviceCodeURL:   srv.URL + "/login/device/code",
		AccessTokenURL:  srv.URL + "/login/oauth/access_token",
		CopilotTokenURL: srv.URL + "/copilot_internal/v2/token",
		PollSleep:       func(context.Context, time.Duration) error { return nil },
	}
	if err := app.LoginGitHubCopilotDevice(ctx, store, provider, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ABCD-EFGH") || strings.Contains(out.String(), "copilot-access") {
		t.Fatalf("login output = %q", out.String())
	}
	cred, err := store.Get(ctx, "github-copilot")
	if err != nil {
		t.Fatal(err)
	}
	if cred.OAuth == nil || cred.OAuth.AccessToken != "copilot-access" || cred.OAuth.RefreshToken != "github-access" {
		t.Fatalf("credential = %+v", cred)
	}
}

func TestModelsService(t *testing.T) {
	models := app.Models(nil)
	if len(models) == 0 {
		t.Fatal("expected default models")
	}
}
