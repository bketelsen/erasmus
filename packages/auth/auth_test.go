package auth_test

import (
	"context"
	"testing"

	"erasmus/packages/auth"
)

func TestMemoryStoreSetGetListDelete(t *testing.T) {
	ctx := context.Background()
	store := auth.NewMemoryStore()
	if err := store.Set(ctx, auth.Credential{Provider: "fake", APIKey: "key"}); err != nil {
		t.Fatal(err)
	}
	cred, err := store.Get(ctx, "fake")
	if err != nil {
		t.Fatal(err)
	}
	if cred.APIKey != "key" {
		t.Fatalf("api key = %q", cred.APIKey)
	}
	creds, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 1 {
		t.Fatalf("creds len = %d", len(creds))
	}
	if err := store.Delete(ctx, "fake"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "fake"); err == nil {
		t.Fatal("expected missing credential error")
	}
}
