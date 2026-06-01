package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"erasmus/packages/app"
	"erasmus/packages/auth"
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

func TestModelsService(t *testing.T) {
	models := app.Models(nil)
	if len(models) == 0 {
		t.Fatal("expected default models")
	}
}
